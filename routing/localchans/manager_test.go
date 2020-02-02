package localchans

import (
	"testing"

	"github.com/ltcsuite/lnd/lnwire"

	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcutil"

	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/lnd/channeldb"
	"github.com/ltcsuite/lnd/discovery"
	"github.com/ltcsuite/lnd/htlcswitch"
	"github.com/ltcsuite/lnd/routing"
)

// TestManager tests that the local channel manager properly propagates fee
// updates to gossiper and links.
func TestManager(t *testing.T) {
	var (
		chanPoint        = wire.OutPoint{Hash: chainhash.Hash{1}, Index: 2}
		chanCap          = ltcutil.Amount(1000)
		maxPendingAmount = lnwire.MilliSatoshi(999000)
		minHTLC          = lnwire.MilliSatoshi(2000)
	)

	newPolicy := routing.ChannelPolicy{
		FeeSchema: routing.FeeSchema{
			BaseFee: 100,
			FeeRate: 200,
		},
		TimeLockDelta: 80,
		MaxHTLC:       5000,
	}

	currentPolicy := channeldb.ChannelEdgePolicy{
		MinHTLC:      minHTLC,
		MessageFlags: lnwire.ChanUpdateOptionMaxHtlc,
	}

	updateForwardingPolicies := func(
		chanPolicies map[wire.OutPoint]htlcswitch.ForwardingPolicy) {

		if len(chanPolicies) != 1 {
			t.Fatal("unexpected number of policies to apply")
		}

		policy := chanPolicies[chanPoint]
		if policy.TimeLockDelta != newPolicy.TimeLockDelta {
			t.Fatal("unexpected time lock delta")
		}
		if policy.BaseFee != newPolicy.BaseFee {
			t.Fatal("unexpected base fee")
		}
		if uint32(policy.FeeRate) != newPolicy.FeeRate {
			t.Fatal("unexpected base fee")
		}
		if policy.MaxHTLC != newPolicy.MaxHTLC {
			t.Fatal("unexpected max htlc")
		}
	}

	propagateChanPolicyUpdate := func(
		edgesToUpdate []discovery.EdgeWithInfo) error {

		if len(edgesToUpdate) != 1 {
			t.Fatal("unexpected number of edges to update")
		}

		policy := edgesToUpdate[0].Edge
		if !policy.MessageFlags.HasMaxHtlc() {
			t.Fatal("expected max htlc flag")
		}
		if policy.TimeLockDelta != uint16(newPolicy.TimeLockDelta) {
			t.Fatal("unexpected time lock delta")
		}
		if policy.FeeBaseMSat != newPolicy.BaseFee {
			t.Fatal("unexpected base fee")
		}
		if uint32(policy.FeeProportionalMillionths) != newPolicy.FeeRate {
			t.Fatal("unexpected base fee")
		}
		if policy.MaxHTLC != newPolicy.MaxHTLC {
			t.Fatal("unexpected max htlc")
		}

		return nil
	}

	forAllOutgoingChannels := func(cb func(*channeldb.ChannelEdgeInfo,
		*channeldb.ChannelEdgePolicy) error) error {

		return cb(
			&channeldb.ChannelEdgeInfo{
				Capacity:     chanCap,
				ChannelPoint: chanPoint,
			},
			&currentPolicy,
		)
	}

	fetchChannel := func(chanPoint wire.OutPoint) (*channeldb.OpenChannel,
		error) {

		constraints := channeldb.ChannelConstraints{
			MaxPendingAmount: maxPendingAmount,
			MinHTLC:          minHTLC,
		}

		return &channeldb.OpenChannel{
			LocalChanCfg: channeldb.ChannelConfig{
				ChannelConstraints: constraints,
			},
		}, nil
	}

	manager := Manager{
		UpdateForwardingPolicies:  updateForwardingPolicies,
		PropagateChanPolicyUpdate: propagateChanPolicyUpdate,
		ForAllOutgoingChannels:    forAllOutgoingChannels,
		FetchChannel:              fetchChannel,
	}

	// Test updating a specific channels.
	err := manager.UpdatePolicy(newPolicy, chanPoint)
	if err != nil {
		t.Fatal(err)
	}

	// Test updating all channels, which comes down to the same as testing a
	// specific channel because there is only one channel.
	err = manager.UpdatePolicy(newPolicy)
	if err != nil {
		t.Fatal(err)
	}

	// If no max htlc is specified, the max htlc value should be kept
	// unchanged.
	currentPolicy.MaxHTLC = newPolicy.MaxHTLC
	noMaxHtlcPolicy := newPolicy
	noMaxHtlcPolicy.MaxHTLC = 0

	err = manager.UpdatePolicy(noMaxHtlcPolicy)
	if err != nil {
		t.Fatal(err)
	}
}
