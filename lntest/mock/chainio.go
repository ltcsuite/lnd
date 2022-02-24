package mock

import (
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/wire"
)

// ChainIO is a mock implementation of the BlockChainIO interface.
type ChainIO struct {
	BestHeight int32
}

// GetBestBlock currently returns dummy values.
func (c *ChainIO) GetBestBlock() (*chainhash.Hash, int32, error) {
	return chaincfg.TestNet4Params.GenesisHash, c.BestHeight, nil
}

// GetUtxo currently returns dummy values.
func (c *ChainIO) GetUtxo(op *wire.OutPoint, _ []byte,
	heightHint uint32, _ <-chan struct{}) (*wire.TxOut, error) {

	return nil, nil
}

// GetBlockHash currently returns dummy values.
func (c *ChainIO) GetBlockHash(blockHeight int64) (*chainhash.Hash, error) {
	return nil, nil
}

// GetBlock currently returns dummy values.
func (c *ChainIO) GetBlock(blockHash *chainhash.Hash) (*wire.MsgBlock, error) {
	return nil, nil
}
