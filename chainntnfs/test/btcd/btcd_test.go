//go:build dev
// +build dev

package btcd_test

import (
	"testing"

	chainntnfstest "github.com/ltcsuite/lnd/chainntnfs/test"
)

// TestInterfaces executes the generic notifier test suite against a ltcd
// powered chain notifier.
func TestInterfaces(t *testing.T) {
	chainntnfstest.TestInterfaces(t, "ltcd")
}
