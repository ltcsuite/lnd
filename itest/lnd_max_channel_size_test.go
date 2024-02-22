package itest

import (
	"fmt"

	"github.com/ltcsuite/lnd/funding"
	"github.com/ltcsuite/lnd/lntest"
	"github.com/ltcsuite/lnd/lnwallet"
	"github.com/ltcsuite/ltcd/ltcutil"
)

// testMaxChannelSize tests that lnd handles --maxchansize parameter correctly.
// Wumbo nodes should enforce a default soft limit of 600 LTC by default. This
// limit can be adjusted with --maxchansize config option.
func testMaxChannelSize(ht *lntest.HarnessTest) {
	// We'll make two new nodes, both wumbo but with the default limit on
	// maximum channel size (600 LTC)
	wumboNode := ht.NewNode(
		"wumbo", []string{"--protocol.wumbo-channels"},
	)
	wumboNode2 := ht.NewNode(
		"wumbo2", []string{"--protocol.wumbo-channels"},
	)

	// We'll send 601 BTC to the wumbo node so it can test the wumbo soft
	// limit.
	ht.FundCoins(601*ltcutil.SatoshiPerBitcoin, wumboNode)

	// Next we'll connect both nodes, then attempt to make a wumbo channel
	// funding request, which should fail as it exceeds the default wumbo
	// soft limit of 600 LTC.
	ht.EnsureConnected(wumboNode, wumboNode2)

	chanAmt := funding.MaxLtcFundingAmountWumbo + 1
	// The test should show failure due to the channel exceeding our max
	// size.
	expectedErr := lnwallet.ErrChanTooLarge(
		chanAmt, funding.MaxLtcFundingAmountWumbo,
	)
	ht.OpenChannelAssertErr(
		wumboNode, wumboNode2,
		lntest.OpenChannelParams{Amt: chanAmt}, expectedErr,
	)

	// We'll now make another wumbo node with appropriate maximum channel
	// size to accept our wumbo channel funding.
	wumboNode3 := ht.NewNode(
		"wumbo3", []string{
			"--protocol.wumbo-channels",
			fmt.Sprintf(
				"--maxchansize=%v",
				int64(funding.MaxLtcFundingAmountWumbo+1),
			),
		},
	)

	// Creating a wumbo channel between these two nodes should succeed.
	ht.EnsureConnected(wumboNode, wumboNode3)
	chanPoint := ht.OpenChannel(
		wumboNode, wumboNode3, lntest.OpenChannelParams{Amt: chanAmt},
	)

	ht.CloseChannel(wumboNode, chanPoint)
}
