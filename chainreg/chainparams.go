package chainreg

import (
	"github.com/ltcsuite/lnd/keychain"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/wire"
)

// LitecoinNetParams couples the p2p parameters of a network with the
// corresponding RPC port of a daemon running on the particular network.
type LitecoinNetParams struct {
	*chaincfg.Params
	RPCPort  string
	CoinType uint32
}

// LitecoinSimNetParams contains parameters specific to the simulation test
// network.
var LitecoinSimNetParams = LitecoinNetParams{
	Params:   &chaincfg.TestNet4Params,
	RPCPort:  "19556",
	CoinType: keychain.CoinTypeTestnet,
}

// LitecoinTestNetParams contains parameters specific to the 4th version of the
// test network.
var LitecoinTestNetParams = LitecoinNetParams{
	Params:   &chaincfg.TestNet4Params,
	RPCPort:  "19334",
	CoinType: keychain.CoinTypeTestnet,
}

// LitecoinMainNetParams contains the parameters specific to the current
// Litecoin mainnet.
var LitecoinMainNetParams = LitecoinNetParams{
	Params:   &chaincfg.MainNetParams,
	RPCPort:  "9334",
	CoinType: keychain.CoinTypeLitecoin,
}

// LitecoinRegTestNetParams contains parameters specific to a local litecoin
// regtest network.
var LitecoinRegTestNetParams = LitecoinNetParams{
	Params:   &chaincfg.RegressionNetParams,
	RPCPort:  "18334",
	CoinType: keychain.CoinTypeTestnet,
}

// IsTestnet tests if the givern params correspond to a testnet
// parameter configuration.
func IsTestnet(params *LitecoinNetParams) bool {
	switch params.Params.Net {
	case wire.TestNet4:
		return true
	default:
		return false
	}
}
