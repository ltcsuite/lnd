package chainreg

import (
	"encoding/json"

	"github.com/ltcsuite/ltcd/rpcclient"
)

// backendSupportsTaproot returns true if the backend understands the taproot
// soft fork.
func backendSupportsTaproot(rpc *rpcclient.Client) bool {
	// First, we'll try to access the normal getblockchaininfo call.
	chainInfo, err := rpc.GetBlockChainInfo()
	if err == nil {
		// If this call worked, then we'll check that the taproot
		// deployment is defined.
		switch {
		// litecoind versions before 0.21 and also ltcd use the
		// SoftForks fields.
		case chainInfo.SoftForks != nil:
			_, ok := chainInfo.SoftForks.Bip9SoftForks["taproot"]
			if ok {
				return ok
			}

		// Litecoind versions after 0.21 will use the UnifiedSoftForks
		// field that factors in the set of "buried" soft forks.
		case chainInfo.UnifiedSoftForks != nil:
			_, ok := chainInfo.UnifiedSoftForks.SoftForks["taproot"]
			if ok {
				return ok
			}
		}
	}

	// The user might be running a newer version of litecoind that doesn't
	// implement the getblockchaininfo call any longer, so we'll fall back
	// here.
	//
	// Alternatively, the fork wasn't specified, but the user might be
	// running a newer version of litecoind that still has the
	// getblockchaininfo call, but doesn't populate the data, so we'll hit
	// the new getdeploymentinfo call.
	resp, err := rpc.RawRequest("getdeploymentinfo", nil)
	if err != nil {
		log.Warnf("unable to make getdeploymentinfo request: %v", err)
		return false
	}

	info := struct {
		Deployments map[string]struct {
			Type   string `json:"type"`
			Active bool   `json:"active"`
			Height int32  `json:"height"`
		} `json:"deployments"`
	}{}
	if err := json.Unmarshal(resp, &info); err != nil {
		log.Warnf("unable to decode getdeploymentinfo resp: %v", err)
		return false
	}

	_, ok := info.Deployments["taproot"]
	return ok
}
