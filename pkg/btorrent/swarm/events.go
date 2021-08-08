package swarm

import (
	"fmt"
	"math/rand"

	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type Event interface{}

type JoinEvent struct {
	*peer.Peer
}

type LeaveEvent struct {
	*peer.Peer
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
		fmt.Println("PEERS", len(s.peers)+1)
		v.Peer.OnClose(func(p *peer.Peer) {
			s.EventCh <- LeaveEvent{Peer:p}
		 })
		return s.addPeer(v.Peer)
	case LeaveEvent:
		fmt.Println("PEER LEFT", len(s.peers)-1)
		return s.removePeer(v.Peer)
	case MulticastMessage:
		return s.handleMulticastMessage(v)
	}

	return true, nil
}

func (s *Swarm) handleMulticastMessage(req MulticastMessage) (bool, error) {
	go func() {
		var res []*peer.Peer
		count := 0
		for _, peer := range s.peers {
			n := rand.Int31n(100)
			if n < 50 {
				continue
			}
			if req.Limit > 0 && count == req.Limit {
				break
			}

			if req.Filter != nil && !req.Filter(peer) {
				continue
			}

			res = append(res, peer)
			count++
		}

		req.Handler(res)
	}()

	return false, nil
}
