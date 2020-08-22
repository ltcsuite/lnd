package autopilot

import (
	"testing"
	"time"

	prand "math/rand"

	"github.com/ltcsuite/lnd/lnwire"
	"github.com/ltcsuite/ltcutil"
)

func TestConstraintsChannelBudget(t *testing.T) {
	t.Parallel()

	prand.Seed(time.Now().Unix())

	const (
		minChanSize = 0
		maxChanSize = ltcutil.Amount(ltcutil.SatoshiPerBitcoin)

		chanLimit = 3

		threshold = 0.5
	)

	constraints := NewConstraints(
		minChanSize,
		maxChanSize,
		chanLimit,
		0,
		threshold,
	)

	randChanID := func() lnwire.ShortChannelID {
		return lnwire.NewShortChanIDFromInt(uint64(prand.Int63()))
	}

	testCases := []struct {
		channels  []Channel
		walletAmt ltcutil.Amount

		needMore     bool
		amtAvailable ltcutil.Amount
		numMore      uint32
	}{
		// Many available funds, but already have too many active open
		// channels.
		{
			[]Channel{
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(prand.Int31()),
				},
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(prand.Int31()),
				},
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(prand.Int31()),
				},
			},
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 10),
			false,
			0,
			0,
		},

		// Ratio of funds in channels and total funds meets the
		// threshold.
		{
			[]Channel{
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
			},
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 2),
			false,
			0,
			0,
		},

		// Ratio of funds in channels and total funds is below the
		// threshold. We have 10 BTC allocated amongst channels and
		// funds, atm. We're targeting 50%, so 5 BTC should be
		// allocated. Only 1 BTC is atm, so 4 BTC should be
		// recommended. We should also request 2 more channels as the
		// limit is 3.
		{
			[]Channel{
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
			},
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 9),
			true,
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 4),
			2,
		},

		// Ratio of funds in channels and total funds is below the
		// threshold. We have 14 BTC total amongst the wallet's
		// balance, and our currently opened channels. Since we're
		// targeting a 50% allocation, we should commit 7 BTC. The
		// current channels commit 4 BTC, so we should expected 3 BTC
		// to be committed. We should only request a single additional
		// channel as the limit is 3.
		{
			[]Channel{
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 3),
				},
			},
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 10),
			true,
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin * 3),
			1,
		},

		// Ratio of funds in channels and total funds is above the
		// threshold.
		{
			[]Channel{
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
				{
					ChanID:   randChanID(),
					Capacity: ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
				},
			},
			ltcutil.Amount(ltcutil.SatoshiPerBitcoin),
			false,
			0,
			0,
		},
	}

	for i, testCase := range testCases {
		amtToAllocate, numMore := constraints.ChannelBudget(
			testCase.channels, testCase.walletAmt,
		)

		if amtToAllocate != testCase.amtAvailable {
			t.Fatalf("test #%v: expected %v, got %v",
				i, testCase.amtAvailable, amtToAllocate)
		}
		if numMore != testCase.numMore {
			t.Fatalf("test #%v: expected %v, got %v",
				i, testCase.numMore, numMore)
		}
	}
}
