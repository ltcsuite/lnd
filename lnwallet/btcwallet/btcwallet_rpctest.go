//go:build rpctest || lowscrypt
// +build rpctest lowscrypt

package btcwallet

import (
	"github.com/ltcsuite/ltcwallet/snacl"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
)

func init() {
	// Instruct waddrmgr to use the cranked down scrypt parameters when
	// creating new wallet encryption keys. This will speed up the itests
	// considerably.
	fastScrypt := waddrmgr.FastScryptOptions
	keyGen := func(passphrase *[]byte, config *waddrmgr.ScryptOptions) (
		*snacl.SecretKey, error) {

		return snacl.NewSecretKey(
			passphrase, fastScrypt.N, fastScrypt.R, fastScrypt.P,
		)
	}
	waddrmgr.SetSecretKeyGen(keyGen)
}
