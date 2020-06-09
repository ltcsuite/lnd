package channeldb

import (
	"github.com/btcsuite/btclog"
	"github.com/ltcsuite/lnd/build"
	"github.com/ltcsuite/lnd/channeldb/migration12"
	"github.com/ltcsuite/lnd/channeldb/migration13"
	"github.com/ltcsuite/lnd/channeldb/migration_01_to_11"
)

// log is a logger that is initialized with no output filters.  This
// means the package will not perform any logging by default until the caller
// requests it.
var log btclog.Logger

func init() {
	UseLogger(build.NewSubLogger("CHDB", nil))
}

// DisableLog disables all library log output.  Logging output is disabled
// by default until UseLogger is called.
func DisableLog() {
	UseLogger(btclog.Disabled)
}

// UseLogger uses a specified Logger to output package logging info.
// This should be used in preference to SetLogWriter if the caller is also
// using btclog.
func UseLogger(logger btclog.Logger) {
	log = logger
	migration_01_to_11.UseLogger(logger)
	migration12.UseLogger(logger)
	migration13.UseLogger(logger)
}
