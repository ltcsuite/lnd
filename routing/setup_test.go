package routing

import (
	"testing"

	"github.com/ltcsuite/lnd/kvdb"
)

func TestMain(m *testing.M) {
	kvdb.RunTests(m)
}
