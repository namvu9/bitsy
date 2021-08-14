package swarm

import (
	"math/rand"
	"sort"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

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
func (s *Swarm) handleEvent(e interface{}) (bool, error) {
	switch v := e.(type) {
	case JoinEvent:
		v.Peer.OnClose(func(p *peer.Peer) {
			s.EventCh <- LeaveEvent{Peer: p}
		})
		return s.addPeer(v.Peer)
	case LeaveEvent:
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

		var cpy []*peer.Peer
		for _, p := range s.peers {
			cpy = append(cpy, p)
		}

		if req.OrderBy != nil {
			sort.SliceStable(cpy, func(i, j int) bool {
				return req.OrderBy(cpy[i], cpy[j]) < 0
			})

		} else {
			rand.Seed(time.Now().UnixNano())
			rand.Shuffle(len(cpy), func(i, j int) {
				cpy[i], cpy[j] = cpy[j], cpy[i]
			})
		}

		for _, peer := range cpy {
			n := rand.Int31n(100)
			if n < 25 {
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
