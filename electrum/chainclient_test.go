package electrum

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"github.com/ltcsuite/neutrino/mwebdb"
	"github.com/stretchr/testify/require"
)

// mockChainClient is a mock Electrum client for testing the chain client.
type mockChainClient struct {
	connected     bool
	headers       map[uint32]*wire.BlockHeader
	headerChan    chan *SubscribeHeadersResult
	currentHeight int32

	mu sync.RWMutex
}

func newMockChainClient() *mockChainClient {
	return &mockChainClient{
		connected:     true,
		headers:       make(map[uint32]*wire.BlockHeader),
		headerChan:    make(chan *SubscribeHeadersResult, 10),
		currentHeight: 100,
	}
}

func (m *mockChainClient) IsConnected() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connected
}

func (m *mockChainClient) SubscribeHeaders(
	ctx context.Context) (<-chan *SubscribeHeadersResult, error) {

	return m.headerChan, nil
}

func (m *mockChainClient) GetBlockHeader(ctx context.Context,
	height uint32) (*wire.BlockHeader, error) {

	m.mu.RLock()
	defer m.mu.RUnlock()

	if header, ok := m.headers[height]; ok {
		return header, nil
	}

	// Return a default header.
	return &wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now().Add(-time.Duration(m.currentHeight-int32(height)) * 10 * time.Minute),
		Bits:      0x1d00ffff,
	}, nil
}

func (m *mockChainClient) GetHistory(ctx context.Context,
	scripthash string) ([]*GetMempoolResult, error) {

	return nil, nil
}

func (m *mockChainClient) GetTransactionMsgTx(ctx context.Context,
	txHash *chainhash.Hash) (*wire.MsgTx, error) {

	return wire.NewMsgTx(wire.TxVersion), nil
}

func (m *mockChainClient) BroadcastTx(ctx context.Context,
	tx *wire.MsgTx) (*chainhash.Hash, error) {

	hash := tx.TxHash()
	return &hash, nil
}

func (m *mockChainClient) setConnected(connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connected = connected
}

func (m *mockChainClient) addHeader(height uint32, header *wire.BlockHeader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.headers[height] = header
}

func (m *mockChainClient) sendHeader(height int32) {
	m.headerChan <- &SubscribeHeadersResult{Height: height}
}

// TestChainClientInterface verifies that ChainClient implements chain.Interface.
func TestChainClientInterface(t *testing.T) {
	t.Parallel()

	var _ chain.Interface = (*ChainClient)(nil)
}

// TestNewChainClient tests creating a new chain client.
func TestNewChainClient(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	require.NotNil(t, chainClient)
	require.NotNil(t, chainClient.client)
	require.NotNil(t, chainClient.headerCache)
	require.NotNil(t, chainClient.heightToHash)
	require.NotNil(t, chainClient.notificationChan)
	require.Equal(t, &chaincfg.MainNetParams, chainClient.chainParams)
}

// TestChainClientBackEnd tests the BackEnd method.
func TestChainClientBackEnd(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	require.Equal(t, "electrum", chainClient.BackEnd())
}

// TestChainClientGetBlockNotSupported tests that GetBlock returns an error.
func TestChainClientGetBlockNotSupported(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	hash := &chainhash.Hash{}
	block, err := chainClient.GetBlock(hash)

	require.Error(t, err)
	require.Nil(t, block)
	require.ErrorIs(t, err, ErrFullBlocksNotSupported)
}

// TestChainClientNotifications tests the Notifications channel.
func TestChainClientNotifications(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	notifChan := chainClient.Notifications()
	require.NotNil(t, notifChan)
}

// TestChainClientTestMempoolAccept tests that TestMempoolAccept returns nil.
func TestChainClientTestMempoolAccept(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	tx := wire.NewMsgTx(wire.TxVersion)
	results, err := chainClient.TestMempoolAccept([]*wire.MsgTx{tx}, 0.0)

	// Electrum doesn't support this, so we expect nil results without error.
	require.NoError(t, err)
	require.Nil(t, results)
}

// TestChainClientMapRPCErr tests the MapRPCErr method.
func TestChainClientMapRPCErr(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	testErr := ErrNotConnected
	mappedErr := chainClient.MapRPCErr(testErr)

	require.Equal(t, testErr, mappedErr)
}

// TestChainClientNotifyBlocks tests enabling block notifications.
func TestChainClientNotifyBlocks(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	err := chainClient.NotifyBlocks()
	require.NoError(t, err)
	require.True(t, chainClient.notifyBlocks.Load())
}

// TestChainClientNotifyReceived tests adding watched addresses.
func TestChainClientNotifyReceived(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	// NotifyReceived only monitors P2WPKH addresses (other types are
	// either unsupported by Electrum or handled by mwebsync).
	pubKeyHash := make([]byte, 20)
	addr, err := ltcutil.NewAddressWitnessPubKeyHash(
		pubKeyHash, &chaincfg.MainNetParams,
	)
	require.NoError(t, err)

	err = chainClient.NotifyReceived([]ltcutil.Address{addr})
	require.NoError(t, err)

	chainClient.watchedAddrsMtx.RLock()
	_, exists := chainClient.watchedAddrs[addr.EncodeAddress()]
	chainClient.watchedAddrsMtx.RUnlock()

	require.True(t, exists)
}

// TestPayToAddrScript tests the script generation helper functions.
func TestPayToAddrScript(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		makeAddr  func() (ltcutil.Address, error)
		expectLen int
		expectErr bool
	}{
		{
			name: "P2PKH",
			makeAddr: func() (ltcutil.Address, error) {
				pubKeyHash := make([]byte, 20)
				return ltcutil.NewAddressPubKeyHash(
					pubKeyHash, &chaincfg.MainNetParams,
				)
			},
			expectLen: 25, // OP_DUP OP_HASH160 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
			expectErr: false,
		},
		{
			name: "P2SH",
			makeAddr: func() (ltcutil.Address, error) {
				scriptHash := make([]byte, 20)
				return ltcutil.NewAddressScriptHash(
					scriptHash, &chaincfg.MainNetParams,
				)
			},
			expectLen: 23, // OP_HASH160 <20 bytes> OP_EQUAL
			expectErr: false,
		},
		{
			name: "P2WPKH",
			makeAddr: func() (ltcutil.Address, error) {
				pubKeyHash := make([]byte, 20)
				return ltcutil.NewAddressWitnessPubKeyHash(
					pubKeyHash, &chaincfg.MainNetParams,
				)
			},
			expectLen: 22, // OP_0 <20 bytes>
			expectErr: false,
		},
		{
			name: "P2WSH",
			makeAddr: func() (ltcutil.Address, error) {
				scriptHash := make([]byte, 32)
				return ltcutil.NewAddressWitnessScriptHash(
					scriptHash, &chaincfg.MainNetParams,
				)
			},
			expectLen: 34, // OP_0 <32 bytes>
			expectErr: false,
		},
		{
			name: "P2TR",
			makeAddr: func() (ltcutil.Address, error) {
				pubKey := make([]byte, 32)
				return ltcutil.NewAddressTaproot(
					pubKey, &chaincfg.MainNetParams,
				)
			},
			expectLen: 34, // OP_1 <32 bytes>
			expectErr: false,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			addr, err := tc.makeAddr()
			require.NoError(t, err)

			script, err := PayToAddrScript(addr)

			if tc.expectErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Len(t, script, tc.expectLen)
		})
	}
}

// TestChainClientIsCurrent tests the IsCurrent method.
// Note: IsCurrent() fetches fresh block data from the network, so without
// a live connection it will return false. This test verifies that behavior.
func TestChainClientIsCurrent(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        0, // Don't retry to speed up test
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	// Without a live connection, IsCurrent() should return false since it
	// cannot fetch the best block from the network. This matches the
	// behavior of other backends (bitcoind, btcd) which also call
	// GetBestBlock() and GetBlockHeader() in IsCurrent().
	require.False(t, chainClient.IsCurrent())
}

// TestChainClientCacheHeader tests the header caching functionality.
func TestChainClientCacheHeader(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	// Create a test header.
	header := &wire.BlockHeader{
		Version:   1,
		Timestamp: time.Now(),
		Bits:      0x1d00ffff,
	}
	hash := header.BlockHash()
	height := int32(100)

	// Cache the header.
	chainClient.cacheHeader(height, &hash, header)

	// Verify it's in the header cache.
	chainClient.headerCacheMtx.RLock()
	cachedHeader, exists := chainClient.headerCache[hash]
	chainClient.headerCacheMtx.RUnlock()

	require.True(t, exists)
	require.Equal(t, header, cachedHeader)

	// Verify height to hash mapping.
	chainClient.heightToHashMtx.RLock()
	cachedHash, exists := chainClient.heightToHash[height]
	chainClient.heightToHashMtx.RUnlock()

	require.True(t, exists)
	require.Equal(t, &hash, cachedHash)
}

// TestChainClientGetUtxo tests the GetUtxo method.
func TestChainClientGetUtxo(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    1 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        0,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	// Create a test outpoint and pkScript.
	testHash := chainhash.Hash{0x01, 0x02, 0x03}
	op := &wire.OutPoint{
		Hash:  testHash,
		Index: 0,
	}
	pkScript := []byte{0x00, 0x14, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06,
		0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10,
		0x11, 0x12, 0x13, 0x14}

	// Without a connected client, GetUtxo should return an error.
	cancel := make(chan struct{})
	_, err := chainClient.GetUtxo(op, pkScript, 100, cancel)
	require.Error(t, err)
}

// TestElectrumUtxoSourceInterface verifies that ChainClient implements the
// ElectrumUtxoSource interface used by btcwallet.
func TestElectrumUtxoSourceInterface(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(client, &chaincfg.MainNetParams, "", "", nil)

	// Define the interface locally to test without importing btcwallet.
	type UtxoSource interface {
		GetUtxo(op *wire.OutPoint, pkScript []byte, heightHint uint32,
			cancel <-chan struct{}) (*wire.TxOut, error)
	}

	// Verify ChainClient implements UtxoSource.
	var _ UtxoSource = chainClient
}

// TestSortAndGroupTxs tests that transactions are sorted by block height
// and topologically within each block.
func TestSortAndGroupTxs(t *testing.T) {
	t.Parallel()

	// Create transactions at different heights, deliberately out of order.
	tx1 := wire.NewMsgTx(wire.TxVersion)
	tx1.AddTxOut(&wire.TxOut{Value: 1})
	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxOut(&wire.TxOut{Value: 2})
	tx3 := wire.NewMsgTx(wire.TxVersion)
	tx3.AddTxOut(&wire.TxOut{Value: 3})

	txs := []*collectedTx{
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx3,
				Hash:  tx3.TxHash(),
			},
			height: 300,
		},
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx1,
				Hash:  tx1.TxHash(),
			},
			height: 100,
		},
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx2,
				Hash:  tx2.TxHash(),
			},
			height: 200,
		},
	}

	sorted := sortAndGroupTxs(txs)

	require.Len(t, sorted, 3)
	require.Equal(t, int32(100), sorted[0].height)
	require.Equal(t, int32(200), sorted[1].height)
	require.Equal(t, int32(300), sorted[2].height)
}

// TestSortAndGroupTxsSingleTx tests that a single transaction is returned
// unchanged.
func TestSortAndGroupTxsSingleTx(t *testing.T) {
	t.Parallel()

	tx := wire.NewMsgTx(wire.TxVersion)
	txs := []*collectedTx{
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx,
				Hash:  tx.TxHash(),
			},
			height: 100,
		},
	}

	sorted := sortAndGroupTxs(txs)
	require.Len(t, sorted, 1)
	require.Equal(t, int32(100), sorted[0].height)
}

// TestSortAndGroupTxsEmpty tests that an empty slice is handled correctly.
func TestSortAndGroupTxsEmpty(t *testing.T) {
	t.Parallel()

	sorted := sortAndGroupTxs(nil)
	require.Nil(t, sorted)

	sorted = sortAndGroupTxs([]*collectedTx{})
	require.Len(t, sorted, 0)
}

// TestSendFilteredBlocks tests that sendFilteredBlocks groups transactions
// by block height and sends FilteredBlockConnected notifications.
func TestSendFilteredBlocks(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(
		client, &chaincfg.MainNetParams, "", "", nil,
	)

	// Create transactions in two different blocks.
	tx1 := wire.NewMsgTx(wire.TxVersion)
	tx1.AddTxOut(&wire.TxOut{Value: 1})
	tx2 := wire.NewMsgTx(wire.TxVersion)
	tx2.AddTxOut(&wire.TxOut{Value: 2})
	tx3 := wire.NewMsgTx(wire.TxVersion)
	tx3.AddTxOut(&wire.TxOut{Value: 3})

	hash100 := chainhash.Hash{0x01}
	hash200 := chainhash.Hash{0x02}
	blockTime := time.Now()

	// Transactions are already sorted by height (as sortAndGroupTxs would
	// produce).
	txs := []*collectedTx{
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx1, Hash: tx1.TxHash(),
			},
			height: 100, blockHash: hash100, blockTime: blockTime,
		},
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx2, Hash: tx2.TxHash(),
			},
			height: 100, blockHash: hash100, blockTime: blockTime,
		},
		{
			txRecord: &wtxmgr.TxRecord{
				MsgTx: *tx3, Hash: tx3.TxHash(),
			},
			height: 200, blockHash: hash200, blockTime: blockTime,
		},
	}

	chainClient.sendFilteredBlocks(txs)

	// Should receive 2 FilteredBlockConnected notifications (one per block).
	notifChan := chainClient.Notifications()

	notif1 := <-notifChan
	fbc1, ok := notif1.(chain.FilteredBlockConnected)
	require.True(t, ok)
	require.Equal(t, int32(100), fbc1.Block.Height)
	require.Len(t, fbc1.RelevantTxs, 2)

	notif2 := <-notifChan
	fbc2, ok := notif2.(chain.FilteredBlockConnected)
	require.True(t, ok)
	require.Equal(t, int32(200), fbc2.Block.Height)
	require.Len(t, fbc2.RelevantTxs, 1)
}

// TestSendFilteredBlocksEmpty tests that sendFilteredBlocks does nothing
// with an empty slice.
func TestSendFilteredBlocksEmpty(t *testing.T) {
	t.Parallel()

	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	client := NewClient(cfg)

	chainClient := NewChainClient(
		client, &chaincfg.MainNetParams, "", "", nil,
	)

	chainClient.sendFilteredBlocks(nil)
	chainClient.sendFilteredBlocks([]*collectedTx{})

	// No notifications should have been sent.
	select {
	case <-chainClient.Notifications():
		t.Fatal("unexpected notification")
	default:
	}
}

// --- Mock implementations for MWEB tests ---

// mockCoinDB implements mwebdb.CoinDatabase for testing.
type mockCoinDB struct {
	leafset  *mweb.Leafset
	coins    map[chainhash.Hash]*wire.MwebOutput
	leaves   map[uint64]*wire.MwebNetUtxo
	fetchErr error // injected error for FetchCoin/FetchLeaves
}

func (m *mockCoinDB) GetRollbackHeight() (uint32, error)               { return 0, nil }
func (m *mockCoinDB) PutRollbackHeight(uint32) error                   { return nil }
func (m *mockCoinDB) ClearRollbackHeight(uint32) error                 { return nil }
func (m *mockCoinDB) GetLeavesAtHeight() (map[uint32]uint64, error)    { return nil, nil }
func (m *mockCoinDB) PutLeavesAtHeight(map[uint32]uint64) error        { return nil }
func (m *mockCoinDB) RollbackLeavesAtHeight(uint32) error              { return nil }
func (m *mockCoinDB) PutLeafsetAndPurge(*mweb.Leafset, []uint64) error { return nil }
func (m *mockCoinDB) PutCoins([]*wire.MwebNetUtxo) error               { return nil }
func (m *mockCoinDB) PurgeCoins() error                                { return nil }

func (m *mockCoinDB) GetLeafset() (*mweb.Leafset, error) {
	if m.leafset == nil {
		return &mweb.Leafset{}, nil
	}
	return m.leafset, nil
}

func (m *mockCoinDB) FetchCoin(hash *chainhash.Hash) (*wire.MwebOutput, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	if out, ok := m.coins[*hash]; ok {
		return out, nil
	}
	return nil, mwebdb.ErrCoinNotFound
}

func (m *mockCoinDB) FetchLeaves(indices []uint64) ([]*wire.MwebNetUtxo, error) {
	if m.fetchErr != nil {
		return nil, m.fetchErr
	}
	var result []*wire.MwebNetUtxo
	for _, idx := range indices {
		if utxo, ok := m.leaves[idx]; ok {
			result = append(result, utxo)
		}
	}
	return result, nil
}

var _ mwebdb.CoinDatabase = (*mockCoinDB)(nil)

// mockMwebSyncer implements the mwebSyncer interface for testing.
type mockMwebSyncer struct {
	synced   bool
	doneChan chan struct{}
}

func newMockMwebSyncer(synced bool) *mockMwebSyncer {
	return &mockMwebSyncer{synced: synced, doneChan: make(chan struct{})}
}

func (m *mockMwebSyncer) Start() error          { return nil }
func (m *mockMwebSyncer) Stop() error           { return nil }
func (m *mockMwebSyncer) IsSynced() bool        { return m.synced }
func (m *mockMwebSyncer) Done() <-chan struct{} { return m.doneChan }

var _ mwebSyncer = (*mockMwebSyncer)(nil)

// newTestChainClient creates a minimal ChainClient for MWEB tests.
func newTestChainClient() *ChainClient {
	cfg := &ClientConfig{
		Server:            "localhost:50001",
		UseSSL:            false,
		ReconnectInterval: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
		PingInterval:      60 * time.Second,
		MaxRetries:        3,
	}
	return NewChainClient(
		NewClient(cfg), &chaincfg.MainNetParams, "", "", nil,
	)
}

// TestElectrumMwebInterfaces verifies compile-time interface satisfaction.
func TestElectrumMwebInterfaces(t *testing.T) {
	t.Parallel()

	var _ chain.MwebReplayer = (*ChainClient)(nil)
	var _ chain.MwebUtxoChecker = (*ChainClient)(nil)
}

// TestElectrumMwebUtxoExists tests the MwebUtxoExists method.
func TestElectrumMwebUtxoExists(t *testing.T) {
	t.Parallel()

	knownHash := chainhash.Hash{1, 2, 3}
	unknownHash := chainhash.Hash{4, 5, 6}
	knownOutput := &wire.MwebOutput{}

	t.Run("coin exists", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebCoinDB = &mockCoinDB{
			coins: map[chainhash.Hash]*wire.MwebOutput{
				knownHash: knownOutput,
			},
		}

		exists, err := c.MwebUtxoExists(&knownHash)
		require.NoError(t, err)
		require.True(t, exists)
	})

	t.Run("coin not found", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebCoinDB = &mockCoinDB{
			coins: map[chainhash.Hash]*wire.MwebOutput{},
		}

		exists, err := c.MwebUtxoExists(&unknownHash)
		require.NoError(t, err)
		require.False(t, exists)
	})

	t.Run("DB error propagated", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		dbErr := errors.New("disk I/O error")
		c.mwebCoinDB = &mockCoinDB{fetchErr: dbErr}

		exists, err := c.MwebUtxoExists(&unknownHash)
		require.ErrorIs(t, err, dbErr)
		require.False(t, exists)
	})

	t.Run("nil coinDB returns error", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		// mwebCoinDB is nil (MWEB sync not started).

		exists, err := c.MwebUtxoExists(&unknownHash)
		require.Error(t, err)
		require.False(t, exists)
	})
}

// TestElectrumIsMwebSynced tests the IsMwebSynced method.
func TestElectrumIsMwebSynced(t *testing.T) {
	t.Parallel()

	t.Run("startup in progress", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		// mwebSyncer=nil, mwebStartDone=false → startup still running
		require.False(t, c.IsMwebSynced())
	})

	t.Run("startup failed", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		// mwebSyncer=nil, mwebStartDone=true → startup finished but
		// failed. Returns true to unblock callers waiting for sync.
		c.mwebStartDone.Store(true)
		require.True(t, c.IsMwebSynced())
	})

	t.Run("syncer not synced", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebSyncer = newMockMwebSyncer(false)
		require.False(t, c.IsMwebSynced())
	})

	t.Run("syncer synced", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebSyncer = newMockMwebSyncer(true)
		require.True(t, c.IsMwebSynced())
	})
}

// TestElectrumMwebCoinDBNotPublishedBeforeStartup verifies that mwebCoinDB
// is nil when startup hasn't completed
func TestElectrumMwebCoinDBNotPublishedBeforeStartup(t *testing.T) {
	t.Parallel()
	c := newTestChainClient()

	// Before startup, mwebCoinDB should be nil.
	require.Nil(t, c.mwebCoinDB)

	// ReplayMwebUtxos should return a clean error, not use a stale DB.
	err := c.ReplayMwebUtxos()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")

	// MwebUtxoExists should return a clean error too.
	exists, err := c.MwebUtxoExists(&chainhash.Hash{1})
	require.Error(t, err)
	require.False(t, exists)
}

// TestElectrumMwebStartupFailureUnblocksSynced verifies that IsMwebSynced
// returns true after startup fails, so recoverMwebUtxos doesn't poll forever.
func TestElectrumMwebStartupFailureUnblocksSynced(t *testing.T) {
	t.Parallel()
	c := newTestChainClient()

	// Before startup finishes: IsMwebSynced returns false (wait).
	require.False(t, c.IsMwebSynced())

	// Simulate startup failure: mwebStartDone becomes true but no syncer.
	c.mwebStartDone.Store(true)

	// Now IsMwebSynced returns true (unblocks callers).
	require.True(t, c.IsMwebSynced())

	// But ReplayMwebUtxos returns clean error (coinDB is nil).
	err := c.ReplayMwebUtxos()
	require.Error(t, err)
}

// TestWaitForSyncOrShutdownSyncerDies verifies that when the sync loop
// exits unexpectedly (syncer.Done() fires), waitForSyncOrShutdown clears
// mwebSyncer and mwebCoinDB so IsMwebSynced falls through to mwebStartDone.
func TestWaitForSyncOrShutdownSyncerDies(t *testing.T) {
	t.Parallel()
	c := newTestChainClient()

	// Simulate successful startup state.
	mock := newMockMwebSyncer(false)
	c.mwebSyncer = mock
	c.mwebCoinDB = &mockCoinDB{}
	c.mwebStartDone.Store(false) // will be set by defer in startMwebSync

	// IsMwebSynced should return false (syncer exists, not synced).
	require.False(t, c.IsMwebSynced())

	// Run waitForSyncOrShutdown in a goroutine, simulating
	// the tail end of startMwebSync.
	done := make(chan struct{})
	go func() {
		c.waitForSyncOrShutdown(mock)
		// Simulate the defer in startMwebSync.
		c.mwebStartDone.Store(true)
		close(done)
	}()

	// Simulate sync loop dying.
	close(mock.doneChan)

	// Wait for cleanup to finish.
	<-done

	// Verify cleanup: syncer and coinDB both nil.
	require.Nil(t, c.mwebSyncer)
	require.Nil(t, c.mwebCoinDB)

	// IsMwebSynced should now return true (mwebStartDone fallback).
	require.True(t, c.IsMwebSynced())

	// ReplayMwebUtxos returns clean error.
	err := c.ReplayMwebUtxos()
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}

// TestWaitForSyncOrShutdownNormalStop verifies that on normal shutdown
// (c.quit fires), waitForSyncOrShutdown calls syncer.Stop() and clears
// mwebCoinDB, but leaves mwebSyncer intact.
func TestWaitForSyncOrShutdownNormalStop(t *testing.T) {
	t.Parallel()
	c := newTestChainClient()

	mock := newMockMwebSyncer(true)
	c.mwebSyncer = mock
	c.mwebCoinDB = &mockCoinDB{}

	done := make(chan struct{})
	go func() {
		c.waitForSyncOrShutdown(mock)
		close(done)
	}()

	// Normal shutdown.
	close(c.quit)
	<-done

	// On normal shutdown, mwebSyncer is NOT cleared (syncer.Stop was
	// called instead). mwebCoinDB IS cleared (DB is closing).
	require.NotNil(t, c.mwebSyncer)
	require.Nil(t, c.mwebCoinDB)
}

// TestElectrumReplayMwebUtxos tests the ReplayMwebUtxos method.
func TestElectrumReplayMwebUtxos(t *testing.T) {
	t.Parallel()

	t.Run("nil coinDB returns error", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		err := c.ReplayMwebUtxos()
		require.Error(t, err)
	})

	t.Run("empty leafset sends no notifications", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebCoinDB = &mockCoinDB{
			leafset: &mweb.Leafset{Size: 0, Bits: []byte{}},
		}

		err := c.ReplayMwebUtxos()
		require.NoError(t, err)

		// No notifications should be sent.
		select {
		case <-c.notificationChan:
			t.Fatal("unexpected notification for empty leafset")
		default:
		}
	})

	t.Run("delivers UTXOs through notifications", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()

		// Create a leafset with 3 leaves: indices 0, 1, 2 all present.
		// Bits: 0b11100000 = 0xE0 (MSB first per Contains())
		leafset := &mweb.Leafset{
			Size: 3,
			Bits: []byte{0xE0},
		}

		utxo0 := &wire.MwebNetUtxo{LeafIndex: 0, Height: 100,
			OutputId: &chainhash.Hash{0xAA}}
		utxo1 := &wire.MwebNetUtxo{LeafIndex: 1, Height: 101,
			OutputId: &chainhash.Hash{0xBB}}
		utxo2 := &wire.MwebNetUtxo{LeafIndex: 2, Height: 102,
			OutputId: &chainhash.Hash{0xCC}}

		c.mwebCoinDB = &mockCoinDB{
			leafset: leafset,
			leaves: map[uint64]*wire.MwebNetUtxo{
				0: utxo0, 1: utxo1, 2: utxo2,
			},
		}

		err := c.ReplayMwebUtxos()
		require.NoError(t, err)

		// With 3 UTXOs and batchSize=5000, all should be in one batch.
		notif := <-c.notificationChan
		mwebNotif, ok := notif.(chain.MwebUtxos)
		require.True(t, ok)
		require.Len(t, mwebNotif.Utxos, 3)
		require.Equal(t, leafset, mwebNotif.Leafset)

		// No more notifications.
		select {
		case <-c.notificationChan:
			t.Fatal("unexpected extra notification")
		default:
		}
	})

	t.Run("batches large UTXO sets", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()

		// Create a leafset with 7500 leaves (will require 2 batches).
		numLeaves := uint64(7500)
		bits := make([]byte, (numLeaves+7)/8)
		for i := uint64(0); i < numLeaves; i++ {
			bits[i/8] |= 0x80 >> (i % 8) // set bit
		}
		leafset := &mweb.Leafset{
			Size: numLeaves,
			Bits: bits,
		}

		leaves := make(map[uint64]*wire.MwebNetUtxo)
		for i := uint64(0); i < numLeaves; i++ {
			leaves[i] = &wire.MwebNetUtxo{
				LeafIndex: i,
				Height:    int32(100 + i),
				OutputId:  &chainhash.Hash{byte(i), byte(i >> 8)},
			}
		}

		c.mwebCoinDB = &mockCoinDB{
			leafset: leafset,
			leaves:  leaves,
		}

		err := c.ReplayMwebUtxos()
		require.NoError(t, err)

		// Should receive 2 notifications: batch of 5000 + batch of 2500.
		notif1 := <-c.notificationChan
		mwebNotif1, ok := notif1.(chain.MwebUtxos)
		require.True(t, ok)
		require.Len(t, mwebNotif1.Utxos, 5000)

		notif2 := <-c.notificationChan
		mwebNotif2, ok := notif2.(chain.MwebUtxos)
		require.True(t, ok)
		require.Len(t, mwebNotif2.Utxos, 2500)

		// No more notifications.
		select {
		case <-c.notificationChan:
			t.Fatal("unexpected extra notification")
		default:
		}
	})

	t.Run("FetchLeaves error propagated", func(t *testing.T) {
		t.Parallel()
		c := newTestChainClient()
		c.mwebCoinDB = &mockCoinDB{
			leafset:  &mweb.Leafset{Size: 1, Bits: []byte{0x80}},
			fetchErr: errors.New("DB read failed"),
		}

		err := c.ReplayMwebUtxos()
		require.Error(t, err)
		require.Contains(t, err.Error(), "DB read failed")
	})
}
