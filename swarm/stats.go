package swarm

import (
	"time"

	"github.com/namvu9/bitsy/tracker"
)

// Stats contains statistics relevant to the swarm
type Stats struct {
	eventCh chan Event
	outCh   chan Event

	downloaded        uint64
	leechers          int
	lastPieceReceived time.Time
	peerInfo          []tracker.PeerInfo
	seeders           int
	uploaded          uint64
	peers             int
}

type StatEvent struct {
	name    string
	payload map[string]interface{}
}

func (s *Stats) Subscribe(chan Event) chan Event {
	s.eventCh = make(chan Event)
	s.outCh = make(chan Event)

	go func() {
		for {
			select {
			case s.outCh <- StatEvent{
				name: "Stat",
				payload: map[string]interface{}{
					"peers": s.peers,
					"leechers": s.leechers,
					"seeders": s.seeders,
				},
			}:
			case event := <-s.eventCh:
				switch v := event.(type) {
				case JoinEvent:
					s.peers++
				case LeaveEvent:
					s.peers--
				case TrackerAnnounceEvent:
					s.leechers = v.leechers
					s.seeders = v.seeders
				}

			}
		}

	}()

	return s.eventCh
}

func (s *Stats) handleDataSentEvent(e DataSentEvent) (bool, error) {
	s.uploaded += uint64(len(e.Piece))
	return true, nil
}

func (s *Stats) handleDataReceivedEvent(e DataReceivedEvent) (bool, error) {
	s.downloaded += uint64(len(e.Piece))
	return true, nil
}
