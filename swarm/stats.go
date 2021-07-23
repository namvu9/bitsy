package swarm

import (
	"sort"
	"time"

	"github.com/namvu9/bitsy/pkg/bits"
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
				switch v := event.(type) {
				case DownloadCompleteEvent:
					s.handleDataReceivedEvent(v)
				case MessageReceived:
					s.handleMessageReceived(v)
				case MessageSent:
					s.handleMessageSent(v)
				case TrackerAnnounceEvent:
					s.leechers = v.leechers
					s.seeders = v.seeders
				}
			}
		}

	}()

	return s.eventCh
}

// TODO: Figure out rarest piece
func (s *Stats) handleMessageReceived(e MessageReceived) (bool, error) {
	switch v := e.Message.(type) {
	case btorrent.KeepAliveMessage:
		s.messageSentCount["keepAlive"]++
	case btorrent.ChokeMessage:
		s.messageReceivedCount["choke"]++
	case btorrent.UnchokeMessage:
		s.messageReceivedCount["unchoke"]++
	case btorrent.InterestedMessage:
		s.messageReceivedCount["interested"]++
	case btorrent.NotInterestedMessage:
		s.messageReceivedCount["notInterested"]++
	case btorrent.HaveMessage:
		s.messageReceivedCount["have"]++
		s.pieceFrequency[int(v.Index)]++
	case btorrent.BitFieldMessage:
		bitfield := bits.BitField(v.BitField)
		for i := range s.Pieces() {
			if bitfield.Get(i) {
				s.pieceFrequency[i]++
			}
		}
		s.messageReceivedCount["bitfield"]++
	case btorrent.RequestMessage:
		s.messageReceivedCount["request"]++
	case btorrent.PieceMessage:
		s.messageReceivedCount["piece"]++
		s.downloaded += uint64(len(v.Piece))

		if diff := time.Now().Sub(s.lastDownloadChunk); diff > 5*time.Second {
			s.downloadRate = (s.downloaded - s.downloadedChunk) / uint64(diff.Seconds())
			s.downloadedChunk = s.downloaded
			s.lastDownloadChunk = time.Now()
		}

	case btorrent.CancelMessage:
		s.messageReceivedCount["cancel"]++
	case btorrent.ExtendedMessage:
		s.messageReceivedCount["extended"]++
	default:
		s.messageReceivedCount["unknown"]++
	}

	return true, nil
}

// TODO: Figure out rarest piece
func (s *Stats) handleMessageSent(e MessageSent) (bool, error) {
	switch v := e.Message.(type) {
	case btorrent.KeepAliveMessage:
		s.messageSentCount["keepAlive"]++
	case btorrent.ChokeMessage:
		s.messageSentCount["choke"]++
	case btorrent.UnchokeMessage:
		s.messageSentCount["unchoke"]++
	case btorrent.InterestedMessage:
		s.messageSentCount["interested"]++
	case btorrent.NotInterestedMessage:
		s.messageSentCount["notInterested"]++
	case btorrent.HaveMessage:
		s.messageSentCount["have"]++
		s.pieceFrequency[int(v.Index)]++
	case btorrent.BitFieldMessage:
		bitfield := bits.BitField(v.BitField)
		for i := range s.Pieces() {
			if bitfield.Get(i) {
				s.pieceFrequency[i]++
			}
		}
		s.messageSentCount["bitfield"]++
	case btorrent.RequestMessage:
		s.messageSentCount["request"]++
	case btorrent.PieceMessage:
		s.messageSentCount["piece"]++
		s.uploaded += uint64(len(v.Piece))
	case btorrent.CancelMessage:
		s.messageSentCount["cancel"]++
	case btorrent.ExtendedMessage:
		s.messageSentCount["extended"]++
	default:
		s.messageSentCount["unknown"]++
	}

	return true, nil
}

func (s *Stats) handleDataSentEvent(e DataSentEvent) (bool, error) {
	s.uploaded += uint64(len(e.Piece))
	return true, nil
}

func (s *Stats) handleDataReceivedEvent(e DownloadCompleteEvent) (bool, error) {
	s.downloaded += uint64(len(e.Data))
	return true, nil
}

func NewStats(torrent btorrent.Torrent) *Stats {
	pieceFreq := make(map[int]int)

	for i := range torrent.Pieces() {
		pieceFreq[i] = 0
	}

	return &Stats{
		Torrent:              torrent,
		eventCh:              make(chan Event, 32),
		outCh:                make(chan Event, 32),
		messageReceivedCount: make(map[string]int),
		messageSentCount:     make(map[string]int),
		pieceFrequency:       pieceFreq,
		lastDownloadChunk:    time.Now(),
	}
}
