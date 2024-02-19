package mweb_test

import (
	"cmp"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ltcsuite/lnd"
	"github.com/ltcsuite/lnd/lnrpc"
	"github.com/ltcsuite/lnd/lnrpc/walletrpc"
	"github.com/ltcsuite/lnd/macaroons"
	"github.com/ltcsuite/lnd/signal"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/rogpeppe/go-internal/testscript"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

const (
	lndRpcPort        = "11009"
	p2pPort           = "19555"
	lndWalletPassword = "password"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"lnd": func() int {
			os.Args = append(os.Args,
				"--litecoin.active", "--litecoin.node=neutrino",
				"--litecoin.regtest", "--lnddir=$WORK/lnd",
				"--rpclisten=localhost:"+lndRpcPort,
				"--neutrino.addpeer=localhost:"+p2pPort)
			interceptor, _ := signal.Intercept()
			config, _ := lnd.LoadConfig(interceptor)
			err := lnd.Main(config, lnd.ListenerCfg{},
				config.ImplementationConfig(interceptor), interceptor)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return 0
		},
	}))
}

func TestLitecoindLndInteraction(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(e *testscript.Env) error {
			e.Setenv("P2P_PORT", p2pPort)
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"litecoin-cli":    _litecoinCli,
			"openRpcConn":     openRpcConn,
			"generate":        _generate,
			"createWallet":    createWallet,
			"newAddress":      _newAddress,
			"getResult":       getResult,
			"checkResult":     checkResult,
			"syncToBlock":     syncToBlock,
			"waitForUtxos":    _waitForUtxos,
			"checkUtxo":       _checkUtxo,
			"checkTxnHistory": _checkTxnHistory,
			"sendCoins":       _sendCoins,
			"walletBalance":   _walletBalance,
			"stressTest":      stressTest,
		},
	})
}

func litecoinCli(ts *testscript.TestScript, args ...string) {
	_litecoinCli(ts, false, args)
}

func _litecoinCli(ts *testscript.TestScript, neg bool, args []string) {
	args = append([]string{"litecoin-cli",
		"-datadir=" + ts.Getenv("WORK") + "/litecoin",
		"-chain=regtest"}, args...)
	if slices.Contains(args, "-rpcwait") {
		args = append([]string{"timeout", "5"}, args...)
	}
	ts.Check(ts.Exec(args[0], args[1:]...))
}

var rpcConn *grpc.ClientConn

func openRpcConn(ts *testscript.TestScript, neg bool, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rpcConn != nil {
		rpcConn.Close()
	}
	for ; ; time.Sleep(time.Second) {
		if deadline, _ := ctx.Deadline(); time.Now().After(deadline) {
			ts.Fatalf("open rpc timed out")
		}
		tls, err := credentials.NewClientTLSFromFile(args[0]+"/tls.cert", "")
		if err != nil {
			continue
		}
		opts := []grpc.DialOption{grpc.WithBlock(), grpc.WithTransportCredentials(tls)}
		if args[1] == "1" {
			macCred, err := loadMacaroon(args[0])
			if err != nil {
				continue
			}
			opts = append(opts, grpc.WithPerRPCCredentials(macCred))
		}
		rpcConn, err = grpc.DialContext(ctx, "localhost:"+lndRpcPort, opts...)
		ts.Check(err)
		break
	}
}

func loadMacaroon(lndPath string) (macCred macaroons.MacaroonCredential, err error) {
	buf, err := os.ReadFile(lndPath + "/data/chain/litecoin/regtest/admin.macaroon")
	if err != nil {
		return
	}
	mac := &macaroon.Macaroon{}
	if err = mac.UnmarshalBinary(buf); err != nil {
		return
	}
	return macaroons.NewMacaroonCredential(mac)
}

func generate(ts *testscript.TestScript, count int) {
	if count > 0 {
		_generate(ts, false, []string{strconv.Itoa(count)})
	}
}

func _generate(ts *testscript.TestScript, neg bool, args []string) {
	_litecoinCli(ts, neg, []string{"-generate", args[0]})
	getResult(ts, neg, []string{"BLOCK_HASH", "blocks", "-1"})
	syncToBlock(ts, neg, []string{ts.Getenv("BLOCK_HASH")})
}

func createWallet(ts *testscript.TestScript, neg bool, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for ; ; time.Sleep(time.Second) {
		if deadline, _ := ctx.Deadline(); time.Now().After(deadline) {
			ts.Fatalf("waiting for rpc timed out")
		}
		client := lnrpc.NewStateClient(rpcConn)
		resp, err := client.GetState(ctx, &lnrpc.GetStateRequest{})
		ts.Check(err)
		if resp.State == lnrpc.WalletState_NON_EXISTING {
			break
		}
	}
	client := lnrpc.NewWalletUnlockerClient(rpcConn)
	resp, err := client.GenSeed(ctx, &lnrpc.GenSeedRequest{})
	ts.Check(err)
	_, err = client.InitWallet(ctx, &lnrpc.InitWalletRequest{
		WalletPassword:     []byte(lndWalletPassword),
		CipherSeedMnemonic: resp.CipherSeedMnemonic,
	})
	ts.Check(err)
}

func newAddress(ts *testscript.TestScript, kind string) string {
	addressType := lnrpc.AddressType_UNUSED_WITNESS_PUBKEY_HASH
	if kind == "mweb" {
		addressType = lnrpc.AddressType_UNUSED_MWEB
	}
	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.NewAddress(context.Background(),
		&lnrpc.NewAddressRequest{Type: addressType})
	ts.Check(err)
	return resp.Address
}

func _newAddress(ts *testscript.TestScript, neg bool, args []string) {
	ts.Stdout().Write([]byte(newAddress(ts, args[0])))
}

func extractResult(ts *testscript.TestScript,
	result string, path []string) string {

	result = strings.TrimSpace(result)
	if len(path) > 0 {
		var data any
		ts.Check(json.Unmarshal([]byte(result), &data))
		for ; len(path) > 0; path = path[1:] {
			if i, err := strconv.Atoi(path[0]); err == nil {
				if i < 0 {
					i += len(data.([]any))
				}
				data = data.([]any)[i]
			} else {
				data = data.(map[string]any)[path[0]]
			}
		}
		result = fmt.Sprint(data)
	}
	return result
}

func getResult(ts *testscript.TestScript, neg bool, args []string) {
	envVar, path := args[0], args[1:]
	result := extractResult(ts, ts.ReadFile("stdout"), path)
	ts.Setenv(envVar, result)
	ts.Stdout().Write([]byte(result))
}

func checkResult(ts *testscript.TestScript, neg bool, args []string) {
	expected, envVar, path := args[0], args[1], args[2:]
	result := extractResult(ts, ts.Getenv(envVar), path)
	if !neg && result != expected {
		ts.Fatalf("expected %s, got %s", expected, result)
	} else if neg && result == expected {
		ts.Fatalf("expected ! %s", expected)
	}
}

func syncToBlock(ts *testscript.TestScript, neg bool, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for ; ; time.Sleep(time.Second) {
		if deadline, _ := ctx.Deadline(); time.Now().After(deadline) {
			ts.Fatalf("syncing to block timed out")
		}
		client := lnrpc.NewLightningClient(rpcConn)
		resp, err := client.GetInfo(ctx, &lnrpc.GetInfoRequest{})
		ts.Check(err)
		if resp.BlockHash == args[0] && resp.SyncedToChain {
			break
		}
	}
}

func waitForUtxos(ts *testscript.TestScript, count, maxConfs int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for ; ; time.Sleep(time.Second) {
		if deadline, _ := ctx.Deadline(); time.Now().After(deadline) {
			ts.Fatalf("waiting for utxos timed out")
		}
		client := walletrpc.NewWalletKitClient(rpcConn)
		resp, err := client.ListUnspent(ctx,
			&walletrpc.ListUnspentRequest{MaxConfs: int32(maxConfs)})
		ts.Check(err)
		if len(resp.Utxos) == count {
			break
		}
	}
}

func _waitForUtxos(ts *testscript.TestScript, neg bool, args []string) {
	count, err := strconv.Atoi(args[0])
	ts.Check(err)
	maxConfs := 1
	if len(args) == 2 {
		maxConfs, err = strconv.Atoi(args[1])
		ts.Check(err)
	}
	waitForUtxos(ts, count, maxConfs)
}

func checkUtxo(ts *testscript.TestScript, index int, addr string, value float64) {
	_checkUtxo(ts, false, []string{strconv.Itoa(index), addr, fmt.Sprint(value)})
}

func _checkUtxo(ts *testscript.TestScript, neg bool, args []string) {
	client := walletrpc.NewWalletKitClient(rpcConn)
	resp, err := client.ListUnspent(context.Background(),
		&walletrpc.ListUnspentRequest{MaxConfs: 100})
	ts.Check(err)
	index, err := strconv.Atoi(args[0])
	ts.Check(err)

	slices.SortFunc(resp.Utxos, func(a, b *lnrpc.Utxo) int {
		if a.Confirmations == b.Confirmations {
			return cmp.Compare(a.AmountSat, b.AmountSat)
		}
		return cmp.Compare(a.Confirmations, b.Confirmations)
	})
	utxo := resp.Utxos[index]
	if utxo.Address != args[1] {
		ts.Fatalf("utxo address mismatch")
	}
	value, err := strconv.ParseFloat(args[2], 64)
	ts.Check(err)
	if utxo.AmountSat != int64(math.Round(value*ltcutil.SatoshiPerBitcoin)) {
		ts.Fatalf("utxo value mismatch, expected %v", utxo.AmountSat)
	}
}

func checkTxnHistory(ts *testscript.TestScript, index int,
	txid string, value, fee float64, addrs []string) []string {

	expectedAddrs := make(map[string]int)
	unknown := 0
	for _, addr := range addrs {
		if addr == "?" {
			unknown++
		} else {
			expectedAddrs[addr]++
		}
	}

	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.GetTransactions(context.Background(),
		&lnrpc.GetTransactionsRequest{})
	ts.Check(err)
	tx := resp.Transactions[index]
	if txid != "" && tx.TxHash != txid {
		ts.Fatalf("txn hash mismatch")
	}
	if tx.Amount != int64(math.Round(value*ltcutil.SatoshiPerBitcoin)) {
		ts.Fatalf("txn value mismatch, expected %v", tx.Amount)
	}
	if tx.TotalFees != int64(math.Round(fee*ltcutil.SatoshiPerBitcoin)) {
		ts.Fatalf("txn fee mismatch")
	}

	var unknownAddrs []string
	for _, addr := range tx.DestAddresses {
		if expectedAddrs[addr] > 0 {
			if expectedAddrs[addr]--; expectedAddrs[addr] == 0 {
				delete(expectedAddrs, addr)
			}
		} else {
			unknownAddrs = append(unknownAddrs, addr)
		}
	}

	if len(expectedAddrs) > 0 {
		ts.Fatalf("failed to match destination addresses")
	}
	if len(unknownAddrs) > unknown {
		ts.Fatalf("unknown address count greater than expected")
	}
	return unknownAddrs
}

func _checkTxnHistory(ts *testscript.TestScript, neg bool, args []string) {
	index, err := strconv.Atoi(args[0])
	ts.Check(err)
	value, err := strconv.ParseFloat(args[2], 64)
	ts.Check(err)
	fee, err := strconv.ParseFloat(args[3], 64)
	ts.Check(err)
	checkTxnHistory(ts, index, args[1], value, fee, strings.Split(args[4], ","))
}

func sendCoins(ts *testscript.TestScript, addr string, value float64) string {
	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.SendCoins(context.Background(),
		&lnrpc.SendCoinsRequest{Addr: addr,
			Amount: int64(math.Round(value * ltcutil.SatoshiPerBitcoin))})
	ts.Check(err)
	return resp.Txid
}

func _sendCoins(ts *testscript.TestScript, neg bool, args []string) {
	value, err := strconv.ParseFloat(args[1], 64)
	ts.Check(err)
	ts.Stdout().Write([]byte(sendCoins(ts, args[0], value)))
}

func walletBalance(ts *testscript.TestScript) float64 {
	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.WalletBalance(context.Background(),
		&lnrpc.WalletBalanceRequest{})
	ts.Check(err)
	return float64(resp.ConfirmedBalance) / ltcutil.SatoshiPerBitcoin
}

func _walletBalance(ts *testscript.TestScript, neg bool, args []string) {
	ts.Stdout().Write([]byte(fmt.Sprint(walletBalance(ts))))
}

func getTransactionFee(ts *testscript.TestScript, txid string) float64 {
	client := walletrpc.NewWalletKitClient(rpcConn)
	resp, err := client.GetTransaction(context.Background(),
		&walletrpc.GetTransactionRequest{Txid: txid})
	ts.Check(err)
	return float64(resp.TotalFees) / ltcutil.SatoshiPerBitcoin
}

func isMwebTransaction(ts *testscript.TestScript, txid string) bool {
	client := walletrpc.NewWalletKitClient(rpcConn)
	resp, err := client.GetTransaction(context.Background(),
		&walletrpc.GetTransactionRequest{Txid: txid})
	ts.Check(err)
	var tx wire.MsgTx
	ts.Check(tx.Deserialize(hex.NewDecoder(strings.NewReader(resp.RawTxHex))))
	return tx.Mweb != nil
}

type litecoindUnspents []struct {
	Txid    string  `json:"txid"`
	Amount  float64 `json:"amount"`
	Address string  `json:"address"`
}

func litecoinCliListUnspent(ts *testscript.TestScript, maxConfs int) litecoindUnspents {
	litecoinCli(ts, "listunspent", "0", strconv.Itoa(maxConfs))
	dec := json.NewDecoder(strings.NewReader(ts.ReadFile("stdout")))
	var v litecoindUnspents
	ts.Check(dec.Decode(&v))
	return v
}

func stressTest(ts *testscript.TestScript, neg bool, args []string) {
	cycles, err := strconv.Atoi(args[0])
	ts.Check(err)
	for i := 0; i < cycles; i++ {
		litecoinCli(ts, "getbalance")
		litecoindBal, err := strconv.ParseFloat(
			strings.TrimSpace(ts.ReadFile("stdout")), 64)
		ts.Check(err)
		lndBal := walletBalance(ts)

		const (
			LitecoindToLndMwebAddr = iota
			LitecoindToLndCanonicalAddr
			LndToLitecoindMwebAddr
			LndToLitecoindCanonicalAddr
			LndToLndMwebAddr
			LndToLndCanonicalAddr
			NumberOfCases
		)
		selectedCase := rand.Intn(NumberOfCases)
		switch selectedCase {
		case LitecoindToLndMwebAddr, LitecoindToLndCanonicalAddr:
			amt := math.Round(litecoindBal*rand.Float64()*0.99*
				ltcutil.SatoshiPerBitcoin) / ltcutil.SatoshiPerBitcoin
			if amt < 0.001 {
				continue
			}
			var addr string
			var confs int
			switch selectedCase {
			case LitecoindToLndMwebAddr:
				addr = newAddress(ts, "mweb")
				confs = 1
			case LitecoindToLndCanonicalAddr:
				addr = newAddress(ts, "p2wkh")
				confs = 6
			}
			ts.Logf("Sending %v to %s (litecoind -> lnd)", amt, addr)
			litecoinCli(ts, "sendtoaddress", addr, fmt.Sprint(amt))
			generate(ts, confs)
			waitForUtxos(ts, 1, confs)
			checkUtxo(ts, 0, addr, amt)
			checkTxnHistory(ts, 0, "", amt, 0, []string{addr, "?"})

		case LndToLitecoindMwebAddr, LndToLitecoindCanonicalAddr:
			amt := math.Round(lndBal*rand.Float64()*0.99*
				ltcutil.SatoshiPerBitcoin) / ltcutil.SatoshiPerBitcoin
			if amt < 0.001 {
				continue
			}
			var confs int
			switch selectedCase {
			case LndToLitecoindMwebAddr:
				litecoinCli(ts, "getnewaddress", "", "mweb")
				confs = 1
			case LndToLitecoindCanonicalAddr:
				litecoinCli(ts, "getnewaddress")
				confs = 6
			}
			addr := strings.TrimSpace(ts.ReadFile("stdout"))
			ts.Logf("Sending %v to %s (lnd -> litecoind)", amt, addr)
			txid := sendCoins(ts, addr, amt)
			usedBal := lndBal - walletBalance(ts)
			fee := getTransactionFee(ts, txid)
			generate(ts, 1)
			txnAddrs := checkTxnHistory(ts, 0, txid, -amt-fee, fee, []string{addr, "?"})
			waitForUtxos(ts, 1, 1)
			checkUtxo(ts, 0, txnAddrs[0], usedBal-amt-fee)
			generate(ts, confs-1)
			unspents := litecoinCliListUnspent(ts, confs)
			if unspents[0].Txid != txid && selectedCase != LndToLitecoindCanonicalAddr {
				ts.Fatalf("txid mismatch")
			}
			if unspents[0].Amount != amt {
				ts.Fatalf("amount mismatch")
			}
			if unspents[0].Address != addr {
				ts.Fatalf("address mismatch")
			}

		case LndToLndMwebAddr, LndToLndCanonicalAddr:
			amt := math.Round(lndBal*rand.Float64()*0.99*
				ltcutil.SatoshiPerBitcoin) / ltcutil.SatoshiPerBitcoin
			if amt < 0.001 {
				continue
			}
			var addr string
			var confs int
			switch selectedCase {
			case LndToLndMwebAddr:
				addr = newAddress(ts, "mweb")
				confs = 1
			case LndToLndCanonicalAddr:
				addr = newAddress(ts, "p2wkh")
				confs = 6
			}
			ts.Logf("Sending %v to %s (lnd -> lnd)", amt, addr)
			txid := sendCoins(ts, addr, amt)
			usedBal := lndBal - walletBalance(ts)
			fee := getTransactionFee(ts, txid)
			txnValue := -fee
			if selectedCase == LndToLndCanonicalAddr && isMwebTransaction(ts, txid) {
				txnValue -= amt
			}
			txnAddrs := checkTxnHistory(ts, 0, txid, txnValue, fee, []string{addr, "?"})
			generate(ts, confs)
			waitForUtxos(ts, 2, confs)
			x := 0
			if amt > usedBal-amt-fee {
				x = 1
			}
			checkUtxo(ts, 0^x, addr, amt)
			checkUtxo(ts, 1^x, txnAddrs[0], usedBal-amt-fee)
		}
	}
}
