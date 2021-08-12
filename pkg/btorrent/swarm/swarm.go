package swarm

import (
	"context"
	"math/rand"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	btorrent.Torrent

	EventCh chan Event
	OutCh   chan Event
	PeerCh  chan *peer.Peer

	peers    []*peer.Peer
	trackers []*tracker.TrackerGroup
	dialCfg  peer.DialConfig
}

func (s *Swarm) Size() int {
	return len(s.peers)
}

type SwarmStat struct {
	Peers int
}

func (s *Swarm) Stat() SwarmStat {
	return SwarmStat{
		Peers: len(s.peers),
	}
}

func New(t btorrent.Torrent, out chan Event, trackers []*tracker.TrackerGroup, cfg peer.DialConfig) Swarm {
	swarm := Swarm{
		Torrent:  t,
		PeerCh:   make(chan *peer.Peer, 32),
		EventCh:  make(chan Event, 32),
		OutCh:    out,
		trackers: trackers,
		dialCfg:  cfg,
	}

	return swarm
}

func (s *Swarm) Init() {
	ticker := time.NewTicker(5 * time.Second)
	group := s.trackers[0]

	group.Announce(tracker.NewRequest(s.dialCfg.InfoHash, 6881, s.dialCfg.PeerID))

	for {
		select {
		case p := <-s.PeerCh:
			go func() {
				s.EventCh <- JoinEvent{p}
			}()
		case <-ticker.C:
			if len(s.peers) < 10 {
				go func() {
					stat := s.trackers[0].Stat()
					count := len(s.peers)
					for _, ts := range stat {
						if len(ts.Peers) == 0 {
							continue
						}
						rand.Seed(time.Now().UnixNano())
						rand.Shuffle(len(ts.Peers), func(i, j int) {
							ts.Peers[i], ts.Peers[j] = ts.Peers[j], ts.Peers[i]
						})

						for _, pInfo := range ts.Peers {
							if count == 10 {
								break
							}

							p, err := peer.Dial(context.Background(), pInfo, s.dialCfg)
							if err != nil {
								continue
							}

							count++
							s.EventCh <- JoinEvent{p}
						}
					}

				}()
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

func (s *Swarm) publish(e Event) {
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
