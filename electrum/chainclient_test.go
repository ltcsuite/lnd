package electrum

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
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

	// Create a test address.
	pubKeyHash := make([]byte, 20)
	addr, err := ltcutil.NewAddressPubKeyHash(pubKeyHash, &chaincfg.MainNetParams)
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
