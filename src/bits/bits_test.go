package bits_test

import (
	"testing"

	"github.com/namvu9/bitsy/src/bits"
)

func TestParseBitField(t *testing.T) {
	bitField := byte(0b11001101)

	got := bits.GetOnes(0, bitField)
	got2 := bits.GetOnes(1, bitField)

	want := []int{0, 1, 4, 5, 7}
	want2 := []int{8, 9, 12, 13, 15}

	if len(got) != len(want) {
		t.Errorf("Want len %d got %d", len(want), len(got))
	}
	if len(got2) != len(want2) {
		t.Errorf("Want len %d got %d", len(want), len(got))
	}

	for i, index := range want {
		gotIndex := got[i]

		if gotIndex != index {
			t.Errorf("%d: Want %d got %d", i, index, gotIndex)
		}
	}

	for i, index := range want2 {
		gotIndex := got2[i]

		if gotIndex != index {
			t.Errorf("%d: Want %d got %d", i, index, gotIndex)
		}
	}
}

func TestIndexSet(t *testing.T) {
	for i, test := range []struct {
		bitField bits.BitField
		index    int
		want     bool
	}{
		{
			bitField: []byte{0b11111111, 0b10000000},
			index:    8,
			want:     true,
		},
		{
			bitField: []byte{0b11111111, 0b10000000},
			index:    9,
			want:     false,
		},
		{
			bitField: []byte{0b11111110, 0b10000000},
			index:    7,
			want:     false,
		},
	} {

		got := test.bitField.Get(test.index)

		if got != test.want {
			t.Errorf("%d: Want %v got %v", i, test.want, got)
		}

	}
}

func TestGetMaxIndex(t *testing.T) {
	for i, test := range []struct {
		bitField []byte
		wantIdx  int
	}{
		{
			bitField: []byte{0b11111111, 0b11111111, 0b10010000, 0},
			wantIdx:  19,
		},
		{
			bitField: []byte{0, 0, 0b10010000, 0},
			wantIdx:  19,
		},
		{
			bitField: []byte{0, 0, 0, 0},
			wantIdx:  0,
		},
		{
			bitField: []byte{0, 0, 0, 0, 0, 0, 1},
			wantIdx:  55,
		},
		{
			bitField: []byte{1, 0, 0, 0, 0, 0, 0},
			wantIdx:  7,
		},
	} {
		res := bits.GetMaxIndex(test.bitField)

		if res != test.wantIdx {

			t.Errorf("%d: Want %d got %d", i, test.wantIdx, res)
		}
	}
}

func TestNewBitField(t *testing.T) {
	for i, test := range []struct {
		bits int
		want int // slice length
	} {
		{
			bits: 80,
			want: 10,
		},
		{
			bits: 81,
			want: 11,
		},
		{
			bits: 79,
			want: 10,
		},
	} {

		bf := bits.NewBitField(test.bits)

		if len(bf) != test.want {
			t.Errorf("%d: Want %d got %d", i, test.want, len(bf))
		}
	}
}
