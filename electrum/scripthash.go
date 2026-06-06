package electrum

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/txscript"
	"github.com/ltcsuite/ltcwallet/chain"
)

// ScripthashFromScript converts a pkScript (output script) to an Electrum
// scripthash. The scripthash is the SHA256 hash of the script with the bytes
// reversed (displayed in little-endian order).
func ScripthashFromScript(pkScript []byte) string {
	hash := sha256.Sum256(pkScript)

	// Reverse the hash bytes for Electrum's format.
	reversed := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		reversed[i] = hash[len(hash)-1-i]
	}

	return hex.EncodeToString(reversed)
}

// ScripthashFromAddress converts a Bitcoin address to an Electrum scripthash.
// This creates the appropriate pkScript for the address type and then computes
// the scripthash.
func ScripthashFromAddress(address string,
	params *chaincfg.Params) (string, error) {

	addr, err := ltcutil.DecodeAddress(address, params)
	if err != nil {
		return "", fmt.Errorf("failed to decode address: %w", err)
	}

	pkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return "", fmt.Errorf("failed to create pkScript: %w", err)
	}

	return ScripthashFromScript(pkScript), nil
}

// ScripthashFromAddressUnchecked converts a Bitcoin address to an Electrum
// scripthash without network validation. This is useful when the network
// parameters are not available but the address format is known to be valid.
func ScripthashFromAddressUnchecked(address string) (string, error) {
	// Try mainnet first, then testnet, then regtest.
	networks := []*chaincfg.Params{
		&chaincfg.MainNetParams,
		&chaincfg.TestNet4Params,
		&chaincfg.RegressionNetParams,
		&chaincfg.SigNetParams,
	}

	for _, params := range networks {
		scripthash, err := ScripthashFromAddress(address, params)
		if err == nil {
			return scripthash, nil
		}
	}

	return "", fmt.Errorf("failed to decode address on any network: %s",
		address)
}

// ReverseBytes reverses a byte slice in place and returns it.
func ReverseBytes(b []byte) []byte {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// ReversedHash returns a copy of the hash with bytes reversed. This is useful
// for converting between internal byte order and display order.
func ReversedHash(hash []byte) []byte {
	reversed := make([]byte, len(hash))
	for i := 0; i < len(hash); i++ {
		reversed[i] = hash[len(hash)-1-i]
	}
	return reversed
}

// electrumStatus computes the Electrum status for the set of transactions
// touching an address, matching the value an ElectrumX server reports for
// blockchain.scripthash.subscribe: the lowercase-hex SHA256 over
// "<txid>:<height>:" for each transaction, ordered by block height ascending
// with unconfirmed transactions last. An empty set yields the empty string,
// mirroring the server's null status for an address with no history.
//
// Unconfirmed transactions are emitted at height 0 (the committed history does
// not record the server's 0-vs-(-1) parent distinction), and same-block
// transactions keep the caller's order. Either divergence only costs a
// redundant fetch, never a missed transaction.
func electrumStatus(txs []chain.AddressTx) string {
	if len(txs) == 0 {
		return ""
	}

	ordered := make([]chain.AddressTx, len(txs))
	copy(ordered, txs)
	sort.SliceStable(ordered, func(i, j int) bool {
		return statusOrder(ordered[i].Height) < statusOrder(ordered[j].Height)
	})

	var sb strings.Builder
	for _, tx := range ordered {
		height := tx.Height
		if height < 0 {
			height = 0
		}
		sb.WriteString(tx.TxID.String())
		sb.WriteByte(':')
		sb.WriteString(strconv.Itoa(int(height)))
		sb.WriteByte(':')
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])
}

// statusOrder maps a transaction height to its sort position within an Electrum
// status string. Confirmed transactions ascend by height; unconfirmed ones
// (height <= 0) sort after every confirmed one.
func statusOrder(height int32) int64 {
	if height > 0 {
		return int64(height)
	}
	return 1 << 40
}
