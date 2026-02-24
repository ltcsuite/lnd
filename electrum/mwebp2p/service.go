package mwebp2p

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/ltcsuite/ltcd/chaincfg"
	"github.com/ltcsuite/ltcd/connmgr"
	"github.com/ltcsuite/ltcd/peer"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/neutrino/banman"
	"github.com/ltcsuite/neutrino/query"
)

const (
	// defaultTargetOutbound is the default number of outbound peers.
	defaultTargetOutbound = 4

	// defaultRetryDuration is the default connection retry interval.
	defaultRetryDuration = 10 * time.Second

	// banDuration is the duration a peer is banned for.
	banDuration = 24 * time.Hour

	// defaultUserAgentName is the default user agent name.
	defaultUserAgentName = "lnd-electrum-mweb"

	// defaultUserAgentVersion is the default user agent version.
	defaultUserAgentVersion = "0.1.0"
)

// Config holds configuration for the P2P service.
type Config struct {
	// ChainParams specifies the network parameters.
	ChainParams *chaincfg.Params

	// TargetOutbound is the number of outbound peers. Defaults to 4.
	TargetOutbound int

	// ConnectPeers is an optional list of explicit peers to connect to.
	// If set, DNS seeding is skipped.
	ConnectPeers []string

	// UserAgentName is the user agent name sent during version exchange.
	UserAgentName string

	// UserAgentVersion is the user agent version sent during version exchange.
	UserAgentVersion string

	// Dial is an optional custom dialer function. Defaults to net.Dial.
	Dial func(net.Addr) (net.Conn, error)
}

// peerSubscription holds a peer subscription that is notified about connected
// peers.
type peerSubscription struct {
	peers  chan<- query.Peer
	cancel <-chan struct{}
}

// Service manages lightweight P2P connections to Litecoin nodes for MWEB
// queries. It connects to a small number of peers via DNS seeds and wraps
// them as query.Peer objects.
type Service struct {
	cfg     *Config
	connMgr *connmgr.ConnManager

	// peers tracks all connected peers.
	peersMtx sync.RWMutex
	peers    map[int32]*MwebPeer

	// bannedPeers tracks banned peer addresses with expiry times.
	bannedMtx sync.RWMutex
	banned    map[string]time.Time

	// peerSubscribers receives notifications about new peers.
	subscribersMtx  sync.Mutex
	peerSubscribers []*peerSubscription

	// seedAddrs holds addresses discovered from DNS seeds.
	seedAddrsMtx sync.Mutex
	seedAddrs    []net.Addr
	seedIdx      int

	quit chan struct{}
	wg   sync.WaitGroup
}

// NewService creates a new P2P service.
func NewService(cfg *Config) *Service {
	if cfg.TargetOutbound == 0 {
		cfg.TargetOutbound = defaultTargetOutbound
	}
	if cfg.UserAgentName == "" {
		cfg.UserAgentName = defaultUserAgentName
	}
	if cfg.UserAgentVersion == "" {
		cfg.UserAgentVersion = defaultUserAgentVersion
	}

	return &Service{
		cfg:    cfg,
		peers:  make(map[int32]*MwebPeer),
		banned: make(map[string]time.Time),
		quit:   make(chan struct{}),
	}
}

// Start initializes DNS seeding and begins connecting to peers.
func (s *Service) Start() error {
	log.Info("Starting MWEB P2P service")

	dialer := s.cfg.Dial
	if dialer == nil {
		dialer = func(addr net.Addr) (net.Conn, error) {
			return net.DialTimeout(addr.Network(), addr.String(), 30*time.Second)
		}
	}

	// Seed addresses from DNS or use explicit connect peers.
	if len(s.cfg.ConnectPeers) > 0 {
		for _, addr := range s.cfg.ConnectPeers {
			tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
			if err != nil {
				log.Warnf("Invalid connect peer address %s: %v", addr, err)
				continue
			}
			s.seedAddrs = append(s.seedAddrs, tcpAddr)
		}
	} else {
		s.seedFromDNS()
	}

	cmgrCfg := &connmgr.Config{
		TargetOutbound: uint32(s.cfg.TargetOutbound),
		RetryDuration:  defaultRetryDuration,
		OnConnection:   s.outboundPeerConnected,
		Dial:           dialer,
		GetNewAddress:  s.getNewAddress,
	}

	cmgr, err := connmgr.New(cmgrCfg)
	if err != nil {
		return fmt.Errorf("failed to create connection manager: %w", err)
	}
	s.connMgr = cmgr
	s.connMgr.Start()

	log.Infof("MWEB P2P service started, targeting %d peers",
		s.cfg.TargetOutbound)

	return nil
}

// Stop disconnects all peers and stops the connection manager.
func (s *Service) Stop() error {
	log.Info("Stopping MWEB P2P service")

	close(s.quit)

	if s.connMgr != nil {
		s.connMgr.Stop()
	}

	// Disconnect all peers. This closes the underlying TCP connections,
	// which unblocks peerDoneHandler's WaitForDisconnect call.
	s.peersMtx.Lock()
	for id, mp := range s.peers {
		mp.Disconnect()
		delete(s.peers, id)
	}
	s.peersMtx.Unlock()

	s.wg.Wait()

	log.Info("MWEB P2P service stopped")
	return nil
}

// ConnectedPeers returns a channel that sends all currently connected peers
// and then sends new peers as they connect. The returned cancel function
// stops the subscription.
//
// This matches the signature required by query.Config.ConnectedPeers.
func (s *Service) ConnectedPeers() (<-chan query.Peer, func(), error) {
	s.peersMtx.RLock()
	currentPeers := make([]*MwebPeer, 0, len(s.peers))
	for _, p := range s.peers {
		currentPeers = append(currentPeers, p)
	}
	s.peersMtx.RUnlock()

	cancelChan := make(chan struct{})
	peerChan := make(chan query.Peer, len(currentPeers)+s.cfg.TargetOutbound)

	// Send all currently connected peers.
	for _, p := range currentPeers {
		peerChan <- p
	}

	// Register for future peer connections.
	s.subscribersMtx.Lock()
	s.peerSubscribers = append(s.peerSubscribers, &peerSubscription{
		peers:  peerChan,
		cancel: cancelChan,
	})
	s.subscribersMtx.Unlock()

	return peerChan, func() {
		close(cancelChan)
	}, nil
}

// BanPeer bans a peer address for misbehavior.
func (s *Service) BanPeer(addr string, _ banman.Reason) error {
	s.bannedMtx.Lock()
	s.banned[addr] = time.Now().Add(banDuration)
	s.bannedMtx.Unlock()

	// Disconnect the peer if currently connected.
	s.peersMtx.RLock()
	for _, mp := range s.peers {
		if mp.Addr() == addr {
			mp.Disconnect()
			break
		}
	}
	s.peersMtx.RUnlock()

	log.Infof("Banned MWEB peer %s", addr)
	return nil
}

// spMsg pairs a message with the peer that sent it.
type spMsg struct {
	peer *MwebPeer
	msg  wire.Message
}

// QueryAllPeers queries all connected peers with a message and calls
// checkResponse for each response received. Responses are processed
// sequentially in a single goroutine to match neutrino's queryAllPeers
// pattern, preventing concurrent calls to checkResponse which may close
// shared channels.
func (s *Service) QueryAllPeers(
	queryMsg wire.Message,
	checkResponse func(sp query.Peer, resp wire.Message,
		quit chan<- struct{}, peerQuit chan<- struct{}),
) {
	s.peersMtx.RLock()
	peers := make([]*MwebPeer, 0, len(s.peers))
	for _, p := range s.peers {
		peers = append(peers, p)
	}
	s.peersMtx.RUnlock()

	if len(peers) == 0 {
		return
	}

	// Shared state between per-peer goroutines.
	queryQuit := make(chan struct{})
	allQuit := make(chan struct{})
	var wg sync.WaitGroup
	msgChan := make(chan spMsg)

	// Per-peer quit channels.
	peerQuits := make(map[int32]chan struct{})

	for _, mp := range peers {
		peerQuits[mp.ID()] = make(chan struct{})

		// Subscribe to receive messages from this peer.
		recvChan, cancel := mp.SubscribeRecvMsg()
		wg.Add(1)

		go func(mp *MwebPeer, recvChan <-chan wire.Message, cancel func(), peerQuit <-chan struct{}) {
			defer wg.Done()
			defer cancel()

			// Send the query to this peer.
			mp.QueueMessageWithEncoding(queryMsg, nil, wire.BaseEncoding)

			// Forward received messages to the central msgChan
			// until timeout, quit, or peer quit.
			timeout := time.After(30 * time.Second)
			for {
				select {
				case <-queryQuit:
					return
				case <-peerQuit:
					return
				case <-allQuit:
					return
				case <-timeout:
					return
				case msg := <-recvChan:
					select {
					case msgChan <- spMsg{peer: mp, msg: msg}:
					case <-queryQuit:
						return
					case <-peerQuit:
						return
					case <-allQuit:
						return
					}
				}
			}
		}(mp, recvChan, cancel, peerQuits[mp.ID()])
	}

	// Wait for all per-peer goroutines to finish, then close allQuit.
	go func() {
		wg.Wait()
		close(allQuit)
	}()

	// Process responses sequentially in this goroutine. This prevents
	// concurrent calls to checkResponse which may close shared channels.
checkResponses:
	for {
		select {
		case <-queryQuit:
			break checkResponses
		default:
		}

		select {
		case <-s.quit:
			break checkResponses

		case <-allQuit:
			break checkResponses

		case sm := <-msgChan:
			// Skip if this peer was already told to quit.
			select {
			case <-peerQuits[sm.peer.ID()]:
				continue
			default:
			}

			checkResponse(sm.peer, sm.msg, queryQuit,
				peerQuits[sm.peer.ID()])
		}
	}
}

// isBanned returns true if the given address is currently banned.
func (s *Service) isBanned(addr string) bool {
	s.bannedMtx.RLock()
	defer s.bannedMtx.RUnlock()

	expiry, ok := s.banned[addr]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(s.banned, addr)
		return false
	}
	return true
}

// seedFromDNS discovers peer addresses from DNS seeds.
func (s *Service) seedFromDNS() {
	connmgr.SeedFromDNS(s.cfg.ChainParams, wire.SFNodeNetwork,
		net.LookupIP, func(addrs []*wire.NetAddressV2) {
			s.seedAddrsMtx.Lock()
			defer s.seedAddrsMtx.Unlock()

			for _, addr := range addrs {
				legacyAddr := addr.ToLegacy()
				if legacyAddr == nil {
					continue
				}
				tcpAddr := &net.TCPAddr{
					IP:   legacyAddr.IP,
					Port: int(legacyAddr.Port),
				}
				s.seedAddrs = append(s.seedAddrs, tcpAddr)
			}
			log.Infof("Discovered %d MWEB P2P peer addresses from DNS seeds",
				len(addrs))
		})
}

// getNewAddress returns the next address to connect to from the seed pool.
func (s *Service) getNewAddress() (net.Addr, error) {
	s.seedAddrsMtx.Lock()
	defer s.seedAddrsMtx.Unlock()

	if len(s.seedAddrs) == 0 {
		return nil, fmt.Errorf("no peer addresses available")
	}

	addr := s.seedAddrs[s.seedIdx%len(s.seedAddrs)]
	s.seedIdx++
	return addr, nil
}

// outboundPeerConnected is called by the connection manager when a new
// outbound connection is established.
func (s *Service) outboundPeerConnected(c *connmgr.ConnReq, conn net.Conn) {
	peerAddr := c.Addr.String()

	disconnect := func() {
		s.connMgr.Remove(c.ID())
		go s.connMgr.NewConnReq()
	}

	// Reject banned peers.
	if s.isBanned(peerAddr) {
		disconnect()
		conn.Close()
		return
	}

	// Reject already-connected peers.
	s.peersMtx.RLock()
	for _, mp := range s.peers {
		if mp.Addr() == peerAddr {
			s.peersMtx.RUnlock()
			disconnect()
			conn.Close()
			return
		}
	}
	s.peersMtx.RUnlock()

	mp := NewMwebPeer(nil)

	// Use a channel to know when the version handshake completes.
	verackCh := make(chan struct{})

	peerCfg := &peer.Config{
		Listeners: peer.MessageListeners{
			OnRead: mp.OnRead,
			OnVerAck: func(_ *peer.Peer, _ *wire.MsgVerAck) {
				close(verackCh)
			},
		},
		UserAgentName:    s.cfg.UserAgentName,
		UserAgentVersion: s.cfg.UserAgentVersion,
		ChainParams:      s.cfg.ChainParams,
		Services:         wire.SFNodeNetwork,
		ProtocolVersion:  wire.MwebLightClientVersion,
		DisableRelayTx:   true,
	}

	p, err := peer.NewOutboundPeer(peerCfg, peerAddr)
	if err != nil {
		log.Debugf("Cannot create outbound peer %s: %v", peerAddr, err)
		disconnect()
		conn.Close()
		return
	}

	mp.Peer = p
	p.AssociateConnection(conn)

	// Wait for the version handshake to complete so we can check the
	// peer's protocol version. Peers that don't support MWEB (protocol
	// version < MwebLightClientVersion) will silently drop MWEB messages,
	// causing all queries to timeout.
	select {
	case <-verackCh:
		// Handshake complete.
	case <-time.After(30 * time.Second):
		log.Debugf("Peer %s handshake timeout, disconnecting", peerAddr)
		p.Disconnect()
		disconnect()
		return
	case <-s.quit:
		p.Disconnect()
		return
	}

	// Check that the peer supports the MWEB light client protocol.
	if p.ProtocolVersion() < wire.MwebLightClientVersion {
		log.Debugf("Peer %s protocol version %d < %d (no MWEB support), disconnecting",
			peerAddr, p.ProtocolVersion(), wire.MwebLightClientVersion)
		p.Disconnect()
		disconnect()
		return
	}

	// Register the peer.
	s.peersMtx.Lock()
	s.peers[p.ID()] = mp
	s.peersMtx.Unlock()

	log.Infof("Connected to MWEB P2P peer %s (version=%d)",
		peerAddr, p.ProtocolVersion())

	// Notify subscribers of the new peer.
	s.notifyPeerConnected(mp)

	// Handle peer disconnection.
	s.wg.Add(1)
	go s.peerDoneHandler(mp, c)
}

// notifyPeerConnected sends the peer to all active subscribers.
func (s *Service) notifyPeerConnected(mp *MwebPeer) {
	s.subscribersMtx.Lock()
	defer s.subscribersMtx.Unlock()

	n := 0
	for i, sub := range s.peerSubscribers {
		select {
		case <-sub.cancel:
			s.peerSubscribers[i] = nil
			continue
		default:
		}

		s.peerSubscribers[n] = sub
		n++

		// Non-blocking send in goroutine.
		s.wg.Add(1)
		go func(sub *peerSubscription) {
			defer s.wg.Done()
			select {
			case sub.peers <- mp:
			case <-sub.cancel:
			case <-s.quit:
			}
		}(sub)
	}
	s.peerSubscribers = s.peerSubscribers[:n]
}

// peerDoneHandler waits for a peer to disconnect and cleans up.
func (s *Service) peerDoneHandler(mp *MwebPeer, c *connmgr.ConnReq) {
	defer s.wg.Done()

	mp.WaitForDisconnect()

	log.Infof("MWEB P2P peer %s disconnected", mp.Addr())

	// Remove from peers map.
	s.peersMtx.Lock()
	delete(s.peers, mp.ID())
	s.peersMtx.Unlock()

	// Close the quit channel to notify subscribers.
	select {
	case <-mp.quit:
		// Already closed (e.g. during Stop()).
	default:
		close(mp.quit)
	}

	// Request a new connection to replace this one.
	select {
	case <-s.quit:
	default:
		s.connMgr.Remove(c.ID())
		go s.connMgr.NewConnReq()
	}
}
