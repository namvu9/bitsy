package swarm

import (
	"sort"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

// Stats contains statistics relevant to the swarm
type Stats struct {
	btorrent.Torrent

	eventCh chan Event
	outCh   chan Event

	downloaded uint64
	uploaded   uint64
	leechers   int
	seeders    int
	peers      int

	messageReceivedCount map[string]int
	messageSentCount     map[string]int
	pieceFrequency       map[int]int

	// B / second
	downloadRate      uint64
	downloadedChunk   uint64
	lastDownloadChunk time.Time
}

type pieceFreq struct {
	index int
	freq  int
}

// PieceIdxByFreq returns a slice of piece indices ordered
// (ascending) by their frequency in the swarm.
func (s *Stats) PieceIdxByFreq() []pieceFreq {
	var out []pieceFreq

	for index, freq := range s.pieceFrequency {
		out = append(out, pieceFreq{
			index: index,
			freq:  freq,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].freq < out[j].freq
	})

	return out
}

type StatEvent struct {
	name    string
	payload map[string]interface{}
}

func (s *Stats) Subscribe(chan Event) chan Event {
	go func() {
		for {
			select {
			case s.outCh <- StatEvent{
				name: "Stat",
				payload: map[string]interface{}{
					"msgReceivedCount": s.messageReceivedCount,
					"msgSentCount":     s.messageSentCount,
					"pieceFreq":        s.pieceFrequency,
					"uploaded":         s.uploaded,
					"downloaded":       s.downloaded,
					"downloadRate":     s.downloadRate,
				},
			}:
			case event, ok := <-s.eventCh:
				if !ok {
					return
				}
				switch event.(type) {
				case DownloadCompleteEvent:
					//s.handleDataReceivedEvent(v)
				}
			}
		}

	}()

	return s.eventCh
}

