package lnwallet

import (
	"github.com/ltcsuite/ltcutil"
	"github.com/ltcsuite/ltcwallet/wallet/txrules"
	"github.com/lightningnetwork/lnd/input"
)

// DefaultDustLimit is used to calculate the dust HTLC amount which will be
// send to other node during funding process.
func DefaultDustLimit() ltcutil.Amount {
	return txrules.GetDustThreshold(input.P2WSHSize, txrules.DefaultRelayFeePerKb)
}
