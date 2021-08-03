package peer

import (
	"context"
	"net"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/rs/zerolog/log"
)

// Peer represents a connection to another peer in the
// swarm
type Peer struct {
	net.Conn
	closed bool

	Msg chan Message
	//Out chan Event

	// This peer has choked the client and should not be
	// be asked for pieces
	Blocking bool

	// This peer has been choked by the client and will not be
	// sent any pieces
	Choked bool

	// This peer wants one or more of the pieces that the
	// client has
	Interested bool

	// This peer has one or more of the pieces that client
	// wants
	Interesting bool

	// Stats
	// Number of bytes uploaded in the last n-second window
	UploadRate   int64
	Uploaded     int64
	DownloadRate int64
	Downloaded   int64

	// A bitfield specifying the indexes of the pieces that
	// the peer has
	Pieces     bits.BitField

	// Extensions enabled by the peer's client
	Extensions btorrent.Extensions

	LastMessageReceived time.Time
	LastMessageSent     time.Time

	requests []RequestMessage
}

func (p *Peer) Close() error {
	return p.Conn.Close()
}

func (p *Peer) isServing(index uint32, offset uint32) bool {
	for _, req := range p.requests {
		if req.Index == index && req.Offset == offset {
			return true
		}
	}

	return false
}

func (p *Peer) Send(msg Message) error {
	if p.closed {
		return nil
	}

	go func() {
		_, err := p.write(msg.Bytes())
		if err != nil {
		}
	}()

	switch msg.(type) {
	case UnchokeMessage:
		p.Choked = false
	case ChokeMessage:
		p.Choked = true
	case InterestedMessage:
		p.Interesting = true
	case NotInterestedMessage:
		p.Interesting = false
	}

	return nil
}

func (p *Peer) Idle() bool {
	return time.Now().Sub(p.LastMessageReceived) > 15*time.Second
}

func (p *Peer) write(data []byte) (int, error) {
	p.LastMessageSent = time.Now()
	p.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	n, err := p.Conn.Write(data)
	if err != nil {
		return n, errors.Wrap(err, errors.Network)
	}

	return n, nil
}

//func (peer *Peer) publish(e Event) {
	//peer.Out <- e
//}

func (peer *Peer) HasPiece(index int) bool {
	return peer.Pieces.Get(index)
}

// Listen for incoming messages
func (p *Peer) Listen(ctx context.Context) {
	for {
		msg, err := UnmarshallMessage(p.Conn)
		p.LastMessageReceived = time.Now()

		if err != nil && errors.IsEOF(err) {
			p.Close()
			//peer.publish(LeaveEvent{peer})
			return
		}

		if err != nil {
			log.Err(err).Msgf("failed to unmarshal message from %s", p.RemoteAddr())
			return
		}

		switch v := msg.(type) {
		case HaveMessage:
			p.Pieces.Set(int(v.Index))
		case PieceMessage:
			p.Uploaded += int64(len(v.Piece))
		case ChokeMessage:
			p.Blocking = true
		case UnchokeMessage:
			p.Blocking = false
		case InterestedMessage:
			p.Interested = true
		case NotInterestedMessage:
			p.Interested = false
		case BitFieldMessage:
			p.Pieces = v.BitField
		}
	}
}

func New(c net.Conn) *Peer {
	peer := &Peer{
		Conn:                c,
		Blocking:            true,
		Choked:              true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
	}
	return peer
}
