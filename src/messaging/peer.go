package messaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/namvu9/bitsy/src/bits"
	"github.com/namvu9/bitsy/src/errors"
	"github.com/rs/zerolog/log"
)

// Peer represents a connection to another peer in the
// swarm
type Peer struct {
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

	// A bitfield specifying the indexes of the pieces that
	// the peer has
	BitField bits.BitField

	LastMessageReceived time.Time
	LastMessageSent     time.Time

	// Pending request messages
	// TODO: Remove these once fulfilled
	requests []RequestMessage
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

func (p *Peer) Unchoke() {
	p.Choked = false
}

// Listen for incoming messages
func (c *Peer) Listen(handleMessage func(*Peer, []byte, error)) {
	c.SetReadDeadline(time.Now().Add(30 * time.Second))
	for {
		buf := make([]byte, 4)
		n, err := c.Read(buf)
		if err != nil {
			handleMessage(c, nil, err)
			return
		}

		if n < 4 {
			handleMessage(c, nil, io.EOF)
			return
		}

		// Keep-alive - ignored
		messageLength := binary.BigEndian.Uint32(buf)
		if messageLength == 0 {
			continue
		}

		if messageLength > 32*1024 {
			log.Info().Msgf("%s Received packet of length: %d/%d, ignoring\n", c.RemoteAddr(), messageLength, len(buf))
			handleMessage(c, nil, io.EOF)
			return
		}

		buf = make([]byte, messageLength)
		n, err = c.Read(buf)

		if n < int(messageLength) {
			firstPart := buf[:n]

			time.Sleep(5 * time.Second)
			buf = make([]byte, messageLength-uint32(n))
			n, err := c.Read(buf)
			if err != nil {
				handleMessage(c, nil, err)
				return
			}

			if uint32(n) != messageLength-uint32(len(firstPart)) {
				continue
			}

			firstPart = append(firstPart, buf...)
			handleMessage(c, firstPart, nil)
			c.LastMessageReceived = time.Now()
			continue
		}

		c.LastMessageReceived = time.Now()
		handleMessage(c, buf, nil)
	}
}

// Handshake attempts to perform a handshake with the given
// client at addr with infoHash.
func Handshake(conn net.Conn, infoHash [20]byte, peerID [20]byte) (*Peer, error) {
	fmt.Println("HANDSHAKE")
	msg := HandShakeMessage{
		infoHash: infoHash,
		peerID:   peerID,
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(msg.Bytes())
	if err != nil {
		return nil, err
	}

	c := &Peer{
		Conn:                conn,
		Choked:              true,
		Blocking:            true,
		LastMessageReceived: time.Now(),
		LastMessageSent:     time.Now(),
	}

	return c, nil
}

func unmarshalHandshake(r io.Reader, m *HandShakeMessage) error {
	var op errors.Op = "messaging.unmarshalHandshake"
	var buf bytes.Buffer

	n, err := io.CopyN(&buf, r, 68)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	if n != 68 {
		err := errors.Newf("expected handshake message of length %d but got %d", 68, n)
		return errors.Wrap(err, op, errors.BadArgument)
	}

	data := buf.Bytes()

	m.pStrLen = data[0]
	m.pStr = data[1:20]

	if n := copy(m.reserved[:], data[20:28]); n != 8 {
		err := errors.Newf("Reserved bytes must be of length 8, got %d", n)
		return errors.Wrap(err, op, errors.BadArgument)
	}

	//if (m.reserved[5] & 0x10) == 0x10 {
	//fmt.Println("SUPPORTS EXTENSION PROTOCOL")
	//} else {
	//fmt.Println("DOES NOT SUPPORT EXTENSION PROTOCOL")
	//}

	if n := copy(m.infoHash[:], data[28:48]); n != 20 {
		err := errors.Newf("Info hash must be of length 20, got %d", n)
		return errors.Wrap(err, op, errors.BadArgument)
	}

	// TODO: PeerIDs are not returned by UDP trackers, which
	// is currently not supported
	//peerID := data[48:]

	return nil
}
