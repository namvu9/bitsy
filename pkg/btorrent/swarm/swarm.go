package swarm

import (
	"fmt"
	"net"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	btorrent.Torrent

	EventCh    chan Event
	OutCh      chan Event
	PeerCh     chan *peer.Peer
	MoarPeerCh chan struct{}

	peers []*peer.Peer
}

func (s *Swarm) Size() int {
	return len(s.peers)
}

func New(t btorrent.Torrent, out chan Event) Swarm {
	swarm := Swarm{
		Torrent: t,
		PeerCh:  make(chan *peer.Peer, 32),
		EventCh: make(chan Event, 32),
		OutCh:   out,
		MoarPeerCh: make(chan struct{}, 32),
	}

	return swarm
}

func (s *Swarm) Init() {
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case p := <-s.PeerCh:
			event := JoinEvent{p}
			go func() {
				s.EventCh <- event
			}()
		case event := <-s.EventCh:
			propagate, err := s.handleEvent(event)
			if err != nil {
				log.Err(err).Str("swarm", s.HexHash()).Msg("Handle event failed")
				continue
			}

			if propagate {
				go s.publish(event)
			}
		case <-ticker.C:
			fmt.Println("SWARM TICK", len(s.peers))
			if len(s.peers) < 10 {
				go func() {
					s.MoarPeerCh <- NeedMOARPeers{}
				}()
			}
		}
	}
}

type NeedMOARPeers struct{}

func (s *Swarm) choked() (choked []*peer.Peer, unchoked []*peer.Peer) {
	for _, peer := range s.peers {
		if peer.Choked {
			choked = append(choked, peer)
		} else {
			unchoked = append(unchoked, peer)
		}
	}

	return
}

func (s *Swarm) publish(e Event) {
	s.OutCh <- e
}

func (s *Swarm) addPeer(peer *peer.Peer) (bool, error) {
	s.peers = append(s.peers, peer)

	return true, nil
}

func (s *Swarm) removePeer(peer *peer.Peer) (bool, error) {
	for i, p := range s.peers {
		if p == peer {
			s.peers[i] = s.peers[len(s.peers)-1]
			s.peers = s.peers[:len(s.peers)-1]
			return true, nil
		}
	}

	return false, fmt.Errorf("peer %p not found", peer)
}

func (s *Swarm) getPeer(addr net.Addr) (*peer.Peer, bool) {
	for _, peer := range s.peers {
		if peer.RemoteAddr().String() == addr.String() {
			return peer, true
		}
	}

	return nil, false
}
