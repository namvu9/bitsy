package swarm

import (
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type Event interface{}

type JoinEvent struct {
	*peer.Peer
}

type LeaveEvent struct {
	*peer.Peer
}

type InterestedEvent struct {
	*peer.Peer
	peer.Message
}

type InterestingEvent struct {
	*peer.Peer
	peer.Message
}

// BlockedEvent represents the peer choking the client
type BlockedEvent struct {
	By *peer.Peer
	peer.Message
}

// ChokeEvent represents the client choking the given peer
type ChokeEvent struct {
	*peer.Peer
	peer.Message
}

type UnchokeEvent struct {
	*peer.Peer
	peer.Message
}

type DataReceivedEvent struct {
	peer.PieceMessage
	Sender *peer.Peer
}

type DataSentEvent struct {
	peer.PieceMessage
	Receiver *peer.Peer
}

type DataRequestEvent struct {
	peer.RequestMessage
	Sender *peer.Peer
}

type BitFieldEvent struct {
	peer.BitFieldMessage
	Sender *peer.Peer
}

type DownloadCompleteEvent struct {
	time.Duration
	Index uint32
	Data  []byte
}

type MulticastMessage struct {
	// Specify a subset of peers to broadcast to
	Filter func(*peer.Peer) bool

	OrderBy func(*peer.Peer, *peer.Peer) int

	// The desired number of peers to broadcast the message
	// to.  The limit assumes the filter, if present, has
	// already been applied.
	Limit int

	Handler func([]*peer.Peer)
}

// the first response value is an indicator of whether the
// swarm should propagate the event to subscribers
func (s *Swarm) handleEvent(e Event) (bool, error) {
	switch v := e.(type) {
	case JoinEvent:
		return s.addPeer(v.Peer)
	case LeaveEvent:
		return s.removePeer(v.Peer)
	case MulticastMessage:
		return s.handleBroadcastRequest(v)
	}

	return true, nil
}

func (s *Swarm) handleBroadcastRequest(req MulticastMessage) (bool, error) {
	go func() {
		count := 0
		for i, peer := range s.peers {
			if req.Limit > 0 && i == req.Limit {
				break
			}

			if req.Filter != nil && !req.Filter(peer) {
				break
			}

			count++
			//go peer.Send(req.Message)
		}
	}()

	return false, nil
}
