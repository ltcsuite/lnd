package peersrpc

import (
	"github.com/btcsuite/btclog"
	"github.com/ltcsuite/lnd/build"
)

// log is a logger that is initialized with no output filters. This means the
// package will not perform any logging by default until the caller requests
// it.
var log btclog.Logger

// Subsystem defines the logging code for this subsystem.
const Subsystem = "PRPC"

// The default amount of logging is none.
func init() {
	UseLogger(build.NewSubLogger(Subsystem, nil))
}

// DisableLog disables all library log output.  Logging output is disabled by
// by default until UseLogger is called.
func DisableLog() {
	UseLogger(btclog.Disabled)
}

// UseLogger uses a specified Logger to output package logging info. This
// should be used in preference to SetLogWriter if the caller is also using
// btclog.
func UseLogger(logger btclog.Logger) {
	log = logger
}
