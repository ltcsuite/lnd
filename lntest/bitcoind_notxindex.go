//go:build bitcoind && notxindex && !rpcpolling
// +build bitcoind,notxindex,!rpcpolling

package lntest

import (
	"github.com/ltcsuite/ltcd/chaincfg"
)

// NewBackend starts a litecoind node without the txindex enabled and returns a
// BitoindBackendConfig for that node.
func NewBackend(miner string, netParams *chaincfg.Params) (
	*BitcoindBackendConfig, func() error, error) {

	extraArgs := []string{
		"-debug",
		"-regtest",
		"-disablewallet",
	}

	return newBackend(miner, netParams, extraArgs, false)
}
