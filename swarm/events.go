package swarm

import (
	"fmt"
	"io/ioutil"
	"net"
	"path"
	"time"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
	"github.com/rs/zerolog/log"
)

type Subscriber interface {
	Subscribe(out chan Event) (in chan Event)
}

type Event interface{}

type JoinEvent struct {
	Peer
}

type LeaveEvent struct {
	Peer
}

type MessageReceived struct {
	btorrent.Message
	Peer
}

type InterestedEvent struct {
	Peer
	btorrent.Message
}

type InterestingEvent struct {
	Peer
	btorrent.Message
}

// BlockedEvent represents the peer choking the client
type BlockedEvent struct {
	By Peer
	btorrent.Message
}

// ChokeEvent represents the client choking the given peer
type ChokeEvent struct {
	Peer
	btorrent.Message
}

type UnchokeEvent struct {
	Peer
	btorrent.Message
}

type DataReceivedEvent struct {
	btorrent.PieceMessage
	Sender Peer
}

type DataSentEvent struct {
	btorrent.PieceMessage
	Receiver Peer
}

type DataRequestEvent struct {
	btorrent.RequestMessage
	Sender Peer
}

type BitFieldEvent struct {
	btorrent.BitFieldMessage
	Sender Peer
}

type DownloadCompleteEvent struct {
	time.Duration
	Index uint32
	Data  []byte
}

type TrackerAnnounceEvent struct {
	name      string
	seeders   int
	leechers  int
	peers     []map[string]string
	timestamp time.Time
}

type BroadcastRequest struct {
	btorrent.Message

	// Specify a subset of peers to broadcast to
	Filter func(*Peer) bool

	OrderBy func(*Peer, *Peer) int

	// The desired number of peers to broadcast the message
	// to.  The limit assumes the filter, if present, has
	// already been applied.
	Limit int
}

// the first response value is an indicator of whether the
// swarm should propagate the event to subscribers
func (s *Swarm) handleEvent(e Event) (bool, error) {
	switch v := e.(type) {
	case JoinEvent:
		return s.addPeer(&v.Peer)
	case LeaveEvent:
		return s.removePeer(&v.Peer)
	case InterestingEvent:
		return s.handleInterestingEvent(v)
	case ChokeEvent:
		return s.handleChokeEvent(v)
	case UnchokeEvent:
		return s.handleUnchokeEvent(v)
	case BitFieldEvent:
		return s.handleBitfieldMessage(v)
	case DownloadCompleteEvent:
		return s.handleDownloadCompleteEvent(v)
	case DataRequestEvent:
		return s.handleDataRequestEvent(v)
	case BroadcastRequest:
		return s.handleBroadcastRequest(v)
	}

	return true, nil
}

func (s *Swarm) handleDataRequestEvent(e DataRequestEvent) (bool, error) {
	var (
		req      = e.RequestMessage
		peer     = e.Sender
		filePath = path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", req.Index))
	)

	log.Printf("Request: Index: %d, offset: %d, length: %d (path: %s)\n", req.Index, req.Offset, req.Length, filePath)

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	data = data[req.Offset : req.Offset+req.Length]

	msg := btorrent.PieceMessage{
		Index:  req.Index,
		Offset: req.Offset,
		Piece:  data,
	}

	peer.Send(msg)

	return true, nil
}

func (s *Swarm) handleDownloadCompleteEvent(e DownloadCompleteEvent) (bool, error) {
	s.have.Set(int(e.Index))
	go s.broadcastEvent(BroadcastRequest{
		Message: btorrent.HaveMessage{Index: e.Index},
	})
	return true, nil
}

func (s *Swarm) handleBitfieldMessage(e BitFieldEvent) (bool, error) {
	var op errors.Op = "(*Swarm).handleBitfieldMessage"
	msg := e.BitFieldMessage
	peer := e.Sender

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(btorrent.BitField)).
		Msgf("Received BitField message from %s", peer.RemoteAddr())

	var (
		maxIndex = bits.GetMaxIndex(msg.BitField)
		pieces   = s.Pieces()
	)

	if maxIndex >= len(pieces) {
		err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		return false, errors.Wrap(err, op, errors.BadArgument)
	}

	for i := range pieces {
		if !s.have.Get(i) && bits.BitField(e.BitField).Get(i) {
			go func() {
				s.broadcastEvent(InterestingEvent{
					Peer:    peer,
					Message: btorrent.InterestedMessage{},
				})
			}()

			return false, nil
		}
	}

	return false, nil
}

func (s *Swarm) getPeer(addr net.Addr) (*Peer, bool) {
	for _, peer := range s.peers {
		if peer.RemoteAddr().String() == addr.String() {
			return peer, true
		}
	}

	return nil, false
}

func (s *Swarm) handleUnchokeEvent(e UnchokeEvent) (bool, error) {
	peer, ok := s.getPeer(e.RemoteAddr())
	if !ok {
		return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	}

	if !peer.Choked {
		return true, nil
	}

	peer.Choked = false
	go peer.Send(btorrent.UnchokeMessage{})

	return true, nil

}

func (s *Swarm) handleChokeEvent(e ChokeEvent) (bool, error) {
	peer, ok := s.getPeer(e.Peer.RemoteAddr())
	if !ok {
		return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	}

	peer.Choked = true
	return true, nil
}

func (s *Swarm) handleInterestingEvent(e InterestingEvent) (bool, error) {
	peer, ok := s.getPeer(e.Peer.RemoteAddr())
	if !ok {
		return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	}

	peer.Interesting = true
	go peer.Send(InterestedEvent{})

	return false, nil
}

func (s *Swarm) handleBroadcastRequest(req BroadcastRequest) (bool, error) {
	log.Print("BROADCATING", req)
	go func() {
		for i, peer := range s.peers {
			if req.Limit > 0 && i == req.Limit {
				break
			}

			if req.Filter != nil && req.Filter(peer) {
				peer.Send(req.Message)
			}
		}
	}()

	return false, nil
}
