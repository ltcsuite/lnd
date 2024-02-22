package bitcoind_test

import (
	"testing"

	lnwallettest "github.com/ltcsuite/lnd/lnwallet/test"
)

// TestLightningWalletBitcoindZMQ tests LightningWallet powered by litecoind,
// using its ZMQ interface, against our suite of interface tests.
func TestLightningWalletBitcoindZMQ(t *testing.T) {
	lnwallettest.TestLightningWallet(t, "litecoind")
}

// TestLightningWalletBitcoindRPCPolling tests LightningWallet powered by
// litecoind, using its RPC interface, against our suite of interface tests.
func TestLightningWalletBitcoindRPCPolling(t *testing.T) {
	lnwallettest.TestLightningWallet(t, "litecoind-rpc-polling")
}
