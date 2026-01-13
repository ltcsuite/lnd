package electrum

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ltcsuite/ltcd/ltcutil"
	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/chaincfg/chainhash"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/waddrmgr"
	"github.com/ltcsuite/ltcwallet/wtxmgr"
	"github.com/checksum0/go-electrum/electrum"
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

	quit chan struct{}
	wg   sync.WaitGroup
}

// Compile time check to ensure ChainClient implements chain.Interface.
var _ chain.Interface = (*ChainClient)(nil)

// NewChainClient creates a new Electrum chain client.
// If restURL is provided, the client will be able to fetch full blocks
// via the mempool/electrs REST API.
func NewChainClient(client *Client, chainParams *chaincfg.Params,
	restURL string) *ChainClient {

	var restClient *RESTClient
	if restURL != "" {
		restClient = NewRESTClient(restURL)
		log.Infof("Electrum REST API enabled: %s", restURL)
	}

	return &ChainClient{
		client:           client,
		restClient:       restClient,
		chainParams:      chainParams,
		headerCache:      make(map[chainhash.Hash]*wire.BlockHeader),
		heightToHash:     make(map[int32]*chainhash.Hash),
		notificationChan: make(chan interface{}, 2100),
		watchedAddrs:     make(map[string]ltcutil.Address),
		watchedOutpoints: make(map[wire.OutPoint]ltcutil.Address),
		historyCache:     make(map[string][]*electrum.GetMempoolResult),
		quit:             make(chan struct{}),
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

	// Start the notification handler.
	c.wg.Add(1)
	go c.notificationHandler(headerChan)

	// Send ClientConnected notification first. This triggers the wallet to
	// start the sync process by calling syncWithChain.
	log.Infof("Sending ClientConnected notification to trigger wallet sync")
	if !c.sendNotification(chain.ClientConnected{}) {
		return errors.New("client shutting down")
	}

	// Send initial rescan finished notification.
	c.bestBlockMtx.RLock()
	bestBlock := c.bestBlock
	c.bestBlockMtx.RUnlock()

	if !c.sendNotification(&chain.RescanFinished{
		Hash:   &bestBlock.Hash,
		Height: bestBlock.Height,
		Time:   bestBlock.Timestamp,
	}) {
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
// addresses of interest. For each requested block, the corresponding compact
// filter will first be checked for matches, skipping those that do not report
// anything. If the filter returns a positive match, the full block will be
// fetched and filtered for addresses using a block filterer.
//
// NOTE: For Electrum, we use scripthash queries instead of compact filters.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) FilterBlocks(
	req *chain.FilterBlocksRequest) (*chain.FilterBlocksResponse, error) {

	// For Electrum, we can't scan full blocks. Instead, we query the
	// history for each watched address and check if any transactions
	// appeared in the requested block range.

	totalAddrs := len(req.ExternalAddrs) + len(req.InternalAddrs)
	log.Infof("FilterBlocks: scanning %d blocks for %d addresses "+
		"(using %d concurrent workers)",
		len(req.Blocks), totalAddrs, addressScanConcurrency)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Combine all addresses into a single slice for parallel processing.
	allAddrs := make([]ltcutil.Address, 0, totalAddrs)
	for _, addr := range req.ExternalAddrs {
		allAddrs = append(allAddrs, addr)
	}
	for _, addr := range req.InternalAddrs {
		allAddrs = append(allAddrs, addr)
	}

	// Process addresses in parallel using worker pool.
	type addressResult struct {
		txns []*wire.MsgTx
		idx  uint32
		err  error
	}

	results := make(chan addressResult, totalAddrs)
	semaphore := make(chan struct{}, addressScanConcurrency)
	var wg sync.WaitGroup

	// Launch workers for all addresses.
	for i, addr := range allAddrs {
		wg.Add(1)
		go func(address ltcutil.Address, addrNum int) {
			defer wg.Done()

			// Acquire semaphore slot.
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Log progress periodically.
			if addrNum%100 == 0 {
				log.Debugf("FilterBlocks: processing address %d/%d",
					addrNum, totalAddrs)
			}

			txns, idx, err := c.filterAddressInBlocks(
				ctx, address, req.Blocks,
			)
			if err != nil {
				log.Warnf("Failed to filter address %s: %v",
					address, err)
			}

			results <- addressResult{
				txns: txns,
				idx:  idx,
				err:  err,
			}
		}(addr, i)
	}

	// Close results channel when all workers complete.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results from all workers.
	var (
		relevantTxns  []*wire.MsgTx
		batchIndex    uint32
		foundRelevant bool
	)

	for result := range results {
		if result.err != nil {
			continue
		}

		if len(result.txns) > 0 {
			relevantTxns = append(relevantTxns, result.txns...)
			if !foundRelevant || result.idx < batchIndex {
				batchIndex = result.idx
			}
			foundRelevant = true
		}
	}

	if !foundRelevant {
		log.Infof("FilterBlocks: complete, no relevant transactions found")
		return nil, nil
	}

	log.Infof("FilterBlocks: complete, found %d relevant transactions",
		len(relevantTxns))

	return &chain.FilterBlocksResponse{
		BatchIndex:   batchIndex,
		BlockMeta:    req.Blocks[batchIndex],
		RelevantTxns: relevantTxns,
	}, nil
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

// filterAddressInBlocks checks if an address has any activity in the given
// blocks.
func (c *ChainClient) filterAddressInBlocks(ctx context.Context,
	addr ltcutil.Address,
	blocks []wtxmgr.BlockMeta) ([]*wire.MsgTx, uint32, error) {

	pkScript, err := scriptFromAddress(addr, c.chainParams)
	if err != nil {
		return nil, 0, err
	}

	scripthash := ScripthashFromScript(pkScript)

	log.Tracef("filterAddressInBlocks: fetching history for %s", addr.EncodeAddress())
	history, err := c.getHistoryWithCache(ctx, scripthash)
	if err != nil {
		return nil, 0, err
	}

	log.Tracef("filterAddressInBlocks: got %d history items for %s",
		len(history), addr.EncodeAddress())

	var (
		relevantTxns []*wire.MsgTx
		batchIdx     uint32 = ^uint32(0)
	)

	// Build map of block heights for faster lookup
	blockHeights := make(map[int32]int)
	for i, block := range blocks {
		blockHeights[block.Height] = i
	}

	// Collect heights we'll need for header caching
	var neededHeights []int32

	for _, histItem := range history {
		if histItem.Height <= 0 {
			continue
		}

		// Check if this height is in our target blocks
		if idx, ok := blockHeights[int32(histItem.Height)]; ok {
			neededHeights = append(neededHeights, int32(histItem.Height))

			txHash, err := chainhash.NewHashFromStr(
				histItem.Hash,
			)
			if err != nil {
				continue
			}

			// Fetch the transaction.
			tx, err := c.client.GetTransactionMsgTx(
				ctx, txHash,
			)
			if err != nil {
				log.Warnf("Failed to get tx %s: %v",
					histItem.Hash, err)
				continue
			}

			relevantTxns = append(relevantTxns, tx)

			if uint32(idx) < batchIdx {
				batchIdx = uint32(idx)
			}
		}
	}

	// Pre-cache headers for all needed heights
	if len(neededHeights) > 0 {
		c.ensureHeadersCached(ctx, neededHeights)
	}

	log.Tracef("filterAddressInBlocks: found %d relevant txns for %s",
		len(relevantTxns), addr.EncodeAddress())

	return relevantTxns, batchIdx, nil
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

	// Get the start height from the hash.
	startHeader, err := c.GetBlockHeader(startHash)
	if err != nil {
		return fmt.Errorf("failed to get start block header: %w", err)
	}

	// Get start height by searching for the hash.
	startHeight := int32(0)
	c.heightToHashMtx.RLock()
	for height, hash := range c.heightToHash {
		if hash.IsEqual(startHash) {
			startHeight = height
			break
		}
	}
	c.heightToHashMtx.RUnlock()

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

		// Scan addresses in parallel using worker pool.
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

				err := c.scanAddressHistory(ctx, address, startHeight)
				if err != nil {
					log.Warnf("Failed to scan address %s: %v",
						address, err)
				}
			}(addr, i)
		}

		// Wait for all address scans to complete.
		wg.Wait()

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

// scanAddressHistory scans the history of an address from the given start
// height and sends relevant transaction notifications.
func (c *ChainClient) scanAddressHistory(ctx context.Context,
	addr ltcutil.Address, startHeight int32) error {

	pkScript, err := scriptFromAddress(addr, c.chainParams)
	if err != nil {
		return err
	}

	scripthash := ScripthashFromScript(pkScript)

	log.Tracef("Scanning history for address %s (scripthash: %s) from height %d",
		addr.EncodeAddress(), scripthash, startHeight)

	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		return err
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

	// Second pass: process transactions with cached headers.
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

		// Get block hash for this height (should be cached now).
		blockHash, err := c.GetBlockHash(int64(histItem.Height))
		if err != nil {
			log.Warnf("Failed to get block hash for height %d: %v",
				histItem.Height, err)
			continue
		}

		// Send relevant transaction notification.
		log.Debugf("scanAddressHistory: sending RelevantTx for tx %s at height %d",
			txHash, histItem.Height)

		if !c.sendNotification(chain.RelevantTx{
			TxRecord: &wtxmgr.TxRecord{
				MsgTx:    *tx,
				Hash:     *txHash,
				Received: time.Now(),
			},
			Block: &wtxmgr.BlockMeta{
				Block: wtxmgr.Block{
					Hash:   *blockHash,
					Height: int32(histItem.Height),
				},
			},
		}) {
			return nil
		}

	}

	return nil
}

// NotifyReceived marks the addresses to be monitored for incoming transactions.
// It also scans for any existing transactions to these addresses and sends
// notifications for them.
//
// NOTE: This is part of the chain.Interface interface.
func (c *ChainClient) NotifyReceived(addrs []ltcutil.Address) error {
	log.Debugf("NotifyReceived called with %d addresses", len(addrs))

	c.watchedAddrsMtx.Lock()
	for _, addr := range addrs {
		log.Tracef("Watching address: %s", addr.EncodeAddress())
		c.watchedAddrs[addr.EncodeAddress()] = addr
	}
	c.watchedAddrsMtx.Unlock()

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

		// Scan addresses in parallel using worker pool.
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

				if err := c.scanAddressForExistingTxs(ctx, address); err != nil {
					log.Tracef("Failed to scan address %s: %v",
						address.EncodeAddress(), err)
				}
			}(addr, i)
		}

		// Wait for all scans to complete.
		wg.Wait()

		log.Debugf("Finished background scan for %d addresses", len(addrs))
	}()

	return nil
}

// scanAddressForExistingTxs scans the blockchain for existing transactions
// involving the given address and sends notifications for any found.
func (c *ChainClient) scanAddressForExistingTxs(ctx context.Context,
	addr ltcutil.Address) error {

	pkScript, err := scriptFromAddress(addr, c.chainParams)
	if err != nil {
		return err
	}

	scripthash := ScripthashFromScript(pkScript)

	history, err := c.client.GetHistory(ctx, scripthash)
	if err != nil {
		return err
	}

	if len(history) == 0 {
		log.Tracef("No history found for address %s", addr.EncodeAddress())
		return nil
	}

	log.Tracef("Found %d transactions for address %s",
		len(history), addr.EncodeAddress())

	// First pass: collect all heights we'll need.
	var neededHeights []int32
	for _, histItem := range history {
		if histItem.Height > 0 {
			neededHeights = append(neededHeights, int32(histItem.Height))
		}
	}

	// Pre-fetch all needed headers in batch.
	c.ensureHeadersCached(ctx, neededHeights)

	// Second pass: process transactions with cached headers.
	for _, histItem := range history {
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

		var block *wtxmgr.BlockMeta
		if histItem.Height > 0 {
			// Confirmed transaction - get block hash (should be cached now).
			blockHash, err := c.GetBlockHash(int64(histItem.Height))
			if err != nil {
				log.Warnf("Failed to get block hash for height %d: %v",
					histItem.Height, err)
				continue
			}

			block = &wtxmgr.BlockMeta{
				Block: wtxmgr.Block{
					Hash:   *blockHash,
					Height: int32(histItem.Height),
				},
			}
		}

		// Send relevant transaction notification.
		log.Debugf("Sending RelevantTx for tx %s (height=%d)",
			txHash, histItem.Height)

		if !c.sendNotification(chain.RelevantTx{
			TxRecord: &wtxmgr.TxRecord{
				MsgTx:    *tx,
				Hash:     *txHash,
				Received: time.Now(),
			},
			Block: block,
		}) {
			return nil
		}
	}

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
// func (c *ChainClient) TestMempoolAccept(txns []*wire.MsgTx,
// 	maxFeeRate float64) ([]*btcjson.TestMempoolAcceptResult, error) {

// 	// Electrum doesn't support testmempoolaccept. Return nil results
// 	// which should be interpreted as "unknown" by callers.
// 	return nil, nil
// }

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
// block height range. This is optimized for batch processing - it fetches the
// history for each address ONCE and filters locally, rather than querying the
// server repeatedly for each block.
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

	// For each watched address, fetch history once and filter locally.
	for _, addr := range addrs {
		pkScript, err := scriptFromAddress(addr, c.chainParams)
		if err != nil {
			log.Warnf("Failed to get pkScript for address %s: %v",
				addr.EncodeAddress(), err)
			continue
		}

		scripthash := ScripthashFromScript(pkScript)

		// Fetch the ENTIRE history for this address in ONE request.
		history, err := c.client.GetHistory(ctx, scripthash)
		if err != nil {
			log.Warnf("Failed to get history for address %s: %v",
				addr.EncodeAddress(), err)
			continue
		}

		// Collect heights we'll need, then pre-fetch headers.
		var neededHeights []int32
		for _, histItem := range history {
			itemHeight := int32(histItem.Height)

			// Skip unconfirmed and out-of-range transactions.
			if itemHeight <= 0 || itemHeight < startHeight ||
				itemHeight > endHeight {
				continue
			}

			neededHeights = append(neededHeights, itemHeight)
		}

		// Pre-fetch all needed headers in batch before processing.
		c.ensureHeadersCached(ctx, neededHeights)

		// Now process transactions - all headers are cached.
		for _, histItem := range history {
			itemHeight := int32(histItem.Height)

			// Skip unconfirmed and out-of-range transactions.
			if itemHeight <= 0 || itemHeight < startHeight ||
				itemHeight > endHeight {
				continue
			}

			// Found a relevant transaction in this batch range!
			log.Debugf("Found relevant tx %s at height %d for address %s",
				histItem.Hash, itemHeight, addr.EncodeAddress())

			txHash, err := chainhash.NewHashFromStr(histItem.Hash)
			if err != nil {
				log.Warnf("Failed to parse tx hash %s: %v",
					histItem.Hash, err)
				continue
			}

			// Fetch the full transaction.
			tx, err := c.client.GetTransactionMsgTx(ctx, txHash)
			if err != nil {
				log.Warnf("Failed to get transaction %s: %v",
					histItem.Hash, err)
				continue
			}

			// Get block hash for this height (should be cached now).
			blockHash, err := c.GetBlockHash(int64(itemHeight))
			if err != nil {
				log.Warnf("Failed to get block hash for height %d: %v",
					itemHeight, err)
				continue
			}

			// Send relevant transaction notification.
			log.Debugf("Sending RelevantTx for tx %s in block %d",
				txHash, itemHeight)

			if !c.sendNotification(chain.RelevantTx{
				TxRecord: &wtxmgr.TxRecord{
					MsgTx:    *tx,
					Hash:     *txHash,
					Received: time.Now(),
				},
				Block: &wtxmgr.BlockMeta{
					Block: wtxmgr.Block{
						Hash:   *blockHash,
						Height: itemHeight,
					},
				},
			}) {
				return
			}
		}
	}
}

// checkWatchedAddresses checks if any watched addresses have new transactions
// in the given block.
func (c *ChainClient) checkWatchedAddresses(ctx context.Context,
	height int32, blockHash *chainhash.Hash) {

	c.watchedAddrsMtx.RLock()
	addrs := make([]ltcutil.Address, 0, len(c.watchedAddrs))
	for _, addr := range c.watchedAddrs {
		addrs = append(addrs, addr)
	}
	c.watchedAddrsMtx.RUnlock()

	log.Tracef("Checking %d watched addresses for block %d", len(addrs), height)

	for _, addr := range addrs {
		pkScript, err := scriptFromAddress(addr, c.chainParams)
		if err != nil {
			log.Warnf("Failed to get pkScript for address %s: %v",
				addr.EncodeAddress(), err)
			continue
		}

		scripthash := ScripthashFromScript(pkScript)

		log.Tracef("Querying history for address %s (scripthash: %s)",
			addr.EncodeAddress(), scripthash)

		history, err := c.client.GetHistory(ctx, scripthash)
		if err != nil {
			log.Warnf("Failed to get history for address %s: %v",
				addr.EncodeAddress(), err)
			continue
		}

		log.Tracef("Address %s has %d history items",
			addr.EncodeAddress(), len(history))

		for _, histItem := range history {
			log.Tracef("History item for %s: txid=%s height=%d (looking for height %d)",
				addr.EncodeAddress(), histItem.Hash, histItem.Height, height)

			if int32(histItem.Height) != height {
				continue
			}

			log.Debugf("Found relevant tx %s at height %d for address %s",
				histItem.Hash, height, addr.EncodeAddress())

			txHash, err := chainhash.NewHashFromStr(histItem.Hash)
			if err != nil {
				log.Warnf("Failed to parse tx hash %s: %v", histItem.Hash, err)
				continue
			}

			tx, err := c.client.GetTransactionMsgTx(ctx, txHash)
			if err != nil {
				log.Warnf("Failed to get transaction %s: %v", histItem.Hash, err)
				continue
			}

			log.Debugf("Sending RelevantTx for tx %s in block %d",
				txHash, height)

			if !c.sendNotification(chain.RelevantTx{
				TxRecord: &wtxmgr.TxRecord{
					MsgTx:    *tx,
					Hash:     *txHash,
					Received: time.Now(),
				},
				Block: &wtxmgr.BlockMeta{
					Block: wtxmgr.Block{
						Hash:   *blockHash,
						Height: height,
					},
				},
			}) {
				return
			}
		}
	}
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
