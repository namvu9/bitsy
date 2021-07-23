package swarm

import (
	"net"
	"time"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
	"github.com/rs/zerolog/log"
)

// Peer represents a connection to another peer in the
// swarm
type Peer struct {
	out     chan Event
	eventCh chan Event

	net.Conn

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
	BitField bits.BitField

	LastMessageReceived time.Time
	LastMessageSent     time.Time

	requests []btorrent.RequestMessage

	closed bool
}

func (p *Peer) Close() error {
	p.closed = true
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

func (p *Peer) Send(msg btorrent.Message) error {
	if p.closed {
		return nil
	}

	if req, ok := msg.(btorrent.RequestMessage); ok {
		if p.Blocking || !p.has(int(req.Index)) || p.isServing(req.Index, req.Offset) {
			return nil
		}

	}

	_, err := p.Write(msg.Bytes())
	if err != nil {
		return err
	}

	switch msg.(type) {
	case btorrent.UnchokeMessage:
		p.Choked = false
	case btorrent.ChokeMessage:
		p.Choked = true
	case btorrent.InterestedMessage:
		p.Interesting = true
	case btorrent.NotInterestedMessage:
		p.Interesting = false
	}

	go p.publish(MessageSent{
		Message: msg,
		Peer:    p,
	})

	return nil
}

func (p *Peer) Idle() bool {
	return time.Now().Sub(p.LastMessageReceived) > 2*time.Minute
}

func (p *Peer) Subscribe(out chan Event) chan Event {
	p.out = out

	return p.eventCh
}

func (p *Peer) Write(data []byte) (int, error) {
	var op errors.Op = "(*Peer).Write"

	p.LastMessageSent = time.Now()
	p.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	n, err := p.Conn.Write(data)
	if err != nil {
		return n, errors.Wrap(err, op, errors.Network)
	}

	return n, nil
}

func (peer *Peer) publish(e Event) {
	peer.out <- e
}

func (peer *Peer) has(index int) bool {
	return peer.BitField.Get(index)
}

// Listen for incoming messages
func (peer *Peer) Listen() {
	for {
		msg, err := btorrent.UnmarshallMessage(peer.Conn)
		peer.LastMessageReceived = time.Now()

		if err != nil {
			log.Err(err).Msgf("failed to unmarshal message from %s", peer.RemoteAddr())
			return
		}

		switch v := msg.(type) {
		case btorrent.HaveMessage:
			peer.BitField.Set(int(v.Index))
		case btorrent.PieceMessage:
			peer.Uploaded += int64(len(v.Piece))
		case btorrent.ChokeMessage:
			peer.Blocking = true
		case btorrent.UnchokeMessage:
			peer.Blocking = false
		case btorrent.InterestedMessage:
			peer.Interested = true
		case btorrent.NotInterestedMessage:
			peer.Interested = false
		case btorrent.BitFieldMessage:
			peer.BitField = v.BitField
		}

		go peer.publish(MessageReceived{
			Peer:    peer,
			Message: msg,
		})
	}
}

func NewPeer(conn net.Conn) *Peer {
	peer := &Peer{
		Conn:                conn,
		Blocking:            true,
		Choked:              true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
	}
	go peer.Listen()
	return peer
}
