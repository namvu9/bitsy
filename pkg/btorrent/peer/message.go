package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
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
	Suggest       byte = 13
	HaveAll       byte = 14
	HaveNone      byte = 15
	Reject        byte = 16
	AllowedFast   byte = 17
	Extended      byte = 20
)

type Message interface {
	Bytes() []byte
}

type HandshakeMessage struct {
	PStrLen  byte
	PStr     string // Protocol string
	Reserved [8]byte
	InfoHash [20]byte
	PeerID   [20]byte
}

func (m HandshakeMessage) Bytes() []byte {
	var buf bytes.Buffer

	buf.WriteByte(m.PStrLen)
	buf.WriteString(m.PStr)

	buf.Write(m.Reserved[:])

	// infohash
	buf.Write(m.InfoHash[:])
	// Peer ID
	buf.Write(m.PeerID[:])

	return buf.Bytes()
}

type KeepAliveMessage struct{}

func (m KeepAliveMessage) Bytes() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, int32(0))
	return buf.Bytes()
}

type ChokeMessage struct{}

func (m ChokeMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(1))
	buf.WriteByte(byte(Choke))

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

type HaveAllMessage struct{}

func (m HaveAllMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint32(1))
	buf.WriteByte(HaveAll)

	return buf.Bytes()
}

type HaveNoneMessage struct{}

func (m HaveNoneMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint32(1))
	buf.WriteByte(HaveNone)

	return buf.Bytes()
}

type AllowedFastMessage struct {
	Index uint32
}

func (m AllowedFastMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteByte(AllowedFast)
	binary.Write(&buf, binary.BigEndian, m.Index)

	return buf.Bytes()
}

type SuggestPieceMessage struct {
	Index uint32
}

func (m SuggestPieceMessage) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, uint32(5))
	buf.WriteByte(Suggest)
	binary.Write(&buf, binary.BigEndian, m.Index)

	return buf.Bytes()
}

type RejectRequestMsg struct {
	Index  uint32
	Offset uint32
	Length uint32
}

func (m RejectRequestMsg) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, int32(13))
	buf.WriteByte(byte(Reject))
	binary.Write(&buf, binary.BigEndian, m.Index)
	binary.Write(&buf, binary.BigEndian, m.Offset)
	binary.Write(&buf, binary.BigEndian, m.Length)

	return buf.Bytes()
}

func UnmarshalHandshake(r io.Reader, msg *HandshakeMessage) error {
	var buf bytes.Buffer

	n, err := io.CopyN(&buf, r, 68)
	if err != nil {
		return err
	}

	if n != 68 {
		return fmt.Errorf("expected handshake message of length %d but got %d", 68, n)
	}

	data := buf.Bytes()

	msg.PStrLen = data[0]
	msg.PStr = string(data[1:20])

	copy(msg.Reserved[:], data[20:28])
	copy(msg.Reserved[:], data[20:28])
	copy(msg.InfoHash[:], data[28:48])
	copy(msg.PeerID[:], data[48:])

	return nil
}

func UnmarshallMessage(r io.ReadCloser) (Message, error) {
	buf := make([]byte, 4)

	n, err := r.Read(buf)
	if n == 0 {
		return nil, io.EOF
	}

	if err != nil {
		return nil, err
	}

	// Keep-alive - ignored
	messageLength := binary.BigEndian.Uint32(buf)
	if messageLength == 0 {
		return KeepAliveMessage{}, nil
	}

	if messageLength > 32*1024 {
		return nil, fmt.Errorf("Received packet of length: %x, ignoring\n", messageLength)
	}

	buf = make([]byte, messageLength)
	k := 0
	for {
		n, err := r.Read(buf[k:])
		if err != nil {
			return nil, err
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
	case BitField:
		return UnmarshallBitFieldMessage(payload)
	case Request:
		return UnmarshalRequestMessage(payload)
	case Piece:
		return UnmarshallPieceMessage(payload)
	case Cancel:
		return UnmarshalCancelMessage(payload)
	case Extended:
		return UnmarshalExtMessage(payload)
	case HaveAll:
		return HaveAllMessage{}, nil
	case HaveNone:
		return HaveNoneMessage{}, nil
	case AllowedFast:
		return UnmarshalAllowedFastMessage(payload)
	case Reject:
		return UnmarshalRejectRequestMsg(payload)

	default:
		return KeepAliveMessage{}, nil
	}
}

func UnmarshalAllowedFastMessage(data []byte) (AllowedFastMessage, error) {
	if len(data) != 4 {
		return AllowedFastMessage{}, fmt.Errorf("corrupted data")
	}

	idx := binary.BigEndian.Uint32(data[:4])
	return AllowedFastMessage{Index: idx}, nil
}

// TODO: TESTS
func UnmarshallBitFieldMessage(data []byte) (BitFieldMessage, error) {
	// TODO: Check BitfieldLength
	return BitFieldMessage{data}, nil
}
func UnmarshalHaveMessage(data []byte) (HaveMessage, error) {
	var msg HaveMessage

	if len(data) != 4 {
		return msg, fmt.Errorf("have message payload longer than 4: %d", len(data))
	}

	msg.Index = binary.BigEndian.Uint32(data)
	return msg, nil
}
func UnmarshalRequestMessage(data []byte) (RequestMessage, error) {
	var msg RequestMessage

	if got := len(data); got != 12 {
		return msg, fmt.Errorf("payload length, want %d but got %d", 12, got)
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
		err := fmt.Errorf("payload length, want %d but got %d", 12, got)
		return msg, err
	}

	msg.Index = binary.BigEndian.Uint32(data[:4])
	msg.Offset = binary.BigEndian.Uint32(data[4:8])
	msg.Length = binary.BigEndian.Uint32(data[8:12])

	return msg, nil
}

func UnmarshalRejectRequestMsg(data []byte) (RejectRequestMsg, error) {
	var msg RejectRequestMsg

	if got := len(data); got != 12 {
		err := fmt.Errorf("payload length, want %d but got %d", 12, got)
		return msg, err
	}

	msg.Index = binary.BigEndian.Uint32(data[:4])
	msg.Offset = binary.BigEndian.Uint32(data[4:8])
	msg.Length = binary.BigEndian.Uint32(data[8:12])

	return msg, nil
}
