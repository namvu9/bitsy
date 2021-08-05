package btorrent

import (
	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/bits"
)

type Extensions struct {
	bits bits.BitField
	m    bencode.Dictionary
}

const (
	// LTEP Extension Protocol
	EXT_PROT = 44 // ?? 20th bit from the right
	EXT_FAST = 61
	EXT_DHT  = 63
)

func (ext *Extensions) Enable(bitIdx int) error {
	return ext.bits.Set(bitIdx)
}

func (ext *Extensions) IsEnabled(bitIdx int) bool {
	return ext.bits.Get(bitIdx)
}

func (ext *Extensions) Get() []int {
	var out []int
	for i := 0; i < 64; i++ {
		if ext.IsEnabled(i) {
			out = append(out, i)
		}
	}

	return out
}

func NewExtensionsField(bits [8]byte) *Extensions {
	return &Extensions{
		bits: bits[:],
	}
}
