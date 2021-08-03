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
	EXT_PROT = 44 // ?? 20th bit from the right
	EXT_FAST = 61
)

func (ext *Extensions) Enable(bitIdx int) error {
	return ext.bits.Set(bitIdx)
}

func (ext *Extensions) IsEnabled(bitIdx int) bool {
	return ext.bits.Get(bitIdx)
}

func newExtensionsField() *Extensions {
	return &Extensions{
		bits: make(bits.BitField, 64),
	}
}
