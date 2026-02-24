package mwebp2p

import (
	"sync"

	"github.com/ltcsuite/ltcd/peer"
	"github.com/ltcsuite/ltcd/wire"
	"github.com/ltcsuite/neutrino/query"
)

// Compile-time check that MwebPeer implements query.Peer.
var _ query.Peer = (*MwebPeer)(nil)

// msgSubscription sends all messages from a peer over a channel, allowing
// pluggable filtering of the messages.
type msgSubscription struct {
	msgChan  chan<- wire.Message
	quitChan <-chan struct{}
}

// MwebPeer wraps a peer.Peer to implement the query.Peer interface.
// This is a lightweight version of neutrino's ServerPeer, containing only
// the functionality needed for MWEB queries.
type MwebPeer struct {
	*peer.Peer

	// recvSubscribers holds subscriptions for received messages.
	// The sends on these channels WILL NOT block; any messages the channel
	// can't accept will be dropped silently (sent via goroutine).
	recvSubscribers map[msgSubscription]struct{}
	mtxSubscribers  sync.RWMutex

	// quit is closed when the peer disconnects.
	quit chan struct{}
}

// NewMwebPeer creates a new MwebPeer wrapping the given peer.Peer.
func NewMwebPeer(p *peer.Peer) *MwebPeer {
	return &MwebPeer{
		Peer:            p,
		recvSubscribers: make(map[msgSubscription]struct{}),
		quit:            make(chan struct{}),
	}
}

// SubscribeRecvMsg adds an OnRead subscription to the peer. All messages
// received from this peer will be sent on the returned channel. A closure is
// also returned, that should be called to cancel the subscription.
//
// NOTE: Part of the query.Peer interface.
func (mp *MwebPeer) SubscribeRecvMsg() (<-chan wire.Message, func()) {
	msgChan := make(chan wire.Message)
	quitChan := make(chan struct{})

	sub := msgSubscription{
		msgChan:  msgChan,
		quitChan: quitChan,
	}

	mp.mtxSubscribers.Lock()
	mp.recvSubscribers[sub] = struct{}{}
	mp.mtxSubscribers.Unlock()

	return msgChan, func() {
		close(quitChan)
	}
}

// OnDisconnect returns a channel that will be closed when this peer is
// disconnected.
//
// NOTE: Part of the query.Peer interface.
func (mp *MwebPeer) OnDisconnect() <-chan struct{} {
	return mp.quit
}

// OnRead is the callback for peer.MessageListeners.OnRead. It broadcasts
// received messages to all subscribers.
func (mp *MwebPeer) OnRead(_ *peer.Peer, _ int, msg wire.Message, _ error) {
	mp.mtxSubscribers.RLock()
	defer mp.mtxSubscribers.RUnlock()

	for sub := range mp.recvSubscribers {
		// Check if subscription has been canceled.
		select {
		case <-sub.quitChan:
			delete(mp.recvSubscribers, sub)
			continue
		default:
		}

		// Send in a goroutine to avoid blocking the read loop.
		go func(sub msgSubscription) {
			select {
			case <-sub.quitChan:
			case sub.msgChan <- msg:
			}
		}(sub)
	}
}
