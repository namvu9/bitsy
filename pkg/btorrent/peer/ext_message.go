package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/namvu9/bencode"
)

// This file contains messages needed to implement the
// extension negotiation protocol defined in BEP-10 (see
// https://www.bittorrent.org/beps/bep_0010.html)

// Extension Message IDs. Note that these are local to this
// client.
const (
	EXT_CODE_HANDSHAKE = 0
	EXT_CODE_META      = 2
)

const (
	EXT_HANDSHAKE    = "handshake"
	EXT_UT_META      = "ut_metadata"
	EXT_UT_META_SIZE = "metadata_size"
)

// ExtHandshakeMsg represents the extended handshake
// message defined in BEP-10 and is used to dynamically
// negotiate extensions to the BitTorrent protocol between
// the client and peer
type ExtHandshakeMsg struct {
	d bencode.Dictionary
}

// M returns a bencoded dictionary mapping extension message
// names to integer IDs. Note that mappings are local to
// each peer and thus cannot be assumed to be valid for any
// other peer.
func (msg *ExtHandshakeMsg) M() *bencode.Dictionary {
	m, ok := msg.d.GetDict("m")
	if !ok {
		m = &bencode.Dictionary{}
		msg.d.SetStringKey("m", m)
	}

	return m
}

// D returns the top-level bencoded dictionary that makes up
// the payload of the handshake message.
func (msg *ExtHandshakeMsg) D() *bencode.Dictionary {
	return &msg.d
}

func (msg *ExtHandshakeMsg) Bytes() []byte {
	var buf bytes.Buffer

	payload, _ := bencode.Marshal(&msg.d)

	binary.Write(&buf, binary.BigEndian, uint32(len(payload)+2))
	buf.WriteByte(Extended)
	buf.WriteByte(EXT_CODE_HANDSHAKE)
	buf.Write(payload)

	return buf.Bytes()
}

func UnmarshalExtMessage(data []byte) (Message, error) {
	msgType := data[0]

	switch msgType {
	case EXT_CODE_HANDSHAKE:
		return unmarshalExtHandshakeMsg(data[1:])
	case EXT_CODE_META:
		return unmarshalMetaMsg(data[1:])
	}

	return nil, nil
}

func unmarshalExtHandshakeMsg(data []byte) (*ExtHandshakeMsg, error) {
	var msg ExtHandshakeMsg

	d, err := bencode.UnmarshalDict(data)
	if err != nil {
		return nil, err
	}

	msg.d = *d
	m, ok := d.GetDict("m")
	if !ok {
		return nil, fmt.Errorf("malformed data")
	}

	msg.d.SetStringKey("m", m)

	return &msg, nil
}
