package swarm

import (
	"net"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	btorrent.Torrent

	eventCh    chan Event
	outCh      chan Event
	AnnounceCh chan []tracker.PeerInfo
	PeerCh     chan net.Conn

	peers []*peer.Peer
	stats *Stats

	downloadDir string
}

func New(t btorrent.Torrent, out chan Event) Swarm {
	swarm := Swarm{
		Torrent:    t,
		PeerCh:     make(chan net.Conn, 32),
		eventCh:    make(chan Event, 32),
		AnnounceCh: make(chan []tracker.PeerInfo, 32),
		outCh:      out,
	}

	return swarm
}

func (s *Swarm) Stat() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["torrent"] = s.HexHash()
	stats["npeers"] = len(s.peers)

	var (
		interested  = 0
		interesting = 0
		choking     = 0
		blocking    = 0
		idle        = 0
	)

	for _, peer := range s.peers {
		if peer.Interested {
			interested++
		}

		if peer.Interesting {
			interesting++
		}

		if peer.Choked {
			choking++
		}

		if peer.Blocking {
			blocking++
		}

		if time.Now().Sub(peer.LastMessageReceived) > 2*time.Minute {
			idle++
		}
	}

	stats["interested"] = interested
	stats["interesting"] = interesting
	stats["choking"] = choking
	stats["blocking"] = blocking
	stats["idle"] = idle

	select {
	case swarmStats := <-s.stats.outCh:
		stats["stats"] = swarmStats.(StatEvent).payload
	default:
	}

	return stats
}

func (s *Swarm) Init() {
	for {
		select {
		case conn := <-s.PeerCh:
			event := JoinEvent{peer.New(conn)}
			go func() {
				s.eventCh <- event
				s.publish(event)
			}()
		case event := <-s.eventCh:
			propagate, err := s.handleEvent(event)
			if err != nil {
				log.Err(err).Str("swarm", s.HexHash()).Msg("Handle event failed")
				continue
			}

			if propagate {
				s.publish(event)
			}
		}
	}
}

func (s *Swarm) listen() {}

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

func (s *Swarm) publish(e Event) {}

func (s *Swarm) addPeer(peer *peer.Peer) (bool, error) {
	s.peers = append(s.peers, peer)

	return true, nil
}

func (s *Swarm) removePeer(peer *peer.Peer) (bool, error) {
	for i, p := range s.peers {
		if p == peer {
			s.peers[i] = s.peers[len(s.peers)-1]
			s.peers = s.peers[:len(s.peers)-1]
		}
	}

	return true, nil
}

func (s *Swarm) getPeer(addr net.Addr) (*peer.Peer, bool) {
	for _, peer := range s.peers {
		if peer.RemoteAddr().String() == addr.String() {
			return peer, true
		}
	}

	return nil, false
}
