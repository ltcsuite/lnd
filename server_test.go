package lnd

import (
	"testing"

	"github.com/ltcsuite/lnd/lncfg"
)

// TestShouldPeerBootstrap tests that we properly skip network bootstrap for
// the developer networks, and also if bootstrapping is explicitly disabled.
func TestShouldPeerBootstrap(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		cfg            *Config
		shouldBoostrap bool
	}{
		// Simnet active, no bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					SimNet: true,
				},
			},
		},

		// Regtest active, no bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					RegTest: true,
				},
			},
		},

		// Signet active, no bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					SigNet: true,
				},
			},
		},

		// Mainnet active, but bootstrap disabled, no bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					MainNet: true,
				},
				NoNetBootstrap: true,
			},
		},

		// Mainnet active, should bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					MainNet: true,
				},
			},
			shouldBoostrap: true,
		},

		// Testnet active, should bootstrap.
		{
			cfg: &Config{
				Litecoin: &lncfg.Chain{
					TestNet4: true,
				},
			},
			shouldBoostrap: true,
		},
	}
	for i, testCase := range testCases {
		bootstrapped := shouldPeerBootstrap(testCase.cfg)
		if bootstrapped != testCase.shouldBoostrap {
			t.Fatalf("#%v: expected bootstrap=%v, got bootstrap=%v",
				i, testCase.shouldBoostrap, bootstrapped)
		}
	}
}
