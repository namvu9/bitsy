package swarm

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

const MIN_PEERS = 10

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	btorrent.Torrent

	EventCh chan interface{}
	OutCh   chan interface{}

	peers        []*peer.Peer
	trackerTiers []*tracker.TrackerGroup
	dialCfg      peer.DialConfig
}

func New(t btorrent.Torrent, out chan interface{}, trackers []*tracker.TrackerGroup, cfg peer.DialConfig) Swarm {
	swarm := Swarm{
		Torrent:      t,
		EventCh:      make(chan interface{}, 32),
		OutCh:        out,
		trackerTiers: trackers,
		dialCfg:      cfg,
	}

	return swarm
}

func (s *Swarm) dialPeers(ctx context.Context, n int) {
	// TODO: Also handle fallback tiers if they exist
	stat := s.trackerTiers[0].Stat()
	for _, ts := range stat {
		if len(ts.Peers) == 0 {
			continue
		}

		shuffle(ts.Peers)

		count := 0
		for _, pInfo := range ts.Peers {
			if count == n {
				break
			}

			select {
			case <-ctx.Done():
				return
			default:
			}

			// TODO: Don't dial peers we've already connected to
			if s.hasPeer(pInfo) {
				continue
			}

			ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			p, err := peer.Dial(ctx, pInfo, s.dialCfg)
			if err != nil {
				cancel()
				continue
			}

			count++
			go func() {
				s.EventCh <- JoinEvent{p}
			}()
			cancel()
		}
	}
}

func (s *Swarm) Init(ctx context.Context) {
	var (
		ticker = time.NewTicker(5 * time.Second)
		group  = s.trackerTiers[0]
	)

	group.Announce(tracker.NewRequest(s.dialCfg.InfoHash, 6881, s.dialCfg.PeerID))

	for {
		select {
		case <-ticker.C:
			// Get more peers
			// TODO: Keep track of which peers have most recently
			// been dialed
			if len(s.peers) < MIN_PEERS {
				go s.dialPeers(ctx, 10)
			}

		case event := <-s.EventCh:
			propagate, err := s.handleEvent(event)
			if err != nil {
				continue
			}

			if propagate {
				go s.publish(event)
			}
		}
	}
}

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

func (s *Swarm) publish(e interface{}) {
	s.OutCh <- e
}

func (s *Swarm) addPeer(p *peer.Peer) (bool, error) {
	s.peers = append(s.peers, p)

	return true, nil
}

func (s *Swarm) removePeer(p *peer.Peer) (bool, error) {
	for i, peer := range s.peers {
		if p == peer {
			s.peers[i] = s.peers[len(s.peers)-1]
			s.peers = s.peers[:len(s.peers)-1]
			return true, nil
		}
	}

	return false, nil
}

func shuffle(peers []net.Addr) {
	// Shuffle peers
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})
}

func (s *Swarm) hasPeer(addr net.Addr) bool {
	for _, p := range s.peers {
		if addr.String() == p.RemoteAddr().String() {
			return true
		}
	}

	return false
}

type SwarmStat struct {
	Peers       int
	Choked      int
	Blocking    int
	Interested  int
	Interesting int
}

func (s SwarmStat) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Peers: %d\n", s.Peers)
	fmt.Fprintf(&sb, "Choked: %d\n", s.Choked)
	fmt.Fprintf(&sb, "Choking: %d\n", s.Blocking)
	fmt.Fprintf(&sb, "Interested: %d\n", s.Interested)
	fmt.Fprintf(&sb, "Interesting: %d\n", s.Interesting)

	return sb.String()
}

func (s *Swarm) Stat() SwarmStat {
	cpy := make([]*peer.Peer, len(s.peers))
	copy(cpy, s.peers)

	var (
		choked      int
		blocking    int
		interested  int
		interesting int
	)

	for _, p := range cpy {
		if p.Blocking {
			blocking++
		}

		if p.Choked {
			choked++
		}

		if p.Interested {
			interested++
		}

		if p.Interesting {
			interesting++
		}

	}

	return SwarmStat{
		Peers:       len(cpy),
		Choked:      choked,
		Blocking:    blocking,
		Interested:  interested,
		Interesting: interesting,
	}
}
