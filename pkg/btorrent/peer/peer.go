package peer

import (
	"context"
	"errors"
	stderrors "errors"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/bits"
)

// Peer represents a connection to another peer in the
// swarm
type Peer struct {
	ID []byte
	net.Conn
	closed   bool
	InfoHash [20]byte

	Msg        chan Message
	DataStream chan PieceMessage

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
	lastUploadTime time.Time
	lastUploaded   int64
	UploadRate     int64
	Uploaded       int64
	DownloadRate   int64
	Downloaded     int64

	// A bitfield specifying the indexes of the pieces that
	// the peer has
	Pieces bits.BitField

	// Extensions enabled by the peer's client
	Extensions *Extensions

	LastMessageReceived time.Time
	LastMessageSent     time.Time

	Requests []RequestMessage

	onClose []func(*Peer)
}

func (p *Peer) Client() string {
	return peerIDStr(p.ID)
}

func (p *Peer) ClientTag() string {
	return string(p.ID[:8])
}

func (p *Peer) IDStr() string {
	return fmt.Sprintf("%x", p.ID[8:])
}

func (p *Peer) Closed() bool {
	return p.closed
}

func (p *Peer) Close(reason string) error {
	if p == nil {
		return nil
	}

	if p.closed {
		return nil
	}

	for _, fn := range p.onClose {
		fn(p)
	}

	p.closed = true

	return p.Conn.Close()
}

func (p *Peer) OnClose(fn func(*Peer)) {
	p.onClose = append(p.onClose, fn)
}

func (p *Peer) IsServing(index uint32, offset uint32) bool {
	for _, req := range p.Requests {
		if req.Index == index && req.Offset == offset {
			return true
		}
	}

	return false
}

func (p *Peer) Send(msg Message) error {
	switch v := msg.(type) {
	case UnchokeMessage:
		p.Choked = false
	case ChokeMessage:
		p.Choked = true
	case InterestedMessage:
		if p.Interesting {
			return nil
		}
		p.Interesting = true
	case NotInterestedMessage:
		p.Interesting = false
	case RequestMessage:
		p.Requests = append(p.Requests, v)
	case PieceMessage:
		p.Downloaded += int64(len(v.Piece))
	}

	_, err := p.write(msg.Bytes())
	if err != nil {
		p.Close(err.Error())
		return err
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
		return n, err
	}

	return n, nil
}

func (peer *Peer) HasPiece(index int) bool {
	return peer.Pieces.Get(index)
}

// Listen for incoming messages
func (p *Peer) Listen(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			if p.Idle() {
				return
			}

			if diffTime := time.Now().Sub(p.lastUploadTime); diffTime > 5*time.Second {
				diff := p.Uploaded - p.lastUploaded
				p.lastUploaded = p.Uploaded
				p.lastUploadTime = time.Now()

				p.UploadRate = diff / (int64(diffTime.Seconds()))
			}

		case <-ctx.Done():
			return
		default:
		}

		p.Conn.SetDeadline(time.Now().Add(5 * time.Second))
		msg, err := UnmarshallMessage(p.Conn)
		p.LastMessageReceived = time.Now()

		if err != nil && errors.Is(err, io.EOF) {
			p.Close(err.Error())
			return
		}

		if err != nil && stderrors.Is(err, os.ErrDeadlineExceeded) {
			continue
		}

		if err != nil {
			p.Close(err.Error())
			return
		}

		switch v := msg.(type) {
		case HaveMessage:
			p.Pieces.Set(int(v.Index))
		case PieceMessage:
			p.Uploaded += int64(len(v.Piece))

			p.DataStream <- v
			var out []RequestMessage
			for _, msg := range p.Requests {
				if msg.Index != v.Index && msg.Offset != v.Offset {
					out = append(out, msg)
				}
			}

			p.Requests = out
		case ChokeMessage:
			p.Blocking = true
			p.Requests = make([]RequestMessage, 0)
		case UnchokeMessage:
			p.Blocking = false
		case InterestedMessage:
			p.Interested = true
			p.Send(UnchokeMessage{})
		case NotInterestedMessage:
			p.Interested = false
		case BitFieldMessage:
			p.Pieces = v.BitField
		case HaveAllMessage:
			p.Pieces = bits.Ones(len(p.Pieces))
		case *ExtHandshakeMsg:
			p.handleExtHandshakeMsg(v)
		case RejectRequestMsg:
			var out []RequestMessage
			for _, msg := range p.Requests {
				if msg.Index != v.Index {
					out = append(out, msg)
				}
			}

			p.Requests = out
		}

		p.Msg <- msg
	}
}

func (p *Peer) handleExtHandshakeMsg(msg *ExtHandshakeMsg) error {
	for key, value := range msg.M().Entries() {
		if v, ok := value.(bencode.Value); ok {
			p.Extensions.M().SetStringKey(key, v)
		}
	}

	for key, value := range msg.D().Entries() {
		if key == "m" {
			continue
		}

		if v, ok := value.(bencode.Value); ok {
			p.Extensions.D().SetStringKey(key, v)
		}
	}

	return nil
}

func (p *Peer) Init() error {
	var msg HandshakeMessage
	err := UnmarshalHandshake(p, &msg)
	if err != nil {
		return err
	}

	if msg.PStr != "BitTorrent protocol" {
		return fmt.Errorf("got %s want %s", msg.PStr, "BitTorrent protocol")
	}

	p.Extensions = NewExtensionsField(msg.Reserved)
	p.ID = msg.PeerID[:]
	p.InfoHash = msg.InfoHash

	return nil
}

func (p *Peer) Is(other *Peer) bool {
	return p.RemoteAddr() == other.RemoteAddr() || p == other
}

func New(c net.Conn) *Peer {
	peer := &Peer{
		Conn:                c,
		Blocking:            true,
		Choked:              true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
		lastUploadTime:      time.Now(),
		Msg:                 make(chan Message, 32),
		DataStream:          make(chan PieceMessage, 32),
	}
	return peer
}
