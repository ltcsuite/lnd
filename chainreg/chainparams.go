package chainreg

import (
	"github.com/ltcsuite/lnd/keychain"
	"github.com/ltcsuite/ltcd/chaincfg"
	bitcoinCfg "github.com/ltcsuite/ltcd/chaincfg" //TODO(losh)
	litecoinCfg "github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	bitcoinWire "github.com/ltcsuite/ltcd/wire" //TODO(losh)
	litecoinWire "github.com/ltcsuite/ltcd/wire"
)

// BitcoinNetParams couples the p2p parameters of a network with the
// corresponding RPC port of a daemon running on the particular network.
type BitcoinNetParams struct {
	*bitcoinCfg.Params
	RPCPort  string
	CoinType uint32
}

// LitecoinNetParams couples the p2p parameters of a network with the
// corresponding RPC port of a daemon running on the particular network.
type LitecoinNetParams struct {
	*litecoinCfg.Params
	RPCPort  string
	CoinType uint32
}

// BitcoinTestNetParams contains parameters specific to the 3rd version of the
// test network.
var BitcoinTestNetParams = BitcoinNetParams{
	Params:   &bitcoinCfg.TestNet4Params,
	RPCPort:  "18334",
	CoinType: keychain.CoinTypeTestnet,
}

// BitcoinMainNetParams contains parameters specific to the current Bitcoin
// mainnet.
var BitcoinMainNetParams = BitcoinNetParams{
	Params:   &bitcoinCfg.MainNetParams,
	RPCPort:  "8334",
	CoinType: keychain.CoinTypeBitcoin,
}

// BitcoinSimNetParams contains parameters specific to the simulation test
// network.
var BitcoinSimNetParams = BitcoinNetParams{
	Params:   &bitcoinCfg.SimNetParams,
	RPCPort:  "18556",
	CoinType: keychain.CoinTypeTestnet,
}

// BitcoinSigNetParams contains parameters specific to the signet test network.
var BitcoinSigNetParams = BitcoinNetParams{
	Params:   &bitcoinCfg.SigNetParams,
	RPCPort:  "38332",
	CoinType: keychain.CoinTypeTestnet,
}

// LitecoinSimNetParams contains parameters specific to the simulation test
// network.
var LitecoinSimNetParams = LitecoinNetParams{
	Params:   &litecoinCfg.TestNet4Params,
	RPCPort:  "18556",
	CoinType: keychain.CoinTypeTestnet,
}

// LitecoinTestNetParams contains parameters specific to the 4th version of the
// test network.
var LitecoinTestNetParams = LitecoinNetParams{
	Params:   &litecoinCfg.TestNet4Params,
	RPCPort:  "19334",
	CoinType: keychain.CoinTypeTestnet,
}

// LitecoinMainNetParams contains the parameters specific to the current
// Litecoin mainnet.
var LitecoinMainNetParams = LitecoinNetParams{
	Params:   &litecoinCfg.MainNetParams,
	RPCPort:  "9334",
	CoinType: keychain.CoinTypeLitecoin,
}

// LitecoinRegTestNetParams contains parameters specific to a local litecoin
// regtest network.
var LitecoinRegTestNetParams = LitecoinNetParams{
	Params:   &litecoinCfg.RegressionNetParams,
	RPCPort:  "18334",
	CoinType: keychain.CoinTypeTestnet,
}

// BitcoinRegTestNetParams contains parameters specific to a local bitcoin
// regtest network.
var BitcoinRegTestNetParams = BitcoinNetParams{
	Params:   &bitcoinCfg.RegressionNetParams,
	RPCPort:  "18334",
	CoinType: keychain.CoinTypeTestnet,
}

// ApplyLitecoinParams applies the relevant chain configuration parameters that
// differ for litecoin to the chain parameters typed for btcsuite derivation.
// This function is used in place of using something like interface{} to
// abstract over _which_ chain (or fork) the parameters are for.
func ApplyLitecoinParams(params *LitecoinNetParams,
	litecoinParams *LitecoinNetParams) {

	params.Name = litecoinParams.Name
	params.Net = bitcoinWire.BitcoinNet(litecoinParams.Net)
	params.DefaultPort = litecoinParams.DefaultPort
	params.CoinbaseMaturity = litecoinParams.CoinbaseMaturity

	copy(params.GenesisHash[:], litecoinParams.GenesisHash[:])
	copy(params.GenesisBlock.Header.MerkleRoot[:],
		litecoinParams.GenesisBlock.Header.MerkleRoot[:])
	params.GenesisBlock.Header.Version =
		litecoinParams.GenesisBlock.Header.Version
	params.GenesisBlock.Header.Timestamp =
		litecoinParams.GenesisBlock.Header.Timestamp
	params.GenesisBlock.Header.Bits =
		litecoinParams.GenesisBlock.Header.Bits
	params.GenesisBlock.Header.Nonce =
		litecoinParams.GenesisBlock.Header.Nonce
	params.GenesisBlock.Transactions[0].Version =
		litecoinParams.GenesisBlock.Transactions[0].Version
	params.GenesisBlock.Transactions[0].LockTime =
		litecoinParams.GenesisBlock.Transactions[0].LockTime
	params.GenesisBlock.Transactions[0].TxIn[0].Sequence =
		litecoinParams.GenesisBlock.Transactions[0].TxIn[0].Sequence
	params.GenesisBlock.Transactions[0].TxIn[0].PreviousOutPoint.Index =
		litecoinParams.GenesisBlock.Transactions[0].TxIn[0].PreviousOutPoint.Index
	copy(params.GenesisBlock.Transactions[0].TxIn[0].SignatureScript[:],
		litecoinParams.GenesisBlock.Transactions[0].TxIn[0].SignatureScript[:])
	copy(params.GenesisBlock.Transactions[0].TxOut[0].PkScript[:],
		litecoinParams.GenesisBlock.Transactions[0].TxOut[0].PkScript[:])
	params.GenesisBlock.Transactions[0].TxOut[0].Value =
		litecoinParams.GenesisBlock.Transactions[0].TxOut[0].Value
	params.GenesisBlock.Transactions[0].TxIn[0].PreviousOutPoint.Hash =
		chainhash.Hash{}
	params.PowLimitBits = litecoinParams.PowLimitBits
	params.PowLimit = litecoinParams.PowLimit

	dnsSeeds := make([]chaincfg.DNSSeed, len(litecoinParams.DNSSeeds))
	for i := 0; i < len(litecoinParams.DNSSeeds); i++ {
		dnsSeeds[i] = chaincfg.DNSSeed{
			Host:         litecoinParams.DNSSeeds[i].Host,
			HasFiltering: litecoinParams.DNSSeeds[i].HasFiltering,
		}
	}
	params.DNSSeeds = dnsSeeds

	// Address encoding magics
	params.PubKeyHashAddrID = litecoinParams.PubKeyHashAddrID
	params.ScriptHashAddrID = litecoinParams.ScriptHashAddrID
	params.PrivateKeyID = litecoinParams.PrivateKeyID
	params.WitnessPubKeyHashAddrID = litecoinParams.WitnessPubKeyHashAddrID
	params.WitnessScriptHashAddrID = litecoinParams.WitnessScriptHashAddrID
	params.Bech32HRPSegwit = litecoinParams.Bech32HRPSegwit

	copy(params.HDPrivateKeyID[:], litecoinParams.HDPrivateKeyID[:])
	copy(params.HDPublicKeyID[:], litecoinParams.HDPublicKeyID[:])

	params.HDCoinType = litecoinParams.HDCoinType

	checkPoints := make([]chaincfg.Checkpoint, len(litecoinParams.Checkpoints))
	for i := 0; i < len(litecoinParams.Checkpoints); i++ {
		var chainHash chainhash.Hash
		copy(chainHash[:], litecoinParams.Checkpoints[i].Hash[:])

		checkPoints[i] = chaincfg.Checkpoint{
			Height: litecoinParams.Checkpoints[i].Height,
			Hash:   &chainHash,
		}
	}
	params.Checkpoints = checkPoints

	params.RPCPort = litecoinParams.RPCPort
	params.CoinType = litecoinParams.CoinType
	chaincfg.Register(params.Params) // JMC
}

// IsTestnet tests if the givern params correspond to a testnet
// parameter configuration.
func IsTestnet(params *LitecoinNetParams) bool {
	switch params.Params.Net {
	case litecoinWire.TestNet4:
		return true
	default:
		return false
	}
}
