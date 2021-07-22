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

	// Pending request messages
	// TODO: Remove these once fulfilled
	requests []btorrent.RequestMessage
}

func (p *Peer) Send(msg btorrent.Message) error {
	_, err := p.Write(msg.Bytes())
	if err != nil {
		return err
	}

	return nil
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

// Listen for incoming messages
func (peer *Peer) Listen() {
	for {
		msg, err := btorrent.UnmarshallMessage(peer.Conn)
		if err != nil {
			log.Err(err).Msgf("failed to unmarshall message from %s", peer.RemoteAddr())
			return
		}

		switch v := msg.(type) {
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
			Peer:    *peer,
			Message: msg,
		})
	}
}

//func (p *Peer) handleHaveMessage(msg HaveMessage) {
	//if !s.have.Get(int(msg.Index)) && !p.Blocking {
		//p.ShowInterest()
	//}
//}

func NewPeer(conn net.Conn) Peer {
	peer := Peer{
		Conn:                conn,
		Blocking:            true,
		Choked:              true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
	}
	go peer.Listen()
	return peer
}
