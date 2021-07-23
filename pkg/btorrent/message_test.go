package btorrent_test

import (
	"bytes"
	"testing"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

func TestMessage(t *testing.T) {
	for i, test := range []struct {
		msg       btorrent.Message
		wantLen   int
		wantBytes []byte
	}{
		{
			msg:       btorrent.ChokeMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 0},
		},
		{
			msg:       btorrent.UnchokeMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 1},
		},
		{
			msg:       btorrent.InterestedMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 2},
		},
		{
			msg:       btorrent.NotInterestedMessage{},
			wantLen:   5,
			wantBytes: []byte{0, 0, 0, 1, 3},
		},
		{
			msg:       btorrent.HaveMessage{Index: 5},
			wantLen:   9,
			wantBytes: []byte{0, 0, 0, 5, 4, 0, 0, 0, 5},
		},
		{
			msg: btorrent.BitFieldMessage{
				BitField: []byte{1, 134, 155, 155, 0},
			},
			wantLen:   10,
			wantBytes: []byte{0, 0, 0, 6, 5, 1, 134, 155, 155, 0},
		},
		{
			msg: btorrent.RequestMessage{
				Index:  0,
				Offset: 1,
				Length: 134,
			},
			wantLen:   17,
			wantBytes: []byte{0, 0, 0, 13, 6, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 134},
		},
		{
			msg: btorrent.PieceMessage{
				Index:  0,
				Offset: 1,
				Piece:  []byte{1, 2, 3, 4, 5},
			},
			wantLen:   18,
			wantBytes: []byte{0, 0, 0, 13, 7, 0, 0, 0, 0, 0, 0, 0, 1, 1, 2, 3, 4, 5},
		},
		{
			msg: btorrent.CancelMessage{
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