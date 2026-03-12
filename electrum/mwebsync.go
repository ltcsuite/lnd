package electrum

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/ltcsuite/ltcd/ltcutil/mweb"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/ltcwallet/chain"
	"github.com/ltcsuite/ltcwallet/walletdb"
	_ "github.com/ltcsuite/ltcwallet/walletdb/bdb"
	"github.com/ltcsuite/mwebsync"
	"github.com/ltcsuite/mwebsync/electrumadapter"
	"github.com/ltcsuite/neutrino/banman"
	"github.com/ltcsuite/neutrino/mwebdb"
	"github.com/ltcsuite/neutrino/query"

	"github.com/ltcsuite/lnd/electrum/mwebp2p"
)

// startMwebSync initializes and starts the mwebsync syncer for MWEB support.
// It creates a lightweight P2P service for MWEB data queries and wires up the
// mwebsync library to sync MWEB UTXOs.
//
// This is called as a goroutine from Start() and runs until the client stops.
func (c *ChainClient) startMwebSync() {
	defer c.wg.Done()
	defer c.mwebStartDone.Store(true)

	// Ensure the data directory exists.
	if err := os.MkdirAll(c.dataDir, 0700); err != nil {
		log.Errorf("Failed to create MWEB data directory: %v", err)
		return
	}

	// Create the MWEB coin database.
	dbPath := filepath.Join(c.dataDir, "mweb.db")
	db, err := walletdb.Create("bdb", dbPath, true, 60*time.Second)
	if err != nil {
		log.Errorf("Failed to create MWEB database: %v", err)
		return
	}
	defer db.Close()

	coinDB, err := mwebdb.NewCoinStore(db)
	if err != nil {
		log.Errorf("Failed to create MWEB coin store: %v", err)
		return
	}

	// 1. Start the P2P service for MWEB queries.
	p2pCfg := &mwebp2p.Config{
		ChainParams:  c.chainParams,
		ConnectPeers: c.mwebP2PPeers,
	}
	p2pService := mwebp2p.NewService(p2pCfg)
	if err := p2pService.Start(); err != nil {
		log.Errorf("Failed to start MWEB P2P service: %v", err)
		return
	}
	c.p2pService = p2pService

	// 2. Create query.WorkManager using the P2P service's peers.
	workManager := query.NewWorkManager(&query.Config{
		ConnectedPeers: p2pService.ConnectedPeers,
		NewWorker:      query.NewWorker,
		Ranking:        query.NewPeerRanking(),
		OnPeerMaxRetries: func(peerAddr string) {
			log.Warnf("MWEB peer %s exhausted retries, "+
				"banning", peerAddr)
			p2pService.BanPeer(
				peerAddr, banman.ExceededBanThreshold,
			)
		},
	})
	if err := workManager.Start(); err != nil {
		log.Errorf("Failed to start MWEB query work manager: %v", err)
		p2pService.Stop()
		return
	}
	c.workManager = workManager

	// 3. Create electrum adapter instances.
	headerProvider := electrumadapter.NewHeaderProvider(c)

	syncNotifier := electrumadapter.NewSyncNotifier(
		func(ctx context.Context) error {
			// Block until headers are synced by polling IsCurrent().
			for !c.IsCurrent() {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-c.quit:
					return mwebsync.ErrShuttingDown
				case <-time.After(time.Second):
				}
			}
			return nil
		},
		func() uint32 {
			_, height, _ := c.GetBestBlock()
			return uint32(height)
		},
	)

	peerProvider := electrumadapter.NewPeerProvider(p2pService)
	mwebLogger := electrumadapter.NewLogger(log)

	// 4. Create mwebsync configuration.
	mwebCfg := &mwebsync.Config{
		HeaderProvider:  headerProvider,
		SyncNotifier:    syncNotifier,
		QueryDispatcher: workManager,
		PeerProvider:    peerProvider,
		ChainParams:     c.chainParams,
		CoinDatabase:    coinDB,
		Logger:          mwebLogger,
		OnUtxosAdded: func(leafset *mweb.Leafset, utxos []*wire.MwebNetUtxo) {
			c.sendNotification(chain.MwebUtxos{
				Leafset: leafset,
				Utxos:   utxos,
			})
		},
	}

	// 5. Create and start the syncer.
	syncer, err := mwebsync.New(mwebCfg)
	if err != nil {
		log.Errorf("Failed to create mwebsync: %v", err)
		workManager.Stop()
		p2pService.Stop()
		return
	}
	c.mwebMtx.Lock()
	c.mwebSyncer = syncer
	c.mwebMtx.Unlock()

	if err := syncer.Start(); err != nil {
		log.Errorf("Failed to start mwebsync: %v", err)
		c.mwebMtx.Lock()
		c.mwebSyncer = nil
		c.mwebMtx.Unlock()
		workManager.Stop()
		p2pService.Stop()
		return
	}

	// All startup steps succeeded — publish the coin DB for
	// ReplayMwebUtxos and MwebUtxoExists. This must be set AFTER
	// full startup to avoid exposing a DB that gets closed on a
	// later failure path (defer db.Close() would fire but callers
	// would still see non-nil mwebCoinDB).
	c.mwebMtx.Lock()
	c.mwebCoinDB = coinDB
	c.mwebMtx.Unlock()

	log.Info("MWEB sync started successfully")

	c.waitForSyncOrShutdown(syncer)

	workManager.Stop()
	p2pService.Stop()
}

// waitForSyncOrShutdown blocks until either the client is stopped or the
// sync loop exits unexpectedly. On sync loop death, it clears mwebSyncer
// so IsMwebSynced falls through to the mwebStartDone signal. In both
// cases, mwebCoinDB is cleared before returning so callers get a clean
// "not initialized" error instead of hitting a closing DB.
func (c *ChainClient) waitForSyncOrShutdown(syncer mwebSyncer) {
	select {
	case <-c.quit:
		log.Info("Stopping MWEB sync...")
		if err := syncer.Stop(); err != nil {
			log.Errorf("Failed to stop mwebsync: %v", err)
		}
	case <-syncer.Done():
		log.Warnf("MWEB sync loop exited unexpectedly, cleaning up")
		c.mwebMtx.Lock()
		c.mwebSyncer = nil
		c.mwebMtx.Unlock()
	}

	c.mwebMtx.Lock()
	c.mwebCoinDB = nil
	c.mwebMtx.Unlock()
}
