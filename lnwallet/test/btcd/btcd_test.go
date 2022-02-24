package btcd_test

import (
	"testing"

	lnwallettest "github.com/ltcsuite/lnd/lnwallet/test"
)

// TestLightningWallet tests LightningWallet powered by btcd against our suite
// of interface tests.
func TestLightningWallet(t *testing.T) {
	lnwallettest.TestLightningWallet(t, "btcd")
}
