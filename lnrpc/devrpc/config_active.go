//go:build dev
// +build dev

package devrpc

import (
	"github.com/ltcsuite/lnd/channeldb"
	"github.com/ltcsuite/ltcd/chaincfg"
)

// Config is the primary configuration struct for the DEV RPC server. It
// contains all the items required for the rpc server to carry out its
// duties. Any fields with struct tags are meant to be parsed as normal
// configuration options, while if able to be populated, the latter fields MUST
// also be specified.
type Config struct {
	ActiveNetParams *chaincfg.Params
	GraphDB         *channeldb.ChannelGraph
}
