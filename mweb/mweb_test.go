package mweb_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
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
			"litecoin-cli":    litecoinCli,
			"openRpcConn":     openRpcConn,
			"createWallet":    createWallet,
			"newAddress":      newAddress,
			"getResult":       getResult,
			"syncToBlock":     syncToBlock,
			"waitForUtxos":    waitForUtxos,
			"checkUtxo":       checkUtxo,
			"checkTxnHistory": checkTxnHistory,
			"sendCoins":       sendCoins,
		},
	})
}

func litecoinCli(ts *testscript.TestScript, neg bool, args []string) {
	ts.Check(ts.Exec("litecoin-cli", append([]string{
		"-datadir=" + ts.Getenv("WORK") + "/litecoin",
		"-chain=regtest"}, args...)...))
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

func newAddress(ts *testscript.TestScript, neg bool, args []string) {
	addressType := lnrpc.AddressType_UNUSED_WITNESS_PUBKEY_HASH
	if args[0] == "mweb" {
		addressType = lnrpc.AddressType_UNUSED_MWEB
	}
	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.NewAddress(context.Background(),
		&lnrpc.NewAddressRequest{Type: addressType})
	ts.Check(err)
	ts.Stdout().Write([]byte(resp.Address))
}

func getResult(ts *testscript.TestScript, neg bool, args []string) {
	envVar, args := args[0], args[1:]
	result := strings.TrimSpace(ts.ReadFile("stdout"))
	if len(args) > 0 {
		var data any
		ts.Check(json.Unmarshal([]byte(result), &data))
		for ; len(args) > 0; args = args[1:] {
			if i, err := strconv.Atoi(args[0]); err == nil {
				data = data.([]any)[i]
			} else {
				data = data.(map[string]any)[args[0]]
			}
		}
		result = data.(string)
	}
	ts.Setenv(envVar, result)
	ts.Stdout().Write([]byte(result))
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
		block, err := strconv.Atoi(args[0])
		ts.Check(err)
		if resp.BlockHeight == uint32(block) && resp.SyncedToChain {
			break
		}
	}
}

func waitForUtxos(ts *testscript.TestScript, neg bool, args []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for ; ; time.Sleep(time.Second) {
		if deadline, _ := ctx.Deadline(); time.Now().After(deadline) {
			ts.Fatalf("waiting for utxos timed out")
		}
		client := walletrpc.NewWalletKitClient(rpcConn)
		resp, err := client.ListUnspent(ctx,
			&walletrpc.ListUnspentRequest{MaxConfs: 1})
		ts.Check(err)
		count, err := strconv.Atoi(args[0])
		ts.Check(err)
		if len(resp.Utxos) == count {
			break
		}
	}
}

func checkUtxo(ts *testscript.TestScript, neg bool, args []string) {
	client := walletrpc.NewWalletKitClient(rpcConn)
	resp, err := client.ListUnspent(context.Background(),
		&walletrpc.ListUnspentRequest{MaxConfs: 1})
	ts.Check(err)
	index, err := strconv.Atoi(args[0])
	ts.Check(err)

	slices.SortFunc(resp.Utxos, func(a, b *lnrpc.Utxo) int {
		return cmp.Compare(a.AmountSat, b.AmountSat)
	})
	if resp.Utxos[index].Address != args[1] {
		ts.Fatalf("utxo address mismatch")
	}
	value, err := strconv.ParseFloat(args[2], 64)
	ts.Check(err)
	if resp.Utxos[index].AmountSat != int64(value*ltcutil.SatoshiPerBitcoin) {
		ts.Fatalf("utxo value mismatch")
	}
}

func checkTxnHistory(ts *testscript.TestScript, neg bool, args []string) {
	startHeight, err := strconv.Atoi(args[0])
	ts.Check(err)
	index, err := strconv.Atoi(args[1])
	ts.Check(err)
	value, err := strconv.ParseFloat(args[3], 64)
	ts.Check(err)
	fee, err := strconv.ParseFloat(args[4], 64)
	ts.Check(err)
	addresses := strings.Split(args[5], ",")
	slices.Sort(addresses)

	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.GetTransactions(context.Background(),
		&lnrpc.GetTransactionsRequest{StartHeight: int32(startHeight)})
	ts.Check(err)
	tx := resp.Transactions[index]
	if args[2] != "" && tx.TxHash != args[2] {
		ts.Fatalf("txn hash mismatch")
	}
	if tx.Amount != int64(value*ltcutil.SatoshiPerBitcoin) {
		ts.Fatalf("txn value mismatch")
	}
	if tx.TotalFees != int64(fee*ltcutil.SatoshiPerBitcoin) {
		ts.Fatalf("txn fee mismatch")
	}
	slices.Sort(tx.DestAddresses)
	if !slices.Equal(tx.DestAddresses, addresses) {
		ts.Fatalf("txn addresses mismatch")
	}
}

func sendCoins(ts *testscript.TestScript, neg bool, args []string) {
	value, err := strconv.Atoi(args[1])
	ts.Check(err)
	client := lnrpc.NewLightningClient(rpcConn)
	resp, err := client.SendCoins(context.Background(),
		&lnrpc.SendCoinsRequest{Addr: args[0],
			Amount: int64(value) * ltcutil.SatoshiPerBitcoin})
	ts.Check(err)
	ts.Stdout().Write([]byte(resp.Txid))
}
