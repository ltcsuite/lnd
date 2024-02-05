package mweb_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ltcsuite/lnd/lnrpc"
	"github.com/ltcsuite/lnd/lnrpc/walletrpc"
	"github.com/ltcsuite/lnd/macaroons"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/rogpeppe/go-internal/testscript"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"gopkg.in/macaroon.v2"
)

const (
	lndRpcPort        = 11009
	lndWalletPassword = "password"
)

func TestLitecoindLndInteraction(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir: "testdata",
		Setup: func(e *testscript.Env) error {
			e.Setenv("LND_RPC_PORT", strconv.Itoa(lndRpcPort))
			return nil
		},
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"openRpcConn":  openRpcConn,
			"createWallet": createWallet,
			"getResult":    getResult,
			"syncToBlock":  syncToBlock,
			"waitForUtxos": waitForUtxos,
			"checkUtxo":    checkUtxo,
		},
	})
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
		rpcConn, err = grpc.DialContext(ctx,
			fmt.Sprintf("127.0.0.1:%d", lndRpcPort), opts...)
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
	if resp.Utxos[index].Address != args[1] {
		ts.Fatalf("utxo address mismatch")
	}
	value, err := strconv.Atoi(args[2])
	ts.Check(err)
	if resp.Utxos[index].AmountSat != int64(value)*ltcutil.SatoshiPerBitcoin {
		ts.Fatalf("utxo value mismatch")
	}
}
