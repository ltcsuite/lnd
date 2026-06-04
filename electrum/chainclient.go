package electrum

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/checksum0/go-electrum/electrum"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"github.com/ltcsuite/neutrino/mwebdb"
	"github.com/ltcsuite/neutrino/query"

	"github.com/ltcsuite/lnd/electrum/mwebp2p"
)

const (
	// electrumBackendName is the name of the Electrum backend.
	electrumBackendName = "electrum"

	// defaultRequestTimeout is the default timeout for Electrum requests.
	defaultRequestTimeout = 30 * time.Second

	// batchSize is the number of block headers to fetch in a single batch.
	// Set to 2016 (one Bitcoin difficulty adjustment period) for optimal
	// performance while respecting natural epoch boundaries.
	batchSize = 2016

	// addressScanConcurrency is the number of concurrent workers for
	// address history scanning. Set to 10 to match ElectrumX's default
	// INITIAL_CONCURRENT limit, providing 8-10x speedup while respecting
	// server rate limits.
	addressScanConcurrency = 10

	// reorgSafetyDepth is how many blocks below the synced height the
	// scripthash replay filter leaves in scope. Skipping confirmed history
	// the wallet already holds speeds up restarts, but a reorg can
	// re-confirm a transaction near the synced tip; keeping this many
	// recent blocks unfiltered ensures such a transaction is still
	// delivered, at the cost of redundantly replaying a few recent blocks.
	reorgSafetyDepth = 100
)

var (
	// ErrBlockNotFound is returned when a block cannot be found.
	ErrBlockNotFound = errors.New("block not found")

	// ErrFullBlocksNotSupported is returned when full block retrieval is
	// attempted but not supported by Electrum.
	ErrFullBlocksNotSupported = errors.New("electrum does not support " +
		"full block retrieval")

	// ErrNotImplemented is returned for operations not supported by
	// Electrum.
	ErrNotImplemented = errors.New("operation not implemented for " +
		"electrum backend")

	// ErrOutputSpent is returned when the requested output has been spent.
	ErrOutputSpent = errors.New("output has been spent")

	// ErrOutputNotFound is returned when the requested output cannot be
	// found.
	ErrOutputNotFound = errors.New("output not found")
)

// TestMempoolAcceptResult models the data from the testmempoolaccept command.
type TestMempoolAcceptResult struct {
	TxID    string `json:"txid"`
	Allowed bool   `json:"allowed"`
}

// collectedTx holds a transaction discovered during address scanning along
// with its block metadata. Used to collect transactions before sorting and
// sending them as FilteredBlockConnected notifications.
type collectedTx struct {
	txRecord  *wtxmgr.TxRecord
	height    int32
	blockHash chainhash.Hash
	blockTime time.Time
}

// sortAndGroupTxs sorts collected transactions by block height, then
// topologically sorts within each block so that funding transactions are
// processed before spending transactions. Returns the sorted slice.
//
// This ordering is critical because ltcwallet's addRelevantTx creates debits
// by looking up existing credits. If a spending tx is processed before its
// funding tx, the credit won't exist yet and no debit will be created,
// resulting in zero amounts.
func sortAndGroupTxs(txs []*collectedTx) []*collectedTx {
	if len(txs) <= 1 {
		return txs
	}

	// Sort by block height first for cross-block ordering.
	sort.SliceStable(txs, func(i, j int) bool {
		return txs[i].height < txs[j].height
	})

	// Group by block height and topologically sort each group.
	var result []*collectedTx
	groupStart := 0
	for i := 1; i <= len(txs); i++ {
		if i == len(txs) || txs[i].height != txs[groupStart].height {
			group := txs[groupStart:i]
			sorted := topoSortBlock(group)
			result = append(result, sorted...)
			groupStart = i
		}
	}

	return result
}

// topoSortBlock performs a topological sort on transactions within a single
// block. Transactions that fund other transactions in the same block are
// ordered first, ensuring credits exist before debits reference them.
func topoSortBlock(txs []*collectedTx) []*collectedTx {
	if len(txs) <= 1 {
		return txs
	}

	// Build a set of txids in this block.
	txIndex := make(map[chainhash.Hash]int, len(txs))
	for i, tx := range txs {
		txIndex[tx.txRecord.Hash] = i
	}

	// Build dependency graph: inDegree[i] = number of intra-block
	// dependencies that must be processed before txs[i].
	inDegree := make([]int, len(txs))
	dependents := make([][]int, len(txs))

	for i, tx := range txs {
		for _, txIn := range tx.txRecord.MsgTx.TxIn {
			if j, ok := txIndex[txIn.PreviousOutPoint.Hash]; ok && j != i {
				inDegree[i]++
				dependents[j] = append(dependents[j], i)
			}
		}
	}

	// Kahn's algorithm for topological sort.
	queue := make([]int, 0, len(txs))
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	result := make([]*collectedTx, 0, len(txs))
	for len(queue) > 0 {
		idx := queue[0]
		queue = queue[1:]
		result = append(result, txs[idx])
		for _, dep := range dependents[idx] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	// Append any remaining transactions (cycles shouldn't occur in valid
	// blocks, but handle gracefully).
	if len(result) < len(txs) {
		for i := range txs {
			if inDegree[i] > 0 {
				result = append(result, txs[i])
			}
		}
	}

	return result
}

// ChainClient is an implementation of chain.Interface that uses an Electrum
// server as its backend. Note that Electrum servers have limitations compared
// to full nodes - notably they cannot serve full block data.
type ChainClient struct {
	started int32
	stopped int32

	client *Client

	// restClient is an optional REST API client for fetching full blocks
	// from mempool/electrs. If nil, GetBlock will return an error.
	restClient *RESTClient

	chainParams *chaincfg.Params

	// bestBlockMtx protects bestBlock.
	bestBlockMtx sync.RWMutex
	bestBlock    waddrmgr.BlockStamp

	// headerCache caches block headers by hash for efficient lookups.
	headerCacheMtx sync.RWMutex
	headerCache    map[chainhash.Hash]*wire.BlockHeader

	// heightToHash maps block heights to hashes.
	heightToHashMtx sync.RWMutex
	heightToHash    map[int32]*chainhash.Hash

	// notificationChan is used to send notifications to the wallet.
	notificationChan chan interface{}

	// notifyBlocks indicates whether we should send block notifications.
	notifyBlocks atomic.Bool

	// watchedAddresses contains addresses we're watching for activity.
	watchedAddrsMtx sync.RWMutex
	watchedAddrs    map[string]ltcutil.Address

	// watchedOutpoints contains outpoints we're watching for spends.
	watchedOutpointsMtx sync.RWMutex
	watchedOutpoints    map[wire.OutPoint]ltcutil.Address

	// historyCache caches address history results during sync to avoid
	// redundant queries. This is critical for performance - without it,
	// syncing 400k blocks with 5000 addresses would require 1 million
	// GetHistory calls instead of just 5000.
	historyCacheMtx sync.RWMutex
	historyCache    map[string][]*electrum.GetMempoolResult

	// scripthashSub manages all scripthash subscriptions for real-time
	// address monitoring via the Electrum protocol. A single subscription
	// object multiplexes all addresses onto one notification channel.
	scripthashSub   *ScripthashSubscription
	scripthashNotif <-chan *SubscribeNotif

	// scripthashHistory tracks the last known status hash for each
	// subscribed scripthash. When a notification arrives with a different
	// status, we know there are new transactions to fetch.
	scripthashHistMtx sync.RWMutex
	scripthashHistory map[string]string

	// rescanStartHeight is the wallet's last synced height, captured when
	// Rescan begins. Scripthash notifications skip confirmed transactions
	// below it because the wallet already holds them.
	rescanStartHeight atomic.Int32

	// dataDir is the directory for storing MWEB data (mweb.db).
	dataDir string

	// mwebP2PPeers is an optional list of explicit P2P peers for MWEB sync.
	// If empty, DNS seeds are used for peer discovery.
	mwebP2PPeers []string

	// p2pService manages lightweight P2P connections for MWEB queries.
	p2pService *mwebp2p.Service

	// workManager dispatches MWEB queries to P2P peers.
	workManager query.WorkManager

	// mwebMtx protects mwebSyncer and mwebCoinDB, which are written from
	// the startMwebSync goroutine and read from wallet goroutines via
	// IsMwebSynced, ReplayMwebUtxos, and MwebUtxoExists.
	mwebMtx sync.RWMutex

	// mwebSyncer is the mwebsync syncer that handles MWEB synchronization.
	mwebSyncer mwebSyncer

	// mwebCoinDB is the MWEB coin database, retained from startMwebSync
	// for replay/spentness checks. Only set after startMwebSync fully
	// succeeds; cleared before db.Close() on shutdown.
	mwebCoinDB mwebdb.CoinDatabase

	// mwebStartDone is set to true when startMwebSync exits (whether it
	// succeeded or failed). This allows IsMwebSynced to distinguish
	// "startup in progress" (wait) from "startup failed" (don't wait).
	mwebStartDone atomic.Bool

	quit chan struct{}
	wg   sync.WaitGroup
}

// mwebSyncer is an interface for the mwebsync syncer, used for testability.
type mwebSyncer interface {
	Start() error
	Stop() error
	IsSynced() bool
	Done() <-chan struct{}
}

// Compile time checks.
var (
	_ chain.Interface       = (*ChainClient)(nil)
	_ chain.MwebReplayer    = (*ChainClient)(nil)
	_ chain.MwebUtxoChecker = (*ChainClient)(nil)
)

// NewChainClient creates a new Electrum chain client.
// If restURL is provided, the client will be able to fetch full blocks
// via the mempool/electrs REST API.
// dataDir is the directory for storing MWEB data.
// mwebP2PPeers is an optional list of explicit P2P peers for MWEB sync.
func NewChainClient(client *Client, chainParams *chaincfg.Params,
	restURL string, dataDir string, mwebP2PPeers []string) *ChainClient {

	var restClient *RESTClient
	if restURL != "" {
		restClient = NewRESTClient(restURL)
		log.Infof("Electrum REST API enabled: %s", restURL)
	}

	return &ChainClient{
		client:            client,
		restClient:        restClient,
		chainParams:       chainParams,
		headerCache:       make(map[chainhash.Hash]*wire.BlockHeader),
		heightToHash:      make(map[int32]*chainhash.Hash),
		notificationChan:  make(chan interface{}, 2100),
		watchedAddrs:      make(map[string]ltcutil.Address),
		watchedOutpoints:  make(map[wire.OutPoint]ltcutil.Address),
		historyCache:      make(map[string][]*electrum.GetMempoolResult),
		scripthashHistory: make(map[string]string),
		dataDir:           dataDir,
		mwebP2PPeers:      mwebP2PPeers,
		quit:              make(chan struct{}),
	}
}

// sendNotification sends a notification to the notification channel in a
// non-blocking manner. It only blocks on the quit signal to ensure graceful
// shutdown. This prevents deadlocks when the notification channel buffer is
// full while still allowing operations to complete.
//
// Returns true if the notification was sent, false if the client is shutting
// down.
func (c *ChainClient) sendNotification(notification interface{}) bool {
	select {
	case c.notificationChan <- notification:
		return true
	case <-c.quit:
		return false
	}
}

// Start initializes the chain client and begins processing notifications.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) Start() error {
	if atomic.AddInt32(&c.started, 1) != 1 {
		return nil
	}

	log.Info("Starting Electrum chain client")

	// Ensure the underlying client is connected.
	if !c.client.IsConnected() {
		return ErrNotConnected
	}

	// Subscribe to headers using a background context that won't be
	// cancelled when Start() returns. The subscription needs to live for
	// the lifetime of the client.
	headerChan, err := c.client.SubscribeHeaders(context.Background())
	if err != nil {
		return fmt.Errorf("failed to subscribe to headers: %w", err)
	}

	// Get initial header with a timeout.
	select {
	case header := <-headerChan:
		ctx, cancel := context.WithTimeout(
			context.Background(), defaultRequestTimeout,
		)
		blockHeader, err := c.client.GetBlockHeader(
			ctx, uint32(header.Height),
		)
		cancel()
		if err != nil {
			return fmt.Errorf("failed to get initial header: %w",
				err)
		}

		hash := blockHeader.BlockHash()
		c.bestBlockMtx.Lock()
		c.bestBlock = waddrmgr.BlockStamp{
			Height:    int32(header.Height),
			Hash:      hash,
			Timestamp: blockHeader.Timestamp,
		}
		c.bestBlockMtx.Unlock()

		// Cache the header.
		c.cacheHeader(int32(header.Height), &hash, blockHeader)

	case <-time.After(defaultRequestTimeout):
		return errors.New("timeout waiting for initial header")
	}

	// Start the notification handler for block headers.
	c.wg.Add(1)
	go c.notificationHandler(headerChan)

	// Initialize scripthash subscription for real-time address monitoring.
	// This creates a single subscription object that multiplexes all
	// watched addresses onto one notification channel.
	sub, notifChan, err := c.client.InitScripthashSubscription()
	if err != nil {
		log.Warnf("Failed to initialize scripthash subscription, "+
			"falling back to polling only: %v", err)
	} else {
		c.scripthashSub = sub
		c.scripthashNotif = notifChan

		c.wg.Add(1)
		go c.scripthashHandler()
	}

	// Start MWEB sync in a background goroutine.
	if c.dataDir != "" {
		c.wg.Add(1)
		go c.startMwebSync()
	} else {
		// MWEB not configured — signal startup "done" so
		// IsMwebSynced doesn't block callers indefinitely.
		c.mwebStartDone.Store(true)
	}

	// Send ClientConnected notification. This triggers the wallet to
	// start the sync process by calling syncWithChain, which will
	// call Rescan() to begin historical scanning and address
	// subscription setup.
	log.Infof("Sending ClientConnected notification to trigger wallet sync")
	if !c.sendNotification(chain.ClientConnected{}) {
		return errors.New("client shutting down")
	}

	return nil
}

// Stop shuts down the chain client.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) Stop() {
	if atomic.AddInt32(&c.stopped, 1) != 1 {
		return
	}

	log.Info("Stopping Electrum chain client")

	close(c.quit)
	c.wg.Wait()

	close(c.notificationChan)
}

// WaitForShutdown blocks until the client has finished shutting down.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) WaitForShutdown() {
	c.wg.Wait()
}

// ReplayMwebUtxos replays all known MWEB UTXOs through the notification
// pipeline by reading the leafset and fetching leaves in batches.
//
// NOTE: This is part of the chain.MwebReplayer interface.
func (c *ChainClient) ReplayMwebUtxos() error {
	c.mwebMtx.RLock()
	defer c.mwebMtx.RUnlock()

	if c.mwebCoinDB == nil {
		return fmt.Errorf("MWEB sync not initialized")
	}

	leafset, err := c.mwebCoinDB.GetLeafset()
	if err != nil {
		return err
	}

	// Batch to avoid memory spikes on large UTXO sets.
	const batchSize = 5000
	var batch []uint64
	for i := uint64(0); i < leafset.Size; i++ {
		if !leafset.Contains(i) {
			continue
		}
		batch = append(batch, i)
		if len(batch) >= batchSize {
			utxos, err := c.mwebCoinDB.FetchLeaves(batch)
			if err != nil {
				return err
			}
			c.sendNotification(chain.MwebUtxos{
				Leafset: leafset, Utxos: utxos,
			})
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		utxos, err := c.mwebCoinDB.FetchLeaves(batch)
		if err != nil {
			return err
		}
		c.sendNotification(chain.MwebUtxos{
			Leafset: leafset, Utxos: utxos,
		})
	}
	return nil
}

// MwebUtxoExists checks if a MWEB UTXO with the given output ID exists
// in the coin database.
//
// NOTE: This is part of the chain.MwebUtxoChecker interface.
func (c *ChainClient) MwebUtxoExists(hash *chainhash.Hash) (bool, error) {
	c.mwebMtx.RLock()
	defer c.mwebMtx.RUnlock()

	if c.mwebCoinDB == nil {
		return false, fmt.Errorf("MWEB sync not initialized")
	}
	_, err := c.mwebCoinDB.FetchCoin(hash)
	if err == mwebdb.ErrCoinNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// IsMwebSynced returns whether MWEB is caught up with the chain tip.
// If MWEB startup failed or was never configured, returns true once
// startup is done so callers don't wait forever; they'll get a clear
// error from ReplayMwebUtxos when they try to use it.
func (c *ChainClient) IsMwebSynced() bool {
	c.mwebMtx.RLock()
	syncer := c.mwebSyncer
	c.mwebMtx.RUnlock()

	if syncer == nil {
		// Startup finished but syncer is nil → failed or not configured.
		// Return true to unblock callers; ReplayMwebUtxos will error.
		return c.mwebStartDone.Load()
	}
	return syncer.IsSynced()
}

// GetBestBlock returns the hash and height of the best known block.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) GetBestBlock() (*chainhash.Hash, int32, error) {
	c.bestBlockMtx.RLock()
	defer c.bestBlockMtx.RUnlock()

	hash := c.bestBlock.Hash
	return &hash, c.bestBlock.Height, nil
}

// GetBlock returns the raw block from the server given its hash.
//
// NOTE: Electrum protocol does not support full blocks directly. If a REST
// API URL was configured (for mempool/electrs), this method will use that
// to fetch full blocks. Otherwise, it returns an error.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) GetBlock(hash *chainhash.Hash) (*wire.MsgBlock, error) {
	// If we have a REST client configured, use it to fetch the block.
	if c.restClient != nil {
		ctx, cancel := context.WithTimeout(
			context.Background(), defaultRequestTimeout,
		)
		defer cancel()

		block, err := c.restClient.GetBlock(ctx, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch block via REST: %w", err)
		}

		return block.MsgBlock(), nil
	}

	// Electrum servers cannot serve full blocks. This is a fundamental
	// limitation of the protocol.
	return nil, ErrFullBlocksNotSupported
}

// GetTxIndex returns the index of a transaction within a block at the given height.
// This is needed for constructing proper ShortChannelIDs.
// Returns the TxIndex and the block hash.
func (c *ChainClient) GetTxIndex(height int64, txid string) (uint32, string, error) {
	if c.restClient == nil {
		// Without REST API, we can't determine the TxIndex.
		// Return 0 as a fallback (will cause validation failures for
		// channels where the funding tx is not at index 0).
		return 0, "", nil
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), defaultRequestTimeout,
	)
	defer cancel()

	return c.restClient.GetTxIndexByHeight(ctx, height, txid)
}

// GetBlockHash returns the hash of the block at the given height.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) GetBlockHash(height int64) (*chainhash.Hash, error) {
	// Check cache first.
	c.heightToHashMtx.RLock()
	if hash, ok := c.heightToHash[int32(height)]; ok {
		c.heightToHashMtx.RUnlock()
		log.Tracef("GetBlockHash: height %d found in cache: %s", height, hash)
		return hash, nil
	}
	c.heightToHashMtx.RUnlock()

	log.Debugf("GetBlockHash: fetching height %d from server", height)

	// Optimization: Try to batch-fetch a range of headers around this height.
	// This significantly improves performance for sequential access patterns
	// (e.g., graph builder validation, wallet rescans).
	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	// Determine how many headers to fetch ahead.
	const readAheadCount = 2000
	c.bestBlockMtx.RLock()
	bestHeight := c.bestBlock.Height
	c.bestBlockMtx.RUnlock()

	// Calculate batch size, don't exceed best known height.
	batchCount := readAheadCount
	if int32(height)+int32(batchCount) > bestHeight {
		batchCount = int(bestHeight - int32(height) + 1)
	}

	// Only batch if we're fetching 2+ headers and not at the tip.
	if batchCount >= 2 {
		log.Debugf("GetBlockHash: batch-fetching %d headers starting at %d "+
			"(read-ahead optimization)", batchCount, height)

		headers, err := c.client.GetBlockHeaders(
			ctx, uint32(height), uint32(batchCount),
		)
		if err == nil {
			// Successfully fetched batch - cache all headers.
			log.Debugf("GetBlockHash: cached %d headers [%d-%d]",
				len(headers), height, int64(height)+int64(len(headers))-1)

			for i, header := range headers {
				h := int32(height) + int32(i)
				hash := header.BlockHash()
				c.cacheHeader(h, &hash, header)
			}

			// Return the originally requested hash.
			c.heightToHashMtx.RLock()
			hash, ok := c.heightToHash[int32(height)]
			c.heightToHashMtx.RUnlock()

			if ok {
				return hash, nil
			}
		} else {
			log.Debugf("GetBlockHash: batch fetch failed, falling back "+
				"to single header: %v", err)
		}
	}

	// Fallback: fetch single header.
	header, err := c.client.GetBlockHeader(ctx, uint32(height))
	if err != nil {
		log.Errorf("GetBlockHash: failed to get header at height %d: %v", height, err)
		return nil, fmt.Errorf("failed to get block header at "+
			"height %d: %w", height, err)
	}

	hash := header.BlockHash()
	log.Debugf("GetBlockHash: height %d -> hash %s", height, hash)

	// Cache the result.
	c.cacheHeader(int32(height), &hash, header)

	return &hash, nil
}

// GetBlockHeader returns the block header for the given hash.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) GetBlockHeader(
	hash *chainhash.Hash) (*wire.BlockHeader, error) {

	// Check cache first.
	c.headerCacheMtx.RLock()
	if header, ok := c.headerCache[*hash]; ok {
		c.headerCacheMtx.RUnlock()
		return header, nil
	}
	c.headerCacheMtx.RUnlock()

	// We need to find the height for this hash. Search backwards from
	// best block.
	c.bestBlockMtx.RLock()
	bestHeight := c.bestBlock.Height
	c.bestBlockMtx.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Search for the block by iterating through recent heights.
	const maxSearchDepth = 1000
	startHeight := bestHeight
	if startHeight > maxSearchDepth {
		startHeight = bestHeight - maxSearchDepth
	} else {
		startHeight = 0
	}

	for height := bestHeight; height >= startHeight; height-- {
		header, err := c.client.GetBlockHeader(ctx, uint32(height))
		if err != nil {
			continue
		}

		headerHash := header.BlockHash()
		c.cacheHeader(height, &headerHash, header)

		if headerHash.IsEqual(hash) {
			return header, nil
		}

		if height == 0 {
			break
		}
	}

	return nil, ErrBlockNotFound
}

// IsCurrent returns true if the chain client believes it is synced with the
// network.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) IsCurrent() bool {
	bestHash, _, err := c.GetBestBlock()
	if err != nil {
		return false
	}

	bestHeader, err := c.GetBlockHeader(bestHash)
	if err != nil {
		return false
	}

	// Consider ourselves current if the best block is within 2 hours.
	return time.Since(bestHeader.Timestamp) < 2*time.Hour
}

// FilterBlocks scans the blocks contained in the FilterBlocksRequest for any
// addresses of interest. It iterates blocks one at a time and returns on the
// first match, matching the contract expected by the wallet recovery loop
// (same as neutrino's FilterBlocks).
//
// The implementation queries all address histories up front (cached), builds
// an index by block height, then iterates req.Blocks sequentially. On the
// first block that contains relevant transactions, it fetches the txs, runs
// BlockFilterer, and returns immediately.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) FilterBlocks(
	req *chain.FilterBlocksRequest) (*chain.FilterBlocksResponse, error) {

	// Only scan BIP84 (native segwit P2WPKH) addresses via Electrum.
	// Other address types are either unsupported by the Electrum protocol
	// (MWEB), handled by a separate sync mechanism (mwebsync), or not
	// relevant for on-chain recovery (LND lightning keys, BIP49Plus
	// change-only P2WPKH that can't be discovered without the
	// corresponding P2SH external addresses).
	var p2wpkhAddrs []ltcutil.Address
	for si, addr := range req.ExternalAddrs {
		if si.Scope == waddrmgr.KeyScopeBIP0084 {
			p2wpkhAddrs = append(p2wpkhAddrs, addr)
		}
	}
	for si, addr := range req.InternalAddrs {
		if si.Scope == waddrmgr.KeyScopeBIP0084 {
			p2wpkhAddrs = append(p2wpkhAddrs, addr)
		}
	}

	log.Infof("FilterBlocks: scanning %d blocks for %d BIP84 addresses "+
		"(of %d total)", len(req.Blocks), len(p2wpkhAddrs),
		len(req.ExternalAddrs)+len(req.InternalAddrs))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Phase 1: Collect all address histories concurrently and build an
	// index of tx hashes by block height.
	var (
		heightMu    sync.Mutex
		heightIndex = make(map[int32][]string)
		scanned     int64
		total       = int64(len(p2wpkhAddrs))
	)

	semaphore := make(chan struct{}, addressScanConcurrency)
	var wg sync.WaitGroup

	for _, addr := range p2wpkhAddrs {
		wg.Add(1)
		go func(a ltcutil.Address) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			localIndex := make(map[int32][]string)
			c.indexAddressHistory(ctx, a, localIndex)

			heightMu.Lock()
			for h, txs := range localIndex {
				heightIndex[h] = append(heightIndex[h], txs...)
			}
			heightMu.Unlock()

			n := atomic.AddInt64(&scanned, 1)
			if n%500 == 0 || n == total {
				log.Infof("FilterBlocks: queried %d/%d "+
					"addresses", n, total)
			}
		}(addr)
	}
	wg.Wait()

	// Phase 2: Iterate blocks in order, return on first match.
	blockFilterer := chain.NewBlockFilterer(c.chainParams, req)

	for i, blk := range req.Blocks {
		entries, ok := heightIndex[blk.Height]
		if !ok {
			continue
		}

		// Deduplicate tx hashes for this block height.
		seen := make(map[string]struct{})
		var uniqueTxs []*wire.MsgTx
		for _, txHashStr := range entries {
			if _, dup := seen[txHashStr]; dup {
				continue
			}
			seen[txHashStr] = struct{}{}

			txHash, err := chainhash.NewHashFromStr(txHashStr)
			if err != nil {
				continue
			}

			tx, err := c.client.GetTransactionMsgTx(ctx, txHash)
			if err != nil {
				log.Warnf("FilterBlocks: failed to get tx %s: %v",
					txHashStr, err)
				continue
			}
			uniqueTxs = append(uniqueTxs, tx)
		}

		if len(uniqueTxs) == 0 {
			continue
		}

		// Build a synthetic block and run it through the filterer.
		syntheticBlock := &wire.MsgBlock{
			Transactions: uniqueTxs,
		}

		if !blockFilterer.FilterBlock(syntheticBlock) {
			continue
		}

		log.Infof("FilterBlocks: match at block %d (index %d) "+
			"with %d relevant txns", blk.Height, i,
			len(blockFilterer.RelevantTxns))

		return &chain.FilterBlocksResponse{
			BatchIndex:         uint32(i),
			BlockMeta:          blk,
			FoundExternalAddrs: blockFilterer.FoundExternal,
			FoundInternalAddrs: blockFilterer.FoundInternal,
			FoundOutPoints:     blockFilterer.FoundOutPoints,
			RelevantTxns:       blockFilterer.RelevantTxns,
		}, nil
	}

	log.Infof("FilterBlocks: complete, no relevant transactions found")
	return nil, nil
}

// indexAddressHistory fetches the history for an address (using the cache) and
// adds entries to the height index. This is a helper for FilterBlocks.
func (c *ChainClient) indexAddressHistory(ctx context.Context,
	addr ltcutil.Address, heightIndex map[int32][]string) {

	pkScript, err := scriptFromAddress(addr, c.chainParams)
	if err != nil {
		return
	}

	scripthash := ScripthashFromScript(pkScript)
	history, err := c.getHistoryWithCache(ctx, scripthash)
	if err != nil {
		log.Warnf("FilterBlocks: failed to get history for %s: %v",
			addr.EncodeAddress(), err)
		return
	}

	for _, item := range history {
		if item.Height <= 0 {
			continue
		}
		heightIndex[int32(item.Height)] = append(
			heightIndex[int32(item.Height)], item.Hash,
		)
	}
}

// clearHistoryCache clears the address history cache. Should be called:
// - At the start of a new rescan to ensure fresh data
// - After syncing completes to free memory
// - When a reorg is detected to invalidate cached data
func (c *ChainClient) clearHistoryCache() {
	c.historyCacheMtx.Lock()
	defer c.historyCacheMtx.Unlock()

	numEntries := len(c.historyCache)
	if numEntries > 0 {
		log.Debugf("Clearing history cache with %d entries", numEntries)
		c.historyCache = make(map[string][]*electrum.GetMempoolResult)
	}
}

// getHistoryWithCache retrieves address history, using cache to avoid
// redundant queries during wallet sync. This reduces queries from 1 million
// to 5000 for a 400k block sync.
func (c *ChainClient) getHistoryWithCache(ctx context.Context,
	scripthash string) ([]*electrum.GetMempoolResult, error) {

	// Check cache first.
	c.historyCacheMtx.RLock()
	if cached, ok := c.historyCache[scripthash]; ok {
		c.historyCacheMtx.RUnlock()
		log.Tracef("getHistoryWithCache: cache hit for %s", scripthash)
		return cached, nil
	}
	c.historyCacheMtx.RUnlock()

	// Cache miss - fetch from server.
	log.Tracef("getHistoryWithCache: cache miss, fetching %s", scripthash)
	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		return nil, err
	}

	// Store in cache.
	c.historyCacheMtx.Lock()
	c.historyCache[scripthash] = history
	c.historyCacheMtx.Unlock()

	return history, nil
}

// BlockStamp returns the latest block notified by the client.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) BlockStamp() (*waddrmgr.BlockStamp, error) {
	c.bestBlockMtx.RLock()
	defer c.bestBlockMtx.RUnlock()

	stamp := c.bestBlock
	return &stamp, nil
}

// SendRawTransaction submits a raw transaction to the server which will then
// relay it to the Bitcoin network.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) SendRawTransaction(tx *wire.MsgTx,
	allowHighFees bool) (*chainhash.Hash, error) {

	ctx, cancel := context.WithTimeout(context.Background(), defaultRequestTimeout)
	defer cancel()

	return c.client.BroadcastTx(ctx, tx)
}

// GetUtxo returns the original output referenced by the passed outpoint if it
// is still unspent. This uses Electrum's listunspent RPC to check if the
// output exists.
func (c *ChainClient) GetUtxo(op *wire.OutPoint, pkScript []byte,
	heightHint uint32, cancel <-chan struct{}) (*wire.TxOut, error) {

	// Convert the pkScript to a scripthash for Electrum query.
	scripthash := ScripthashFromScript(pkScript)

	ctx, ctxCancel := context.WithTimeout(
		context.Background(), defaultRequestTimeout,
	)
	defer ctxCancel()

	// Query unspent outputs for this scripthash.
	unspent, err := c.client.ListUnspent(ctx, scripthash)
	if err != nil {
		return nil, fmt.Errorf("failed to list unspent: %w", err)
	}

	// Search for our specific outpoint in the unspent list.
	for _, utxo := range unspent {
		if utxo.Hash == op.Hash.String() &&
			utxo.Position == op.Index {

			// Found the UTXO - it's unspent.
			return &wire.TxOut{
				Value:    int64(utxo.Value),
				PkScript: pkScript,
			}, nil
		}
	}

	// Not found in unspent list. Check if it exists at all by looking at
	// the transaction history.
	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	// Check if any transaction in history matches our outpoint's tx.
	for _, histItem := range history {
		if histItem.Hash == op.Hash.String() {
			// The transaction exists but the output is not in the
			// unspent list, meaning it has been spent.
			return nil, ErrOutputSpent
		}
	}

	// Output was never found.
	return nil, ErrOutputNotFound
}

// Rescan rescans the chain for transactions paying to the given addresses.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) Rescan(startHash *chainhash.Hash,
	addrs []ltcutil.Address,
	outpoints map[wire.OutPoint]ltcutil.Address) error {

	// Only monitor P2WPKH addresses via Electrum; other types (legacy,
	// P2SH, taproot, MWEB) are either unsupported by the Electrum
	// protocol or handled by a separate sync mechanism (mwebsync).
	filtered := make([]ltcutil.Address, 0, len(addrs))
	for _, addr := range addrs {
		if _, ok := addr.(*ltcutil.AddressWitnessPubKeyHash); ok {
			filtered = append(filtered, addr)
		}
	}
	addrs = filtered

	log.Infof("Starting rescan from block %s with %d addresses and "+
		"%d outpoints", startHash, len(addrs), len(outpoints))

	// Log all addresses being watched for debugging.
	for i, addr := range addrs {
		log.Debugf("Rescan address %d: %s", i, addr.EncodeAddress())
	}

	// Store watched addresses and outpoints.
	c.watchedAddrsMtx.Lock()
	for _, addr := range addrs {
		c.watchedAddrs[addr.EncodeAddress()] = addr
	}
	c.watchedAddrsMtx.Unlock()

	c.watchedOutpointsMtx.Lock()
	for op, addr := range outpoints {
		c.watchedOutpoints[op] = addr
	}
	c.watchedOutpointsMtx.Unlock()

	// Resolve the rescan start height before subscribing so scripthash
	// notifications can skip confirmed transactions the wallet already has.
	startHeader, err := c.GetBlockHeader(startHash)
	if err != nil {
		return fmt.Errorf("failed to get start block header: %w", err)
	}

	// Get start height by searching for the hash.
	startHeight := int32(0)
	resolved := false
	c.heightToHashMtx.RLock()
	for height, hash := range c.heightToHash {
		if hash.IsEqual(startHash) {
			startHeight = height
			resolved = true
			break
		}
	}
	c.heightToHashMtx.RUnlock()

	// Only bound the scripthash replay when the synced height was resolved
	// exactly. The timestamp estimate below can land above the wallet's
	// true synced height, so when the hash is unresolved leave the bound at
	// 0 (replay everything) rather than risk skipping transactions the
	// wallet still needs.
	if resolved {
		c.rescanStartHeight.Store(startHeight)
	} else {
		c.rescanStartHeight.Store(0)
	}

	// If we didn't find it, estimate from timestamp.
	if startHeight == 0 && startHeader != nil {
		c.bestBlockMtx.RLock()
		bestHeight := c.bestBlock.Height
		bestTime := c.bestBlock.Timestamp
		c.bestBlockMtx.RUnlock()

		// Rough estimate: 10 minutes per block.
		timeDiff := bestTime.Sub(startHeader.Timestamp)
		blockDiff := int32(timeDiff.Minutes() / 10)
		startHeight = bestHeight - blockDiff
		if startHeight < 0 {
			startHeight = 0
		}
	}

	// Subscribe to scripthash notifications for real-time monitoring.
	// This sets up persistent subscriptions that will fire whenever an
	// address receives a new transaction (confirmed or unconfirmed).
	ctx, cancelSub := context.WithTimeout(
		context.Background(), 5*time.Minute,
	)
	c.subscribeAddresses(ctx, addrs)
	cancelSub()

	// CRITICAL: Run rescan asynchronously to prevent deadlock.
	// The wallet expects Rescan() to return immediately and will read
	// notifications asynchronously. If we block here, we create a deadlock:
	// - This function tries to send notifications
	// - Channel fills up
	// - Wallet is waiting for this function to return before reading notifications
	// - Classic producer-consumer deadlock
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		log.Infof("Rescan goroutine started for %d addresses from "+
			"height %d (using %d concurrent workers)",
			len(addrs), startHeight, addressScanConcurrency)

		// Phase 1: Collect transactions from all addresses concurrently.
		// Using a shared map for automatic deduplication by txid.
		var (
			collectedMu sync.Mutex
			collected   = make(map[chainhash.Hash]*collectedTx)
		)

		semaphore := make(chan struct{}, addressScanConcurrency)
		var wg sync.WaitGroup

		for i, addr := range addrs {
			select {
			case <-c.quit:
				log.Info("Rescan aborted due to client shutdown")
				return
			default:
			}

			wg.Add(1)
			go func(address ltcutil.Address, addrNum int) {
				defer wg.Done()

				// Acquire semaphore slot.
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				// Log progress periodically.
				if addrNum%100 == 0 {
					log.Debugf("Rescan: processing address %d/%d",
						addrNum, len(addrs))
				}

				txs, err := c.collectAddressHistory(
					ctx, address, startHeight,
				)
				if err != nil {
					log.Warnf("Failed to scan address %s: %v",
						address, err)
					return
				}

				collectedMu.Lock()
				for _, tx := range txs {
					collected[tx.txRecord.Hash] = tx
				}
				collectedMu.Unlock()
			}(addr, i)
		}

		// Wait for all address scans to complete.
		wg.Wait()

		// Phase 2: Sort topologically and send as
		// FilteredBlockConnected notifications. This ensures funding
		// transactions are processed before spending transactions,
		// which is required for ltcwallet to properly track credits
		// and debits (and thus calculate correct amounts/fees).
		allTxs := make([]*collectedTx, 0, len(collected))
		for _, tx := range collected {
			allTxs = append(allTxs, tx)
		}

		log.Infof("Rescan collected %d unique transactions, "+
			"sorting and sending", len(allTxs))

		sorted := sortAndGroupTxs(allTxs)
		c.sendFilteredBlocks(sorted)

		// Send rescan finished notification.
		c.bestBlockMtx.RLock()
		bestBlock := c.bestBlock
		c.bestBlockMtx.RUnlock()

		log.Infof("Rescan complete, sending RescanFinished notification")

		c.sendNotification(&chain.RescanFinished{
			Hash:   &bestBlock.Hash,
			Height: bestBlock.Height,
			Time:   bestBlock.Timestamp,
		})

		log.Infof("Rescan goroutine finished")
	}()

	// Return immediately so wallet can start reading notifications.
	return nil
}

// collectAddressHistory scans the history of an address from the given start
// height and returns the discovered transactions instead of sending
// notifications directly. This allows the caller to collect transactions from
// multiple addresses, deduplicate, sort them in proper causal order, and send
// them as FilteredBlockConnected notifications.
func (c *ChainClient) collectAddressHistory(ctx context.Context,
	addr ltcutil.Address, startHeight int32) ([]*collectedTx, error) {

	pkScript, err := scriptFromAddress(addr, c.chainParams)
	if err != nil {
		return nil, err
	}

	scripthash := ScripthashFromScript(pkScript)

	log.Tracef("Scanning history for address %s (scripthash: %s) from height %d",
		addr.EncodeAddress(), scripthash, startHeight)

	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		return nil, err
	}

	log.Tracef("Found %d history items for address %s",
		len(history), addr.EncodeAddress())

	// First pass: collect all heights we'll need.
	var neededHeights []int32
	for _, histItem := range history {
		// Skip unconfirmed and historical transactions.
		if histItem.Height <= 0 || int32(histItem.Height) < startHeight {
			continue
		}
		neededHeights = append(neededHeights, int32(histItem.Height))
	}

	// Pre-fetch all needed headers in batch.
	c.ensureHeadersCached(ctx, neededHeights)

	// Second pass: collect transactions with cached headers.
	var result []*collectedTx
	for _, histItem := range history {
		log.Tracef("History item: txid=%s height=%d",
			histItem.Hash, histItem.Height)

		// Skip unconfirmed and historical transactions.
		if histItem.Height <= 0 || int32(histItem.Height) < startHeight {
			log.Tracef("Skipping tx %s: height=%d < startHeight=%d or unconfirmed",
				histItem.Hash, histItem.Height, startHeight)
			continue
		}

		txHash, err := chainhash.NewHashFromStr(histItem.Hash)
		if err != nil {
			continue
		}

		tx, err := c.client.GetTransactionMsgTx(ctx, txHash)
		if err != nil {
			log.Warnf("Failed to get transaction %s: %v",
				histItem.Hash, err)
			continue
		}

		height := int32(histItem.Height)

		// Get block hash for this height.
		blockHash, err := c.GetBlockHash(int64(height))
		if err != nil {
			log.Warnf("Failed to get block hash for height %d: %v",
				height, err)
			continue
		}

		// Get block header for the timestamp.
		blockHeader, err := c.GetBlockHeader(blockHash)
		if err != nil {
			log.Warnf("Failed to get block header for %s: %v",
				blockHash, err)
			continue
		}

		log.Debugf("Collected tx %s at height %d for address %s",
			txHash, height, addr.EncodeAddress())

		result = append(result, &collectedTx{
			txRecord: &wtxmgr.TxRecord{
				MsgTx:    *tx,
				Hash:     *txHash,
				Received: blockHeader.Timestamp,
			},
			height:    height,
			blockHash: *blockHash,
			blockTime: blockHeader.Timestamp,
		})
	}

	return result, nil
}

// sendFilteredBlocks takes a topologically-sorted slice of collected
// transactions, groups them by block height, and sends one
// FilteredBlockConnected notification per block. This ensures that ltcwallet
// processes all transactions in a block atomically and in the correct causal
// order, matching the behavior of the bitcoind and neutrino backends.
func (c *ChainClient) sendFilteredBlocks(txs []*collectedTx) {
	if len(txs) == 0 {
		return
	}

	log.Infof("Sending %d transactions as FilteredBlockConnected "+
		"notifications", len(txs))

	groupStart := 0
	for i := 1; i <= len(txs); i++ {
		if i == len(txs) || txs[i].height != txs[groupStart].height {
			// Flush this block's transactions.
			block := &wtxmgr.BlockMeta{
				Block: wtxmgr.Block{
					Hash:   txs[groupStart].blockHash,
					Height: txs[groupStart].height,
				},
				Time: txs[groupStart].blockTime,
			}

			records := make([]*wtxmgr.TxRecord, 0, i-groupStart)
			for j := groupStart; j < i; j++ {
				records = append(records, txs[j].txRecord)
			}

			log.Debugf("Sending FilteredBlockConnected for block "+
				"%d with %d transactions", block.Height,
				len(records))

			if !c.sendNotification(chain.FilteredBlockConnected{
				Block:       block,
				RelevantTxs: records,
			}) {
				return
			}

			groupStart = i
		}
	}
}

// NotifyReceived marks the addresses to be monitored for incoming transactions.
// It also scans for any existing transactions to these addresses and sends
// notifications for them.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) NotifyReceived(addrs []ltcutil.Address) error {
	// Only monitor P2WPKH addresses via Electrum; other types (legacy,
	// P2SH, taproot, MWEB) are either unsupported by the Electrum
	// protocol or handled by a separate sync mechanism (mwebsync).
	filtered := make([]ltcutil.Address, 0, len(addrs))
	for _, addr := range addrs {
		if _, ok := addr.(*ltcutil.AddressWitnessPubKeyHash); ok {
			filtered = append(filtered, addr)
		}
	}
	addrs = filtered

	log.Debugf("NotifyReceived called with %d addresses", len(addrs))

	c.watchedAddrsMtx.Lock()
	for _, addr := range addrs {
		log.Tracef("Watching address: %s", addr.EncodeAddress())
		c.watchedAddrs[addr.EncodeAddress()] = addr
	}
	c.watchedAddrsMtx.Unlock()

	// Subscribe to scripthash notifications for real-time monitoring.
	ctx, cancel := context.WithTimeout(
		context.Background(), defaultRequestTimeout,
	)
	c.subscribeAddresses(ctx, addrs)
	cancel()

	// Scan for existing activity on these addresses in a goroutine to avoid
	// blocking. This ensures that if funds were already sent to an address,
	// the wallet will be notified.
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()

		log.Debugf("Starting background scan for %d addresses "+
			"(using %d concurrent workers)",
			len(addrs), addressScanConcurrency)

		ctx, cancel := context.WithTimeout(
			context.Background(), 30*time.Minute,
		)
		defer cancel()

		// Phase 1: Collect transactions from all addresses
		// concurrently, deduplicating by txid.
		var (
			collectedMu sync.Mutex
			collected   = make(map[chainhash.Hash]*collectedTx)
		)

		semaphore := make(chan struct{}, addressScanConcurrency)
		var wg sync.WaitGroup

		for i, addr := range addrs {
			select {
			case <-c.quit:
				log.Info("NotifyReceived scan aborted due to client shutdown")
				return
			default:
			}

			wg.Add(1)
			go func(address ltcutil.Address, addrNum int) {
				defer wg.Done()

				// Acquire semaphore slot.
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				log.Tracef("Scanning address %s for existing transactions",
					address.EncodeAddress())

				txs, err := c.collectAddressHistory(
					ctx, address, 0,
				)
				if err != nil {
					log.Tracef("Failed to scan address %s: %v",
						address.EncodeAddress(), err)
					return
				}

				collectedMu.Lock()
				for _, tx := range txs {
					collected[tx.txRecord.Hash] = tx
				}
				collectedMu.Unlock()
			}(addr, i)
		}

		// Wait for all scans to complete.
		wg.Wait()

		// Phase 2: Sort topologically and send as
		// FilteredBlockConnected notifications.
		allTxs := make([]*collectedTx, 0, len(collected))
		for _, tx := range collected {
			allTxs = append(allTxs, tx)
		}

		if len(allTxs) > 0 {
			log.Debugf("NotifyReceived collected %d unique "+
				"transactions, sorting and sending",
				len(allTxs))

			sorted := sortAndGroupTxs(allTxs)
			c.sendFilteredBlocks(sorted)
		}

		log.Debugf("Finished background scan for %d addresses", len(addrs))
	}()

	return nil
}

// NotifyBlocks starts sending block update notifications to the notification
// channel.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) NotifyBlocks() error {
	c.notifyBlocks.Store(true)
	return nil
}

// Notifications returns a channel that will be sent notifications.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) Notifications() <-chan interface{} {
	return c.notificationChan
}

// BackEnd returns the name of the backend.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) BackEnd() string {
	return electrumBackendName
}

// TestMempoolAccept tests whether a transaction would be accepted to the
// mempool.
//
// NOTE: Electrum does not support this operation.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) TestMempoolAccept(txns []*wire.MsgTx,
	maxFeeRate float64) ([]*TestMempoolAcceptResult, error) {

	// Electrum doesn't support testmempoolaccept. Return nil results
	// which should be interpreted as "unknown" by callers.
	return nil, nil
}

// MapRPCErr maps an error from the underlying RPC client to a chain error.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) MapRPCErr(err error) error {
	return err
}

// notificationHandler processes incoming notifications from the Electrum
// server.
func (c *ChainClient) notificationHandler(
	headerChan <-chan *SubscribeHeadersResult) {

	defer c.wg.Done()

	for {
		select {
		case header, ok := <-headerChan:
			if !ok {
				log.Warn("Header channel closed")
				return
			}

			c.handleNewHeader(header)

		case <-c.quit:
			return
		}
	}
}

// scripthashHandler processes incoming scripthash subscription notifications.
// Each notification indicates that the history of a watched address has changed,
// meaning there are new transactions to fetch. This provides real-time
// transaction detection without polling.
func (c *ChainClient) scripthashHandler() {
	defer c.wg.Done()

	log.Info("Scripthash subscription handler started")

	for {
		select {
		case notif, ok := <-c.scripthashNotif:
			if !ok {
				log.Warn("Scripthash notification channel closed")
				return
			}
			c.handleScripthashNotif(notif)

		case <-c.quit:
			return
		}
	}
}

// handleScripthashNotif processes a single scripthash change notification.
// It fetches the updated history, identifies new transactions, and sends
// appropriate notifications to the wallet.
func (c *ChainClient) handleScripthashNotif(notif *SubscribeNotif) {
	scripthash := notif.Params[0]
	newStatus := notif.Params[1]

	if scripthash == "" {
		return
	}

	// Check if the status actually changed.
	c.scripthashHistMtx.RLock()
	oldStatus := c.scripthashHistory[scripthash]
	c.scripthashHistMtx.RUnlock()

	if oldStatus == newStatus {
		return
	}

	// Look up the address for this scripthash.
	addrStr := ""
	if c.scripthashSub != nil {
		addrStr, _ = c.scripthashSub.GetAddress(scripthash)
	}

	log.Debugf("Scripthash status changed for %s (address: %s)",
		scripthash, addrStr)

	ctx, cancel := context.WithTimeout(
		context.Background(), defaultRequestTimeout,
	)
	defer cancel()

	// Fetch the full history for this scripthash.
	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		log.Warnf("Failed to get history for scripthash %s: %v",
			scripthash, err)
		return
	}

	// Update the stored status.
	c.scripthashHistMtx.Lock()
	c.scripthashHistory[scripthash] = newStatus
	c.scripthashHistMtx.Unlock()

	log.Debugf("Scripthash notification for %s: %d history items",
		addrStr, len(history))

	// Process all transactions in the history. Confirmed transactions well
	// below the rescan start height are skipped because the wallet already
	// holds them; a reorg-safety margin keeps recent blocks in scope so a
	// reorg that re-confirms a transaction there is still delivered.
	skipBelow := c.rescanStartHeight.Load()
	if skipBelow > reorgSafetyDepth {
		skipBelow -= reorgSafetyDepth
	} else {
		skipBelow = 0
	}

	var confirmed []*collectedTx
	for _, histItem := range history {
		if histItem.Height > 0 && int32(histItem.Height) < skipBelow {
			continue
		}

		txHash, err := chainhash.NewHashFromStr(histItem.Hash)
		if err != nil {
			continue
		}

		tx, err := c.client.GetTransactionMsgTx(ctx, txHash)
		if err != nil {
			log.Warnf("Failed to get transaction %s: %v",
				histItem.Hash, err)
			continue
		}

		if histItem.Height <= 0 {
			// Unconfirmed transaction - send as RelevantTx
			// immediately so the wallet can show pending funds.
			log.Debugf("Scripthash notification: unconfirmed tx %s "+
				"for %s", txHash, addrStr)

			rec := &wtxmgr.TxRecord{
				MsgTx:    *tx,
				Hash:     *txHash,
				Received: time.Now(),
			}
			c.sendNotification(chain.RelevantTx{
				TxRecord: rec,
				Block:    nil,
			})
		} else {
			// Confirmed transaction — collect for batch sending.
			height := int32(histItem.Height)

			blockHash, err := c.GetBlockHash(int64(height))
			if err != nil {
				log.Warnf("Failed to get block hash for "+
					"height %d: %v", height, err)
				continue
			}

			blockHeader, err := c.GetBlockHeader(blockHash)
			if err != nil {
				log.Warnf("Failed to get block header "+
					"for %s: %v", blockHash, err)
				continue
			}

			rec := &wtxmgr.TxRecord{
				MsgTx:    *tx,
				Hash:     *txHash,
				Received: blockHeader.Timestamp,
			}
			confirmed = append(confirmed, &collectedTx{
				txRecord:  rec,
				height:    height,
				blockHash: *blockHash,
				blockTime: blockHeader.Timestamp,
			})
		}
	}

	// Send confirmed transactions as FilteredBlockConnected.
	if len(confirmed) > 0 {
		sorted := sortAndGroupTxs(confirmed)
		c.sendFilteredBlocks(sorted)
	}
}

// ensureHeadersCached ensures that headers for the given heights are cached.
// It batch-fetches missing headers efficiently, grouping contiguous ranges.
func (c *ChainClient) ensureHeadersCached(ctx context.Context, heights []int32) {
	if len(heights) == 0 {
		return
	}

	// Filter out heights that are already cached.
	var missingHeights []int32
	c.heightToHashMtx.RLock()
	for _, height := range heights {
		if _, ok := c.heightToHash[height]; !ok {
			missingHeights = append(missingHeights, height)
		}
	}
	c.heightToHashMtx.RUnlock()

	if len(missingHeights) == 0 {
		return
	}

	// Sort heights to identify contiguous ranges.
	sort := func(heights []int32) {
		for i := 0; i < len(heights); i++ {
			for j := i + 1; j < len(heights); j++ {
				if heights[j] < heights[i] {
					heights[i], heights[j] = heights[j], heights[i]
				}
			}
		}
	}
	sort(missingHeights)

	// Remove duplicates.
	unique := missingHeights[:0]
	for i, h := range missingHeights {
		if i == 0 || h != missingHeights[i-1] {
			unique = append(unique, h)
		}
	}
	missingHeights = unique

	log.Debugf("Pre-caching %d missing headers", len(missingHeights))

	// Batch-fetch contiguous ranges.
	i := 0
	for i < len(missingHeights) {
		startHeight := missingHeights[i]
		endIdx := i

		// Find the end of contiguous sequence.
		for endIdx+1 < len(missingHeights) &&
			missingHeights[endIdx+1] == missingHeights[endIdx]+1 {
			endIdx++
		}

		count := endIdx - i + 1

		// Try batch fetch for ranges of 2+ headers.
		if count > 1 {
			log.Tracef("Batch-fetching %d headers from height %d",
				count, startHeight)

			headers, err := c.client.GetBlockHeaders(
				ctx, uint32(startHeight), uint32(count),
			)
			if err == nil {
				// Cache all fetched headers.
				for j, header := range headers {
					hash := header.BlockHash()
					c.cacheHeader(startHeight+int32(j), &hash, header)
				}
				i = endIdx + 1
				continue
			}

			log.Warnf("Batch fetch failed, falling back to "+
				"single-header fetch: %v", err)
		}

		// Fetch single header (fallback or non-contiguous).
		header, err := c.client.GetBlockHeader(ctx, uint32(startHeight))
		if err == nil {
			hash := header.BlockHash()
			c.cacheHeader(startHeight, &hash, header)
		} else {
			log.Warnf("Failed to fetch header at height %d: %v",
				startHeight, err)
		}

		i++
	}
}

// processHeaderBatch validates and caches a batch of block headers.
// It performs strict chain validation and sends block notifications.
func (c *ChainClient) processHeaderBatch(ctx context.Context,
	headers []*wire.BlockHeader, startHeight int32) error {

	if len(headers) == 0 {
		return nil
	}

	// Get the hash of the previous block (the one before this batch).
	// This is used to validate that the first header in the batch links
	// to our current chain.
	c.bestBlockMtx.RLock()
	expectedPrevHash := c.bestBlock.Hash
	c.bestBlockMtx.RUnlock()

	for i, header := range headers {
		currentHeight := startHeight + int32(i)
		currentHash := header.BlockHash()

		// Validate chain linkage.
		if i == 0 {
			// First header in batch must link to our current best block.
			if !header.PrevBlock.IsEqual(&expectedPrevHash) {
				return fmt.Errorf("batch does not link to local tip: "+
					"expected prevBlock %s, got %s at height %d",
					expectedPrevHash, header.PrevBlock, currentHeight)
			}
		} else {
			// Subsequent headers must link to previous header in batch.
			prevHash := headers[i-1].BlockHash()
			if !header.PrevBlock.IsEqual(&prevHash) {
				return fmt.Errorf("chain split detected: "+
					"expected prevBlock %s, got %s at height %d",
					prevHash, header.PrevBlock, currentHeight)
			}
		}

		// Cache the header.
		c.cacheHeader(currentHeight, &currentHash, header)

		// Update best block.
		c.bestBlockMtx.Lock()
		c.bestBlock = waddrmgr.BlockStamp{
			Height:    currentHeight,
			Hash:      currentHash,
			Timestamp: header.Timestamp,
		}
		c.bestBlockMtx.Unlock()

		// Send block connected notification if requested.
		if c.notifyBlocks.Load() {
			c.sendNotification(chain.BlockConnected{
				Block: wtxmgr.Block{
					Hash:   currentHash,
					Height: currentHeight,
				},
				Time: header.Timestamp,
			})
		}

		// Update expectedPrevHash for next iteration.
		expectedPrevHash = currentHash
	}

	return nil
}

// handleNewHeader processes a new block header notification.
func (c *ChainClient) handleNewHeader(header *SubscribeHeadersResult) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Get previous best height before updating.
	c.bestBlockMtx.RLock()
	prevHeight := c.bestBlock.Height
	prevHash := c.bestBlock.Hash
	c.bestBlockMtx.RUnlock()

	newHeight := int32(header.Height)

	// Check for reorg first.
	if newHeight <= prevHeight && !prevHash.IsEqual(&chainhash.Hash{}) {
		// Fetch the header to check if it's actually a reorg.
		blockHeader, err := c.client.GetBlockHeader(ctx, uint32(header.Height))
		if err != nil {
			log.Errorf("Failed to get block header at height %d: %v",
				header.Height, err)
			return
		}
		hash := blockHeader.BlockHash()

		if !hash.IsEqual(&prevHash) {
			// Potential reorg detected - clear history cache to
			// ensure fresh data on next query.
			log.Warnf("Reorg detected at height %d, clearing history cache",
				newHeight)
			c.clearHistoryCache()

			// Notify disconnected blocks.
			for h := prevHeight; h >= newHeight; h-- {
				c.heightToHashMtx.RLock()
				oldHash := c.heightToHash[h]
				c.heightToHashMtx.RUnlock()

				if oldHash != nil && c.notifyBlocks.Load() {
					c.sendNotification(chain.BlockDisconnected{
						Block: wtxmgr.Block{
							Hash:   *oldHash,
							Height: h,
						},
					})
				}
			}
		}
	}

	// Process blocks from prevHeight+1 to newHeight using batch fetching.
	// This ensures the wallet receives BlockConnected for every block.
	startHeight := prevHeight + 1
	if startHeight < 1 {
		startHeight = 1
	}

	// Sync in batches of batchSize blocks for optimal performance.
	for height := startHeight; height <= newHeight; height += batchSize {
		// Calculate how many headers to fetch in this batch.
		count := batchSize
		remaining := newHeight - height + 1
		if remaining < int32(batchSize) {
			count = int(remaining)
		}

		log.Debugf("Fetching batch of %d headers starting at height %d",
			count, height)

		// Fetch the entire batch in ONE network request.
		headers, err := c.client.GetBlockHeaders(
			ctx, uint32(height), uint32(count),
		)
		if err != nil {
			log.Errorf("Failed to fetch header batch at height %d: %v",
				height, err)

			// Fall back to single-header fetching for this range.
			log.Warnf("Falling back to single-header sync for range "+
				"%d-%d", height, height+int32(count)-1)

			fallbackStartHeight := height
			fallbackEndHeight := height + int32(count) - 1

			for h := height; h < height+int32(count); h++ {
				singleHeader, err := c.client.GetBlockHeader(
					ctx, uint32(h),
				)
				if err != nil {
					log.Errorf("Failed to get header at height %d: %v",
						h, err)
					continue
				}

				// Process single header.
				if err := c.processHeaderBatch(
					ctx, []*wire.BlockHeader{singleHeader}, h,
				); err != nil {
					log.Errorf("Failed to process header at "+
						"height %d: %v", h, err)
					break
				}
			}

			// Check watched addresses for the fallback range.
			c.checkAddressesInRange(ctx, fallbackStartHeight, fallbackEndHeight)
			continue
		}

		// Validate and cache the entire batch.
		if err := c.processHeaderBatch(ctx, headers, height); err != nil {
			log.Errorf("Invalid header batch at height %d: %v",
				height, err)
			// Stop syncing on validation failure to prevent accepting
			// invalid chain data.
			break
		}

		// Check watched addresses for the entire batch range.
		// This is more efficient than checking per-block.
		batchEndHeight := height + int32(len(headers)) - 1
		c.checkAddressesInRange(ctx, height, batchEndHeight)
	}
}

// checkAddressesInRange checks watched addresses for transactions within a
// block height range. It collects all transactions across all addresses,
// deduplicates, sorts them topologically, and sends them as
// FilteredBlockConnected notifications to ensure proper credit/debit tracking.
func (c *ChainClient) checkAddressesInRange(ctx context.Context,
	startHeight, endHeight int32) {

	// Get snapshot of watched addresses.
	c.watchedAddrsMtx.RLock()
	addrs := make([]ltcutil.Address, 0, len(c.watchedAddrs))
	for _, addr := range c.watchedAddrs {
		addrs = append(addrs, addr)
	}
	c.watchedAddrsMtx.RUnlock()

	if len(addrs) == 0 {
		return
	}

	log.Tracef("Checking %d watched addresses for block range %d-%d",
		len(addrs), startHeight, endHeight)

	// Collect all transactions across all addresses, deduplicating by txid.
	collected := make(map[chainhash.Hash]*collectedTx)

	for _, addr := range addrs {
		txs, err := c.collectAddressHistory(ctx, addr, startHeight)
		if err != nil {
			log.Warnf("Failed to scan address %s: %v",
				addr.EncodeAddress(), err)
			continue
		}

		for _, tx := range txs {
			// Filter to only transactions within the requested
			// range (collectAddressHistory uses startHeight but
			// not endHeight).
			if tx.height > endHeight {
				continue
			}
			collected[tx.txRecord.Hash] = tx
		}
	}

	if len(collected) == 0 {
		return
	}

	// Sort topologically and send as FilteredBlockConnected.
	allTxs := make([]*collectedTx, 0, len(collected))
	for _, tx := range collected {
		allTxs = append(allTxs, tx)
	}

	log.Debugf("checkAddressesInRange: collected %d unique transactions "+
		"for range %d-%d", len(allTxs), startHeight, endHeight)

	sorted := sortAndGroupTxs(allTxs)
	c.sendFilteredBlocks(sorted)
}

// cacheHeader adds a header to the cache.
func (c *ChainClient) cacheHeader(height int32, hash *chainhash.Hash,
	header *wire.BlockHeader) {

	c.headerCacheMtx.Lock()
	c.headerCache[*hash] = header
	c.headerCacheMtx.Unlock()

	c.heightToHashMtx.Lock()
	hashCopy := *hash
	c.heightToHash[height] = &hashCopy
	c.heightToHashMtx.Unlock()
}

// subscribeAddresses subscribes to scripthash notifications for the given
// addresses. Only P2WPKH addresses are subscribed since the Electrum server
// only indexes standard address types for Litecoin.
func (c *ChainClient) subscribeAddresses(ctx context.Context,
	addrs []ltcutil.Address) {

	if c.scripthashSub == nil {
		return
	}

	for _, addr := range addrs {
		// Only subscribe P2WPKH addresses.
		if _, ok := addr.(*ltcutil.AddressWitnessPubKeyHash); !ok {
			continue
		}

		pkScript, err := scriptFromAddress(addr, c.chainParams)
		if err != nil {
			continue
		}
		scripthash := ScripthashFromScript(pkScript)

		// Add() subscribes to the scripthash and immediately sends the
		// current status on the notification channel.
		err = c.scripthashSub.Add(
			ctx, scripthash, addr.EncodeAddress(),
		)
		if err != nil {
			log.Warnf("Failed to subscribe to scripthash for "+
				"%s: %v", addr.EncodeAddress(), err)
			continue
		}

		log.Tracef("Subscribed to scripthash for address %s",
			addr.EncodeAddress())
	}
}

// scriptFromAddress creates a pkScript from an address.
func scriptFromAddress(addr ltcutil.Address,
	params *chaincfg.Params) ([]byte, error) {

	return PayToAddrScript(addr)
}

// PayToAddrScript creates a new script to pay to the given address.
func PayToAddrScript(addr ltcutil.Address) ([]byte, error) {
	switch addr := addr.(type) {
	case *ltcutil.AddressPubKeyHash:
		return payToPubKeyHashScript(addr.ScriptAddress())

	case *ltcutil.AddressScriptHash:
		return payToScriptHashScript(addr.ScriptAddress())

	case *ltcutil.AddressWitnessPubKeyHash:
		return payToWitnessPubKeyHashScript(addr.ScriptAddress())

	case *ltcutil.AddressWitnessScriptHash:
		return payToWitnessScriptHashScript(addr.ScriptAddress())

	case *ltcutil.AddressTaproot:
		return payToTaprootScript(addr.ScriptAddress())

	default:
		return nil, fmt.Errorf("unsupported address type: %T", addr)
	}
}

// payToPubKeyHashScript creates a P2PKH script.
func payToPubKeyHashScript(pubKeyHash []byte) ([]byte, error) {
	return []byte{
		0x76, // OP_DUP
		0xa9, // OP_HASH160
		0x14, // Push 20 bytes
		pubKeyHash[0], pubKeyHash[1], pubKeyHash[2], pubKeyHash[3],
		pubKeyHash[4], pubKeyHash[5], pubKeyHash[6], pubKeyHash[7],
		pubKeyHash[8], pubKeyHash[9], pubKeyHash[10], pubKeyHash[11],
		pubKeyHash[12], pubKeyHash[13], pubKeyHash[14], pubKeyHash[15],
		pubKeyHash[16], pubKeyHash[17], pubKeyHash[18], pubKeyHash[19],
		0x88, // OP_EQUALVERIFY
		0xac, // OP_CHECKSIG
	}, nil
}

// payToScriptHashScript creates a P2SH script.
func payToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return []byte{
		0xa9, // OP_HASH160
		0x14, // Push 20 bytes
		scriptHash[0], scriptHash[1], scriptHash[2], scriptHash[3],
		scriptHash[4], scriptHash[5], scriptHash[6], scriptHash[7],
		scriptHash[8], scriptHash[9], scriptHash[10], scriptHash[11],
		scriptHash[12], scriptHash[13], scriptHash[14], scriptHash[15],
		scriptHash[16], scriptHash[17], scriptHash[18], scriptHash[19],
		0x87, // OP_EQUAL
	}, nil
}

// payToWitnessPubKeyHashScript creates a P2WPKH script.
func payToWitnessPubKeyHashScript(pubKeyHash []byte) ([]byte, error) {
	return []byte{
		0x00, // OP_0 (witness version)
		0x14, // Push 20 bytes
		pubKeyHash[0], pubKeyHash[1], pubKeyHash[2], pubKeyHash[3],
		pubKeyHash[4], pubKeyHash[5], pubKeyHash[6], pubKeyHash[7],
		pubKeyHash[8], pubKeyHash[9], pubKeyHash[10], pubKeyHash[11],
		pubKeyHash[12], pubKeyHash[13], pubKeyHash[14], pubKeyHash[15],
		pubKeyHash[16], pubKeyHash[17], pubKeyHash[18], pubKeyHash[19],
	}, nil
}

// payToWitnessScriptHashScript creates a P2WSH script.
func payToWitnessScriptHashScript(scriptHash []byte) ([]byte, error) {
	script := make([]byte, 34)
	script[0] = 0x00 // OP_0 (witness version)
	script[1] = 0x20 // Push 32 bytes
	copy(script[2:], scriptHash)
	return script, nil
}

// payToTaprootScript creates a P2TR script.
func payToTaprootScript(pubKey []byte) ([]byte, error) {
	script := make([]byte, 34)
	script[0] = 0x51 // OP_1 (witness version 1)
	script[1] = 0x20 // Push 32 bytes
	copy(script[2:], pubKey)
	return script, nil
}
