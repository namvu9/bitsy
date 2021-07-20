package messaging

import (
	"fmt"
	"io/ioutil"
	"path"

	"github.com/namvu9/bitsy/src/bits"
	"github.com/namvu9/bitsy/src/errors"
	"github.com/rs/zerolog/log"
)

func (s *Swarm) handleMessage(msg Message, p *Peer) {
	switch v := msg.(type) {
	case BitFieldMessage:
		s.handleBitfieldMessage(p, v)
	case ChokeMessage:
		s.handleChokeMessage(p)
	case UnchokeMessage:
		s.handleUnchokeMessage(p)
	case InterestedMessage:
		s.handleInterestedMessage(p)
	case NotInterestedMessage:
		s.handleNotInterestedMessage(p)
	case HaveMessage:
		s.handleHaveMessage(p, v)
	case RequestMessage:
		s.handleRequestMessage(p, v)
	case PieceMessage:
		s.handlePieceMessage(p, v)
	case CancelMessage:
		s.handleCancelMessage(p, v)
	case ExtendedMessage:
		s.handleExtendedMessage(p, v)
	default:
	}
}
func (s *Swarm) handleBitfieldMessage(peer *Peer, msg BitFieldMessage) error {
	var op errors.Op = "(*Swarm).handleBitfieldMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(BitField)).
		Msgf("Received BitField message from %s", peer.RemoteAddr())

	var (
		maxIndex = bits.GetMaxIndex(msg.BitField)
		pieces   = s.Pieces()
	)

	if maxIndex >= len(pieces) {
		err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		return errors.Wrap(err, op, errors.BadArgument)
	}

	peer.BitField = msg.BitField

	for i := range pieces {
		if !s.have.Get(i) && peer.BitField.Get(i) {
			peer.ShowInterest()
			return nil
		}
	}

	return nil
}

func (s *Swarm) handleChokeMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Choke)).
		Msgf("Received Choke message from %s", p.RemoteAddr())

	p.Blocking = true
	p.requests = []RequestMessage{}
	return nil
}

func (s *Swarm) handleUnchokeMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Unchoke)).
		Msgf("Received Unchoke message from %s", p.RemoteAddr())

	p.NotBlocking()
	return nil
}

func (s *Swarm) handleInterestedMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(NotInterested)).
		Msgf("Received NotInterested message from %s", p.RemoteAddr())

	p.Interested = true
	return nil
}

func (s *Swarm) handleNotInterestedMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(NotInterested)).
		Msgf("Received NotInterested message from %s", p.RemoteAddr())

	p.Interested = false
	return nil
}

// TODO: Parse message before sending to handler
func (s *Swarm) handleHaveMessage(p *Peer, msg HaveMessage) error {
	//var op errors.Op = "(*Swarm).handleHaveMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Have)).
		Msgf("Received Have message from %s", p.RemoteAddr())

	if !s.have.Get(int(msg.Index)) && !p.Blocking {
		p.ShowInterest()
	}

	return nil
}

func (s *Swarm) handleRequestMessage(p *Peer, req RequestMessage) error {
	var op errors.Op = "(*Swarm).handleRequestMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Request)).
		Msgf("Received request message from %s", p.RemoteAddr())

	filePath := path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", req.Index))
	fmt.Printf("Request: Index: %d, offset: %d, length: %d (path: %s)\n", req.Index, req.Offset, req.Length, filePath)

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	data = data[req.Offset : req.Offset+req.Length]

	msg := PieceMessage{
		Index:  req.Index,
		Offset: req.Offset,
		Piece:  data,
	}

	go p.ServePiece(msg)
	s.uploaded += uint64(len(data))

	return nil
}

// TODO: Break up into smaller functions
func (s *Swarm) handlePieceMessage(p *Peer, msg PieceMessage) error {
	//var op errors.Op = "(*Swarm).handlePieceMessage"

	go func() {
		s.downloader.responseCh <- msg
	}()

	return nil
}

// TODO: Implement
func (s *Swarm) handleCancelMessage(p *Peer, msg CancelMessage) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Cancel)).
		Msgf("Received cancel message from %s", p.RemoteAddr())

	return nil
}

func (s *Swarm) handleExtendedMessage(p *Peer, msg ExtendedMessage) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Extended)).
		Msgf("Received extended message from %s", p.RemoteAddr())

	return nil
}
