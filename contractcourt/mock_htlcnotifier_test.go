package contractcourt

import (
	"github.com/ltcsuite/lnd/channeldb"
	"github.com/ltcsuite/lnd/channeldb/models"
)

type mockHTLCNotifier struct {
	HtlcNotifier
}

func (m *mockHTLCNotifier) NotifyFinalHtlcEvent(key models.CircuitKey,
	info channeldb.FinalHtlcInfo) { //nolint:whitespace
}
