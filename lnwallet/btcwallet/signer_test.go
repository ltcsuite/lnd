package btcwallet

import (
	"encoding/hex"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/ltcsuite/lnd/blockcache"
	"github.com/ltcsuite/lnd/input"
	"github.com/ltcsuite/lnd/lnwallet"
	"github.com/ltcsuite/ltcd/btcec/v2"
	"github.com/ltcsuite/ltcd/btcec/v2/schnorr"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/integration/rpctest"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/hdkeychain"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/stretchr/testify/require"
)

var (
	// seedBytes is the raw entropy of the aezeed:
	//   able promote dizzy mixture sword myth share public find tattoo
	//   catalog cousin bulb unfair machine alarm cool large promote kick
	//   shop rug mean year
	// Which corresponds to the master root key:
	//   xprv9s21ZrQH143K2KADjED57FvNbptdKLp4sqKzssegwEGKQMGoDkbyhUeCKe5m3A
	//   MU44z4vqkmGswwQVKrv599nFG16PPZDEkNrogwoDGeCmZ
	seedBytes, _ = hex.DecodeString("4a7611b6979ba7c4bc5c5cd2239b2973")

	// firstAddress is the first address that we should get from the wallet,
	// corresponding to the derivation path m/84'/0'/0'/0/0 (even on regtest
	// which is a special case for the BIP49/84 addresses in btcwallet).
	firstAddress = "rltc1qlkp8904tzjyd8qwzjl550k040qz08ulaeajgcn"

	// firstAddressTaproot is the first address that we should get from the
	// wallet when deriving a taproot address.
	firstAddressTaproot = "rltc1ph4tspl0kuwvknu4525t8gue330u5p3fy2t2lj7ne" +
		"6zt7d49cucxs5yl37j"

	testPubKeyBytes, _ = hex.DecodeString(
		"037a67771635344641d4b56aac33cd5f7a265b59678dce3aec31b89125e3" +
			"b8b9b2",
	)
	testPubKey, _          = btcec.ParsePubKey(testPubKeyBytes)
	testTaprootKeyBytes, _ = hex.DecodeString(
		"03f068684c9141027318eed958dccbf4f7f748700e1da53315630d82a362" +
			"d6a887",
	)
	testTaprootKey, _ = btcec.ParsePubKey(testTaprootKeyBytes)

	testTapscriptAddr = "rltc1p7p5xsny3gyp8xx8wm9vdejl57lm5suqwrkjnx9trpk" +
		"p2xckk4zrsa9ngs2"
	testTapscriptPkScript = append(
		[]byte{txscript.OP_1, txscript.OP_DATA_32},
		schnorr.SerializePubKey(testTaprootKey)...,
	)

	testCases = []struct {
		name string
		path []uint32
		err  string
		wif  string
	}{{
		name: "m/84'/2'/0'/0/0",
		path: []uint32{
			hardenedKey(84), hardenedKey(2), hardenedKey(0), 0, 0,
		},
		wif: "cU4xcYTdcV7RJMo17AUYzTFLVZTSTWxDKS9xLjCcyUeYFe2z7uVr",
	}, {
		name: "m/84'/2'/0'/1/0",
		path: []uint32{
			hardenedKey(84), hardenedKey(2), hardenedKey(0), 1, 0,
		},
		wif: "cVB5fJvzvEexUySMYMAUUem3LJQiRCCqXBkgexGJfYCZgPChDfUk",
	}, {
		name: "m/84'/2'/0'/0/12345",
		path: []uint32{
			hardenedKey(84), hardenedKey(2), hardenedKey(0), 0,
			12345,
		},
		wif: "cMfjrTfDZDCH5TKfVF1inPdXkTUpcrfahPCJVFah3sxsSgW2d3cm",
	}, {
		name: "m/49'/2'/0'/0/0",
		path: []uint32{
			hardenedKey(49), hardenedKey(2), hardenedKey(0), 0, 0,
		},
		wif: "cPfwSUFdyGRQm7CNKoRC8oeHMvbruQSM1hvsJ1MPmUh7L3DJfXw7",
	}, {
		name: "m/49'/2'/0'/1/0",
		path: []uint32{
			hardenedKey(49), hardenedKey(2), hardenedKey(0), 1, 0,
		},
		wif: "cPJ2c2bQyWSf9FTUJHJ2mdNZB6pmXT16F48hDNcYNxThr1yanhAL",
	}, {
		name: "m/49'/2'/0'/1/12345",
		path: []uint32{
			hardenedKey(49), hardenedKey(2), hardenedKey(0), 1,
			12345,
		},
		wif: "cNX5cYueBJGpHdGv4dLS7tMwQpkV4iRVrSmeoomFZ2Zd2FQY1kNH",
	}, {
		name: "m/1017'/1'/0'/0/0",
		path: []uint32{
			hardenedKey(1017), hardenedKey(1), hardenedKey(0), 0, 0,
		},
		wif: "cPsCmbWQENgptj3eTiyd85QSAD1xqYKPM9jUkfvm7vgN3SoVPWSP",
	}, {
		name: "m/1017'/1'/6'/0/0",
		path: []uint32{
			hardenedKey(1017), hardenedKey(1), hardenedKey(6), 0, 0,
		},
		wif: "cPeQdpcGJmLqpdmvokh3DK9ZtjYAXxiw4p4ELNUWkWt6bMRqArEV",
	}, {
		name: "m/1017'/1'/7'/0/123",
		path: []uint32{
			hardenedKey(1017), hardenedKey(1), hardenedKey(7), 0,
			123,
		},
		wif: "cPcWZMqY4YErkcwjtFJaYoXkzd7bKxrfxAVzhDgy3n5BGH8CU8sn",
	}, {
		name: "m/84'/0'/0'/0/0",
		path: []uint32{
			hardenedKey(84), hardenedKey(0), hardenedKey(0), 0, 0,
		},
		err: "coin type must be 2 for BIP49/84 ltcwallet keys",
	}, {
		name: "m/84'/1'/0'/0/0",
		path: []uint32{
			hardenedKey(84), hardenedKey(1), hardenedKey(0), 0, 0,
		},
		err: "coin type must be 2 for BIP49/84 ltcwallet keys",
	}, {
		name: "m/1017'/0'/0'/0/0",
		path: []uint32{
			hardenedKey(1017), hardenedKey(0), hardenedKey(0), 0, 0,
		},
		err: "expected coin type 1, instead was 0",
	}, {
		name: "m/84'/2'/1'/0/0",
		path: []uint32{
			hardenedKey(84), hardenedKey(2), hardenedKey(1), 0, 0,
		},
		err: "account 1 not found",
	}, {
		name: "m/49'/2'/1'/0/0",
		path: []uint32{
			hardenedKey(49), hardenedKey(2), hardenedKey(1), 0, 0,
		},
		err: "account 1 not found",
	}, {
		name: "non-hardened purpose m/84/2/0/0/0",
		path: []uint32{84, 2, 0, 0, 0},
		err:  "element at index 0 is not hardened",
	}, {
		name: "non-hardened account m/84'/2'/0/0/0",
		path: []uint32{hardenedKey(84), hardenedKey(2), 0, 0, 0},
		err:  "element at index 2 is not hardened",
	},
	}
)

// TestBip32KeyDerivation makes sure that private keys can be derived from a
// BIP32 key path correctly.
func TestBip32KeyDerivation(t *testing.T) {
	netParams := &chaincfg.RegressionNetParams
	w, _ := newTestWallet(t, netParams, seedBytes)

	// This is just a sanity check that the wallet was initialized
	// correctly. We make sure the first derived address is the expected
	// one.
	firstDerivedAddr, err := w.NewAddress(
		lnwallet.WitnessPubKey, false, lnwallet.DefaultAccountName,
	)
	require.NoError(t, err)
	require.Equal(t, firstAddress, firstDerivedAddr.String())

	// Let's go through the test cases now that we know our wallet is ready.
	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			privKey, err := w.deriveKeyByBIP32Path(tc.path)

			if tc.err == "" {
				require.NoError(t, err)
				wif, err := ltcutil.NewWIF(
					privKey, netParams, true,
				)
				require.NoError(t, err)
				require.Equal(t, tc.wif, wif.String())
			} else {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.err)
			}
		})
	}
}

// TestScriptImport tests the btcwallet's tapscript import capabilities by
// importing both a full taproot script tree and a partially revealed branch
// with a proof to make sure the resulting addresses match up.
func TestScriptImport(t *testing.T) {
	netParams := &chaincfg.RegressionNetParams
	w, miner := newTestWallet(t, netParams, seedBytes)

	firstDerivedAddr, err := w.NewAddress(
		lnwallet.TaprootPubkey, false, lnwallet.DefaultAccountName,
	)
	require.NoError(t, err)
	require.Equal(t, firstAddressTaproot, firstDerivedAddr.String())

	scope := waddrmgr.KeyScopeBIP0086
	_, err = w.InternalWallet().Manager.FetchScopedKeyManager(scope)
	require.NoError(t, err)

	// Let's create a taproot script output now. This is a hash lock with a
	// simple preimage of "foobar".
	builder := txscript.NewScriptBuilder()
	builder.AddOp(txscript.OP_DUP)
	builder.AddOp(txscript.OP_HASH160)
	builder.AddData(ltcutil.Hash160([]byte("foobar")))
	builder.AddOp(txscript.OP_EQUALVERIFY)
	script1, err := builder.Script()
	require.NoError(t, err)
	leaf1 := txscript.NewBaseTapLeaf(script1)

	// Let's add a second script output as well to test the partial reveal.
	builder = txscript.NewScriptBuilder()
	builder.AddData(schnorr.SerializePubKey(testPubKey))
	builder.AddOp(txscript.OP_CHECKSIG)
	script2, err := builder.Script()
	require.NoError(t, err)
	leaf2 := txscript.NewBaseTapLeaf(script2)

	// Our first test case is storing the script with all its leaves.
	tapscript1 := input.TapscriptFullTree(testPubKey, leaf1, leaf2)

	taprootKey1, err := tapscript1.TaprootKey()
	require.NoError(t, err)
	require.Equal(
		t, testTaprootKey.SerializeCompressed(),
		taprootKey1.SerializeCompressed(),
	)

	addr1, err := w.ImportTaprootScript(scope, tapscript1)
	require.NoError(t, err)

	require.Equal(t, testTapscriptAddr, addr1.Address().String())
	pkScript, err := txscript.PayToAddrScript(addr1.Address())
	require.NoError(t, err)
	require.Equal(t, testTapscriptPkScript, pkScript)

	// Send some coins to the taproot address now and wait until they are
	// seen as unconfirmed.
	_, err = miner.SendOutputs([]*wire.TxOut{{
		Value:    ltcutil.SatoshiPerBitcoin,
		PkScript: pkScript,
	}}, 1)
	require.NoError(t, err)

	var utxos []*lnwallet.Utxo
	require.Eventually(t, func() bool {
		utxos, err = w.ListUnspentWitness(0, math.MaxInt32, "")
		require.NoError(t, err)

		return len(utxos) == 1
	}, time.Minute, 50*time.Millisecond)
	require.Equal(t, testTapscriptPkScript, utxos[0].PkScript)

	// Now, as a last test, make sure that when we try adding an address
	// with partial script reveal, we get an error that the address already
	// exists.
	inclusionProof := leaf2.TapHash()
	tapscript2 := input.TapscriptPartialReveal(
		testPubKey, leaf1, inclusionProof[:],
	)
	_, err = w.ImportTaprootScript(scope, tapscript2)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf(
		"address for script hash/key %x already exists",
		schnorr.SerializePubKey(testTaprootKey),
	))
}

func newTestWallet(t *testing.T, netParams *chaincfg.Params,
	seedBytes []byte) (*BtcWallet, *rpctest.Harness) {

	chainBackend, miner, backendCleanup := getChainBackend(t, netParams)
	t.Cleanup(backendCleanup)

	loaderOpt := LoaderWithLocalWalletDB(t.TempDir(), false, time.Minute)
	config := Config{
		PrivatePass: []byte("some-pass"),
		HdSeed:      seedBytes,
		NetParams:   netParams,
		CoinType:    netParams.HDCoinType,
		ChainSource: chainBackend,
		// wallet starts in recovery mode
		RecoveryWindow: 2,
		LoaderOptions:  []LoaderOption{loaderOpt},
	}
	blockCache := blockcache.NewBlockCache(10000)
	w, err := New(config, blockCache)
	if err != nil {
		t.Fatalf("creating wallet failed: %v", err)
	}

	err = w.Start()
	if err != nil {
		t.Fatalf("starting wallet failed: %v", err)
	}

	return w, miner
}

// getChainBackend returns a simple btcd based chain backend to back the wallet.
func getChainBackend(t *testing.T, netParams *chaincfg.Params) (chain.Interface,
	*rpctest.Harness, func()) {

	miningNode, err := rpctest.New(netParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, miningNode.SetUp(true, 25))

	// Next, mine enough blocks in order for SegWit and the CSV package
	// soft-fork to activate on RegNet.
	numBlocks := netParams.MinerConfirmationWindow * 2
	_, err = miningNode.Client.Generate(numBlocks)
	require.NoError(t, err)

	rpcConfig := miningNode.RPCConfig()
	chainClient, err := chain.NewRPCClient(
		netParams, rpcConfig.Host, rpcConfig.User, rpcConfig.Pass,
		rpcConfig.Certificates, false, 20,
	)
	require.NoError(t, err)

	return chainClient, miningNode, func() {
		_ = miningNode.TearDown()
	}
}

// hardenedKey returns a key of a hardened derivation key path.
func hardenedKey(part uint32) uint32 {
	return part + hdkeychain.HardenedKeyStart
}
