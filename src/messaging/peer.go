package messaging

import (
	"fmt"
	"net"
	"time"

	"github.com/namvu9/bitsy/src/bits"
	"github.com/namvu9/bitsy/src/errors"
	"github.com/rs/zerolog/log"
)

// Peer represents a connection to another peer in the
// swarm
type Peer struct {
	closed bool

	in  chan Message // in means send to peer, closed ch represents request to close conn
	out chan Message // out means message (out) from peer, closed ch represents disconnected peer

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
	requests []RequestMessage
}

func (p *Peer) Close() error {
	if p.closed {
		return nil
	}

	p.closed = true
	//close(p.out)
	return p.Conn.Close()
}

// TODO: TEST
// TODO: Improve this API
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

// Listen for incoming messages
func (peer *Peer) Listen() {
	peer.SetReadDeadline(time.Now().Add(30 * time.Second))
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("RECOVERED FROM", r)
		}

		err := peer.Close()
		if err != nil {
			log.Err(err).Msg("Failed to close peer")
		}
	}()

	go func() {
		msg, ok := <-peer.in
		if !ok {
			return
		}

		switch v := msg.(type) {
		case RequestMessage:
			fmt.Println("REQUESTING PIECE", v.Index, v.Offset)
		}

		_, err := peer.Write(msg.Bytes())
		if err != nil {
			//fmt.Println(err)
			return
		}
	}()

	for {
		msg, err := UnmarshallMessage(peer.Conn)
		if err != nil {
			log.Err(err).Msgf("failed to unmarshall message from %s", peer.RemoteAddr())

			return
		}

		switch v := msg.(type) {
		case PieceMessage:
			peer.Uploaded += int64(len(v.Piece))
		}

		go func() {
			peer.out <- msg
		}()
	}
}

func (p *Peer) Send(msg Message) {
	p.in <- msg
}

func (p *Peer) SendBitField(msg BitFieldMessage) {
	p.in <- msg
}

func (p *Peer) RequestPiece(msg RequestMessage) {
	p.ShowInterest()
	go func() {
		_, err := p.Write(msg.Bytes())
		if err != nil {
			//fmt.Println(err)
			return
		}
	}()
}

func (p *Peer) ServePiece(msg PieceMessage) {
	fmt.Println("Serving piece to", p.RemoteAddr())
	p.in <- msg
}

func (p *Peer) ShowInterest() {
	if p.Interesting {
		return
	}

	p.Interesting = true

	p.in <- InterestedMessage{}
}

func (p *Peer) NotInterested() {
	if !p.Interesting {
		return
	}

	p.Interesting = false
	p.in <- NotInterestedMessage{}
}

func (p *Peer) Choke() {
	if p.Choked {
		return
	}

	p.in <- ChokeMessage{}
	p.Choked = true
}

func (p *Peer) Unchoke() {
	if !p.Choked {
		return
	}
	p.in <- UnchokeMessage{}
	p.Choked = false
}

func (p *Peer) IsBlocking() {
	p.Blocking = true
}

func (p *Peer) NotBlocking() {
	p.Blocking = false
}

func NewPeer(conn net.Conn) *Peer {
	return &Peer{
		Conn:                conn,
		in:                  make(chan Message, 10),
		out:                 make(chan Message, 10),
		Blocking:            true,
		Choked:              true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
	}
}

// best-guess, ignore write stream
func isAlive(conn net.Conn) bool {
	_, err := conn.Read([]byte{})
	if err != nil && errors.IsEOF(err) {
		return false
	}

	return true
}
