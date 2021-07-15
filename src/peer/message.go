package peer

import (
	"bytes"
)

// Message represents a message sent between peers
type Message interface {
	Type() int32
	Payload() []byte
}

type HandShakeMessage struct {
	infoHash [20]byte
	peerID   [20]byte
}

func (m HandShakeMessage) Type() int32 {
	return -1
}

func (m HandShakeMessage) Payload() []byte {
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

// KeepAlive Messages are zero-length messages, and ignored. Keepalives are generally sent once every two minutes, but note that timeouts can be done much more quickly when data is expected.
type KeepAliveMessage struct{}

func (k KeepAliveMessage) Type() int32     { return -1 }
func (k KeepAliveMessage) Payload() []byte { return []byte{} }

// ChokeMessage has no payload
type ChokeMessage struct{}

func (m ChokeMessage) Type() int32     { return 0 }
func (m ChokeMessage) Payload() []byte { return []byte{} }

// UnchokeMessage has no payload
type UnchokeMessage struct{}

func (m UnchokeMessage) Type() int32     { return 1 }
func (m UnchokeMessage) Payload() []byte { return []byte{} }

// InterestedMessage has no payload
type InterestedMessage struct{}

func (m InterestedMessage) Type() int32     { return 2 }
func (m InterestedMessage) Payload() []byte { return []byte{} }

// NotInterestedMessage has no payload
type NotInterestedMessage struct{}

func (m NotInterestedMessage) Type() int32     { return 3 }
func (m NotInterestedMessage) Payload() []byte { return []byte{} }

// HaveMessage - The 'have' message's payload is a single
// number, the index which that downloader just completed
// and checked the hash of.  A Have message is sent to
// notify peers that the client has downloaded and verified
// the integrity of a piece
type HaveMessage struct{}

func (m HaveMessage) Type() int32     { return 4 }
func (m HaveMessage) Payload() []byte { return []byte{} }

// BitFieldMessage ... 'bitfield' is only ever sent as the
// first message. Its payload is a bitfield with each index
// that downloader has set to 1 and the rest set to 0.
// Downloaders which don't have anything yet may skip the
// 'bitfield' message. The first byte of the bitfield
// corresponds to indices 0 - 7 from high bit to low bit,
// respectively. The next one 8-15, etc. Spare bits at the
// end are set to zero.
type BitFieldMessage struct{}

func (m BitFieldMessage) Type() int32     { return 5 }
func (m BitFieldMessage) Payload() []byte { return []byte{} }

// RequestMessage 'request' messages contain an index, begin, and length. The last two are byte offsets. Length is generally a power of two unless it gets truncated by the end of the file. All current implementations use 2^14 (16 kiB), and close connections which request an amount greater than that.
type RequestMessage struct{}

func (m RequestMessage) Type() int32     { return 6 }
func (m RequestMessage) Payload() []byte { return []byte{} }

// PieceMessage 'piece' messages contain an index, begin, and piece. Note that they are correlated with request messages implicitly. It's possible for an unexpected piece to arrive if choke and unchoke messages are sent in quick succession and/or transfer is going very slowly.
type PieceMessage struct{}

func (m PieceMessage) Type() int32     { return 7 }
func (m PieceMessage) Payload() []byte { return []byte{} }

// CancelMessage 'cancel' messages have the same payload as request messages. They are generally only sent towards the end of a download, during what's called 'endgame mode'. When a download is almost complete, there's a tendency for the last few pieces to all be downloaded off a single hosed modem line, taking a very long time. To make sure the last few pieces come in quickly, once requests for all pieces a given downloader doesn't have yet are currently pending, it sends requests for everything to everyone it's downloading from. To keep this from becoming horribly inefficient, it sends cancels to everyone else every time a piece arrives.
type CancelMessage struct{}

func (m CancelMessage) Type() int32     { return 8 }
func (m CancelMessage) Payload() []byte { return []byte{} }

// Extension message
type ExtendedMessage struct{}

func (m ExtendedMessage) Type() int32     { return 20 }
func (m ExtendedMessage) Payload() []byte { return []byte{} }
