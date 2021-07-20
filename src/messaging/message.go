package messaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/namvu9/bitsy/src/errors"
)

// BitTorrent message types
const (
	Choke         byte = 0
	Unchoke       byte = 1
	Interested    byte = 2
	NotInterested byte = 3
	Have          byte = 4
	BitField      byte = 5
	Request       byte = 6
	Piece         byte = 7
	Cancel        byte = 8
	Extended      byte = 20
)

type Message interface {
	Bytes() []byte
}

type HandShakeMessage struct {
	pStrLen  byte
	pStr     []byte // Protocol string
	reserved [8]byte
	infoHash [20]byte
	peerID   [20]byte
}

func (m HandShakeMessage) Bytes() []byte {
	var buf bytes.Buffer

	buf.WriteByte(19)
	buf.WriteString("BitTorrent protocol")

	reserved := make([]byte, 8)

	// This client supports the 'Extension Protocol' (BEP-10)
	// and thus support message type 20 (extended message)
	reserved[5] &= 0x10 // ext

	buf.Write(reserved)

	// infohash
	buf.Write(m.infoHash[:])
	// Peer ID
	buf.Write(m.peerID[:])

	return buf.Bytes()
}

type KeepAliveMessage struct{}

func (m KeepAliveMessage) Bytes() []byte { return []byte{} }

type ChokeMessage struct{}

func (m ChokeMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(1))
	buf.WriteByte(Choke)

	return buf.Bytes()
}

type UnchokeMessage struct{}

func (m UnchokeMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(1))
	buf.WriteByte(Unchoke)

	return buf.Bytes()

}

type InterestedMessage struct{}

func (m InterestedMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(1))
	buf.WriteByte(Interested)

	return buf.Bytes()
}

type NotInterestedMessage struct{}

func (m NotInterestedMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(1))
	buf.WriteByte(NotInterested)

	return buf.Bytes()
}

// HaveMessage - The 'have' message's payload is a single
// number, the index which that downloader just completed
// and checked the hash of.  A Have message is sent to
// notify peers that the client has downloaded and verified
// the integrity of a piece
//
// The peer protocol refers to pieces of the file by index
// as described in the metainfo file, starting at zero. When
// a peer finishes downloading a piece and checks that the
// hash matches, it announces
//that it has that piece to all of its peers.
type HaveMessage struct {
	Index uint32
}

func (m HaveMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(5))
	buf.WriteByte(Have)
	binary.Write(&buf, binary.BigEndian, m.Index)

	return buf.Bytes()
}

func (m HaveMessage) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\nHave Message:\n Index: %d\n", m.Index))
	return sb.String()
}

// BitFieldMessage ... 'bitfield' is only ever sent as the
// first message. Its payload is a bitfield with each index
// that downloader has set to 1 and the rest set to 0.
// Downloaders which don't have anything yet may skip the
// 'bitfield' message. The first byte of the bitfield
// corresponds to indices 0 - 7 from high bit to low bit,
// respectively. The next one 8-15, etc. Spare bits at the
// end are set to zero.
//
// A bitfield of the wrong length is considered an error.
// Clients should drop the connection if they receive
// bitfields that are not of the correct size, or if the
// bitfield has any of the spare bits set.
type BitFieldMessage struct {
	BitField []byte
}

func (m BitFieldMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(len(m.BitField)+1))
	buf.WriteByte(BitField)
	buf.Write(m.BitField)

	return buf.Bytes()
}

// RequestMessage 'request' messages contain an index, begin, and length. The last two are byte offsets. Length is generally a power of two unless it gets truncated by the end of the file. All current implementations use 2^14 (16 kiB), and close connections which request an amount greater than that.
// Should be 13 bytes
type RequestMessage struct {
	Index  uint32 // piece index
	Offset uint32 // offset within the piece
	Length uint32 // 2^14 / 16 KiB
}

func (m RequestMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(13))
	buf.WriteByte(byte(Request))
	binary.Write(&buf, binary.BigEndian, m.Index)
	binary.Write(&buf, binary.BigEndian, m.Offset)
	binary.Write(&buf, binary.BigEndian, m.Length)

	return buf.Bytes()
}

// PieceMessage contains piece (or sub-piece) data
type PieceMessage struct {
	Index  uint32
	Offset uint32
	Piece  []byte
}

func (m PieceMessage) Bytes() []byte {
	var buf bytes.Buffer

	msgLen := int32(len(m.Piece) + 8)

	binary.Write(&buf, binary.BigEndian, msgLen)
	binary.Write(&buf, binary.BigEndian, Piece)
	binary.Write(&buf, binary.BigEndian, m.Index)
	binary.Write(&buf, binary.BigEndian, m.Offset)
	binary.Write(&buf, binary.BigEndian, m.Piece)

	return buf.Bytes()
}

// CancelMessage 'cancel' messages have the same payload as
// request messages. They are generally only sent towards
// the end of a download, during what's called 'endgame
// mode'. When a download is almost complete, there's a
// tendency for the last few pieces to all be downloaded off
// a single hosed modem line, taking a very long time. To
// make sure the last few pieces come in quickly, once
// requests for all pieces a given downloader doesn't have
// yet are currently pending, it sends requests for
// everything to everyone it's downloading from. To keep
// this from becoming horribly inefficient, it sends cancels
// to everyone else every time a piece arrives.
// TODO: Cancel pending requests for a subpiece/piece
type CancelMessage struct {
	Index  uint32 // piece index
	Offset uint32 // offset within the piece
	Length uint32 // length of the sub-piece
}

func (m CancelMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(13))
	buf.WriteByte(Cancel)
	binary.Write(&buf, binary.BigEndian, m.Index)
	binary.Write(&buf, binary.BigEndian, m.Offset)
	binary.Write(&buf, binary.BigEndian, m.Length)

	return buf.Bytes()
}

// ExtendedMessage ...
// TODO:
type ExtendedMessage struct{}

// TODO: Implement
func (m ExtendedMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(0))
	buf.WriteByte(Extended)

	return buf.Bytes()
}

// Handshake attempts to perform a handshake with the given
// client at addr with infoHash.
func Handshake(conn net.Conn, infoHash [20]byte, peerID [20]byte) (*Peer, error) {
	msg := HandShakeMessage{
		infoHash: infoHash,
		peerID:   peerID,
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(msg.Bytes())
	if err != nil {
		return nil, err
	}

	return NewPeer(conn), nil
}

func unmarshalHandshake(r io.Reader, msg *HandShakeMessage) error {
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

	msg.pStrLen = data[0]
	msg.pStr = data[1:20]

	if n := copy(msg.reserved[:], data[20:28]); n != 8 {
		err := errors.Newf("Reserved bytes must be of length 8, got %d", n)
		return errors.Wrap(err, op, errors.BadArgument)
	}

	//if (m.reserved[5] & 0x10) == 0x10 {
	//fmt.Println("SUPPORTS EXTENSION PROTOCOL")
	//} else {
	//fmt.Println("DOES NOT SUPPORT EXTENSION PROTOCOL")
	//}

	if n := copy(msg.infoHash[:], data[28:48]); n != 20 {
		err := errors.Newf("Info hash must be of length 20, got %d", n)
		return errors.Wrap(err, op, errors.BadArgument)
	}

	// TODO: PeerIDs are not returned by UDP trackers, which
	// is currently not supported
	//peerID := data[48:]

	return nil
}

func UnmarshallMessage(r io.ReadCloser) (Message, error) {
	var op errors.Op = "messaging.UnmarshallMessage"

	buf := make([]byte, 4)

	_, err := r.Read(buf)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.Network)
	}

	// Keep-alive - ignored
	messageLength := binary.BigEndian.Uint32(buf)
	if messageLength == 0 {
		return KeepAliveMessage{}, nil
	}

	if messageLength > 32*1024 {
		err := errors.Newf("Received packet of length: %x, ignoring\n", messageLength)
		r.Close()
		return nil, errors.Wrap(err, op, io.EOF)
	}

	buf = make([]byte, messageLength)
	k := 0
	for {
		n, err := r.Read(buf[k:])
		if err != nil {
			return nil, errors.Wrap(err, op, errors.Network)
		}

		k += n
		if k == int(messageLength) {
			break
		}
	}

	var (
		messageType = buf[0]
		payload     = buf[1:]
	)

	switch messageType {
	case BitField:
		return UnmarshallBitFieldMessage(payload)
	case Choke:
		return ChokeMessage{}, nil
	case Unchoke:
		return UnchokeMessage{}, nil
	case Interested:
		return InterestedMessage{}, nil
	case NotInterested:
		return NotInterestedMessage{}, nil
	case Have:
		return UnmarshalHaveMessage(payload)
	case Request:
		return UnmarshalRequestMessage(payload)
	case Piece:
		return UnmarshallPieceMessage(payload)
	case Cancel:
		return UnmarshalCancelMessage(payload)
	case Extended:
		return UnmarshalExtendedMessage(payload)
	default:
		return KeepAliveMessage{}, nil
	}
}

// TODO: TESTS
func UnmarshallBitFieldMessage(data []byte) (BitFieldMessage, error) {
	// TODO: Check BitfieldLength
	// if error, return EOF
	return BitFieldMessage{data}, nil
}
func UnmarshalHaveMessage(data []byte) (HaveMessage, error) {
	var msg HaveMessage

	if len(data) != 4 {
		err := errors.Newf("have message payload longer than 4: %d", len(data))
		return msg, errors.Wrap(err, errors.BadArgument)
	}

	msg.Index = binary.BigEndian.Uint32(data)
	return msg, nil
}
func UnmarshalRequestMessage(data []byte) (RequestMessage, error) {
	var msg RequestMessage

	if got := len(data); got != 12 {
		err := errors.Newf("payload length, want %d but got %d", 12, got)
		return msg, errors.Wrap(err, errors.BadArgument)
	}

	msg.Index = binary.BigEndian.Uint32(data[:4])
	msg.Offset = binary.BigEndian.Uint32(data[4:8])
	msg.Length = binary.BigEndian.Uint32(data[8:12])

	return msg, nil
}

func UnmarshallPieceMessage(data []byte) (PieceMessage, error) {
	var msg PieceMessage

	msg.Index = binary.BigEndian.Uint32(data[:4])
	msg.Offset = binary.BigEndian.Uint32(data[4:8])
	msg.Piece = data[8:]

	return msg, nil
}

func UnmarshalCancelMessage(data []byte) (CancelMessage, error) {
	var msg CancelMessage

	if got := len(data); got != 12 {
		err := errors.Newf("payload length, want %d but got %d", 12, got)
		return msg, errors.Wrap(err, errors.BadArgument)
	}

	msg.Index = binary.BigEndian.Uint32(data[:4])
	msg.Offset = binary.BigEndian.Uint32(data[4:8])
	msg.Length = binary.BigEndian.Uint32(data[8:12])

	return msg, nil
}
func UnmarshalExtendedMessage(data []byte) (ExtendedMessage, error) {
	var msg ExtendedMessage
	return msg, nil
}
