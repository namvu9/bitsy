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

// ParseBitField parses a bitField and returns the index of
// the references pieces
func ParseBitField(offset int, bitField byte) []int {
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

func (b BitField) IsIndexSet(index int) bool {
	offset := index / 8
	localIndex := index % 8
	bitMask := byte(128 >> localIndex)

	// TODO: Test this case
	if offset >= len(b) {
		return false
	}

	bit := b[offset]

	return (bit & bitMask) == bitMask
}

// TODO: TEST
func (b BitField) SetIndex(index int) error {
	offset := index / 8
	localIndex := index % 8
	bitMask := byte(128 >> localIndex)

	// TODO: Test this case
	if offset >= len(b) {
		return fmt.Errorf("Out of bounds")
	}

	b[offset] |= bitMask

	return nil
}

func (b BitField) UnsetIndex(index int) {

}
