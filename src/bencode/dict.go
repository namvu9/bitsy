package bencode

import (
	"encoding/hex"
	"strings"
)

type Dictionary struct {
	keys   []string
	values []Value
}

func (d *Dictionary) Type() string { return "Dictionary" }
func (d *Dictionary) Value() interface{} {
	out := make(map[string]Value)

	for i, key := range d.keys {
		out[key] = d.values[i]
	}

	return out
}

func (d *Dictionary) Get(key []byte) (Value, bool) {
	for i, k := range d.keys {
		if k == hex.EncodeToString(key) {
			return d.values[i], true
		}
	}

	return nil, false
}

// Set inserts a key-value pair into the dictionary at a
// lexicographically ordered position
func (d *Dictionary) Set(key []byte, value Value) error {
	for i, k := range d.keys {
		cmp := strings.Compare(k, hex.EncodeToString(key))
		if cmp == 0 {
			d.values[i] = value
			return nil
		} else if cmp > 0 {
			var keys = make([]string, len(d.keys)+1)
			var values = make([]Value, len(d.values)+1)

			copy(keys, d.keys[:i])
			copy(values, d.values[:i])

			keys[i] = hex.EncodeToString(key)
			values[i] = value

			copy(keys[i+1:], d.keys[i:])
			copy(values[i+1:], d.values[i:])

			d.keys = keys
			d.values = values

			return nil
		}
	}

	d.keys = append(d.keys, hex.EncodeToString(key))
	d.values = append(d.values, value)

	return nil
}

func (d *Dictionary) Keys() [][]byte {
	var out [][]byte
	for _, key := range d.keys {
		b, _ := hex.DecodeString(key)
		out = append(out, b)
	}

	return out 
}

func (d *Dictionary) Values() []Value {
	var out []Value
	for _, value := range d.values {
		out = append(out, value)
	}

	return out
}
