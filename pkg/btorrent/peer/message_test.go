package peer_test

import (
	"bytes"
	"testing"

	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

func TestHandshakeMessage(t *testing.T) {
	msg := peer.HandshakeMessage{
		PStr:     "BitTorrent protocol",
		PStrLen:  19,
		InfoHash: [20]byte{1, 2, 3, 4},
		PeerID:   [20]byte{4, 3, 2, 1},
		Reserved: [8]byte{1, 3, 3, 7},
	}

	res := msg.Bytes()

	if len(res) != 68 {
		t.Errorf("len(handshakeMessage) want %d got %d", 68, len(res))
	}

	pStrLen := res[0]
	if pStrLen != 19 {
		t.Errorf("pstrlen want %d got %d", 19, pStrLen)
	}

	pStr := string(res[1:20])
	if pStr != msg.PStr {
		t.Errorf("pstr want %s got %s ", msg.PStr, pStr)
	}

	reserved := res[20:28]
	if !bytes.Equal(reserved, msg.Reserved[:]) {
		t.Errorf("Infohash want %v got %v", msg.Reserved, reserved)
	}

	infoHash := res[28:48]
	if !bytes.Equal(infoHash, msg.InfoHash[:]) {
		t.Errorf("Infohash want %v got %v", msg.InfoHash, infoHash)
	}

	peerID := res[48:68]
	if !bytes.Equal(peerID, msg.PeerID[:]) {
		t.Errorf("Infohash want %v got %v", msg.PeerID, peerID)
	}
}

func TestMessage(t *testing.T) {
	for i, test := range []struct {
		msg       peer.Message
		wantLen   int
		wantBytes []byte
	}{
		{
			msg:       peer.ChokeMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 0},
		},
		{
			msg:       peer.UnchokeMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 1},
		},
		{
			msg:       peer.InterestedMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 2},
		},
		{
			msg:       peer.NotInterestedMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 3},
		},
		{
			msg:       peer.HaveMessage{Index: 5},
			wantLen:   9,
			wantBytes: []byte{0, 0, 0, 5, 4, 0, 0, 0, 5},
		},
		{
			msg: peer.BitFieldMessage{
				BitField: []byte{1, 134, 155, 155, 0},
			},
			wantLen:   10,
			wantBytes: []byte{0, 0, 0, 6, 5, 1, 134, 155, 155, 0},
		},
		{
			msg: peer.RequestMessage{
				Index:  0,
				Offset: 1,
				Length: 134,
			},
			wantLen:   17,
			wantBytes: []byte{0, 0, 0, 13, 6, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 134},
		},
		{
			msg: peer.PieceMessage{
				Index:  0,
				Offset: 1,
				Piece:  []byte{1, 2, 3, 4, 5},
			},
			wantLen:   18,
			wantBytes: []byte{0, 0, 0, 13, 7, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 3, 4, 5},
		},
		{
			msg: peer.CancelMessage{
				Index:  0,
				Offset: 1,
				Length: 134,
			},
			wantLen:   17,
			wantBytes: []byte{0, 0, 0, 13, 8, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 134},
		},
	} {
		data := test.msg.Bytes()

		if got := len(data); got != test.wantLen {
			t.Errorf("%d: Want len %d got %d", i, test.wantLen, got)
		}

		if !bytes.Equal(data, test.wantBytes) {
			t.Errorf("%d: Want %v got %v", i, test.wantBytes, data)
		}
	}
}
