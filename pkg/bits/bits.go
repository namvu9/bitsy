package bits

import "fmt"

type BitField []byte

func (b BitField) Bytes() []byte {
	return []byte(b)
}

// TODO: TEST
// Get the total number of set (1) bits
func (bf BitField) GetSum() int {
	var sum int
	for _, b := range bf {
		for i := 0; i < 8; i++ {
			bitMask := byte(128 >> i)
			if (b & bitMask) == bitMask {
				sum++
			}
		}
	}

	return sum
}

// TODO: TEST
func GetMaxIndex(bitField []byte) int {
	i := len(bitField) - 1
	for i >= 0 {
		if b := bitField[i]; b > 0 {
			for j := 0; j < 8; j++ {
				mask := byte(1 << j)
				if (b & mask) == mask {
					offset := i * 8
					localIndex := 7 - j
					return offset + localIndex
				}

			}
		}

		i--
	}

	return 0
}

// GetOnes returns the indices of the set (1) bits of the
// bitfield
//
// Example: 
// GetOnes([]byte{0b11000000}) -> []int{0, 1}
// GetOnes([]byte{128, 128}) -> []int{0, 8}
func GetOnes(offset int, bitField byte) []int {
	var out []int
	startIndex := offset * 8

	for i := 0; i < 8; i++ {
		bitMask := byte(128 >> i)
		if (bitField & bitMask) == bitMask {
			out = append(out, startIndex+i)
		}
	}

	return out
}

func (b BitField) Get(index int) bool {
	var (
		offset     = index / 8
		localIndex = index % 8
		bitMask    = byte(128 >> localIndex)
	)

	// TODO: Test this case
	if offset >= len(b) {
		return false
	}

	bit := b[offset]

	return (bit & bitMask) == bitMask
}

// TODO: TEST
func (b BitField) Set(index int) error {
	var (
		offset     = index / 8
		localIndex = index % 8
		bitMask    = byte(128 >> localIndex)
	)

	// TODO: Test this case
	if offset >= len(b) {
		return fmt.Errorf("Out of bounds")
	}

	b[offset] |= bitMask

	return nil
}

func (b BitField) Unset(index int) error {
	var (
		offset     = index / 8
		localIndex = index % 8
		bitMask    = byte(128 >> localIndex)
	)

	// TODO: Test this case
	if offset >= len(b) {
		return fmt.Errorf("Out of bounds")
	}

	// Unset bit
	b[offset] &^= bitMask
	return nil
}

// Len returns the number of bits in the bitfield
func (b BitField) Len() int {
	return len(b) * 8
}

// Ones returns an n-length bitfields with all bits set to 1
func Ones(n int) BitField {
	bf := NewBitField(n)
	for i := 0; i < n; i++ {
		bf.Set(i)
	}

	return bf
}

func NewBitField(bits int) BitField {
	if bits%8 == 0 {
		return make([]byte, bits/8)
	}

	return make([]byte, bits/8+1)
}
