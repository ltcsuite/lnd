//go:build dev
// +build dev

package bitcoind_test

import (
	"testing"

	chainntnfstest "github.com/ltcsuite/lnd/chainntnfs/test"
)

// TestInterfaces executes the generic notifier test suite against a litecoind
// powered chain notifier.
func TestInterfaces(t *testing.T) {
	t.Run("litecoind", func(st *testing.T) {
		st.Parallel()
		chainntnfstest.TestInterfaces(st, "litecoind")
	})

	t.Run("litecoind rpc polling", func(st *testing.T) {
		st.Parallel()
		chainntnfstest.TestInterfaces(st, "litecoind-rpc-polling")
	})
}
