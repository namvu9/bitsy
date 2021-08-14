package bencode

import (
	"bytes"
)

// Dictionary represents a bencoded dictionary
type Dictionary struct {
	keys   [][]byte
	values []Value
}

func (d *Dictionary) Type() Type {
	return TypeDict
}

func (d *Dictionary) ToDict() (*Dictionary, bool) {
	return d, true
}
func (d *Dictionary) ToList() (List, bool) {
	return List{}, false
}
func (d *Dictionary) ToInteger() (Integer, bool) {
	return Integer(0), false
}

func (d *Dictionary) ToBytes() (Bytes, bool) {
	return Bytes{}, false
}

// Value returns the underlying value
func (d *Dictionary) Value() interface{} {
	if d == nil {
		return nil
	}

	out := make(map[string]interface{})

	for i, key := range d.keys {
		out[string(key)] = d.values[i].Value()
	}

	return out
}

// Size returns the size of the dict in bytes
func (d *Dictionary) Size() int {
	data, err := Marshal(d)
	if err != nil {
		return 0
	}

	return len(data)
}

func (d *Dictionary) Get(key []byte) (Value, bool) {
	for i, k := range d.keys {
		if bytes.Compare(k, key) == 0 {
			return d.values[i], true
		}
	}

	return nil, false
}

func (d *Dictionary) Entries() map[string]interface{} {
	v := d.Value()
	return v.(map[string]interface{})
}

func (d *Dictionary) SetStringKey(key string, value Value) error {
	return d.Set([]byte(key), value)
}

// Set inserts a key-value pair into the dictionary at a
// lexicographically ordered position
func (d *Dictionary) Set(key []byte, value Value) error {
	for i, k := range d.keys {
		cmp := bytes.Compare(k, key)
		if cmp == 0 {
			d.values[i] = value
			return nil
		} else if cmp > 0 {
			var keys = make([][]byte, len(d.keys)+1)
			var values = make([]Value, len(d.values)+1)

			copy(keys, d.keys[:i])
			copy(values, d.values[:i])

			keys[i] = key
			values[i] = value

			copy(keys[i+1:], d.keys[i:])
			copy(values[i+1:], d.values[i:])

			d.keys = keys
			d.values = values

			return nil
		}
	}

	d.keys = append(d.keys, key)
	d.values = append(d.values, value)

	return nil
}

func (d *Dictionary) Keys() [][]byte {
	return d.keys
}

func (d *Dictionary) Values() []Value {
	var out []Value
	for _, value := range d.values {
		out = append(out, value)
	}

	return out
}

func (d *Dictionary) GetDict(key string) (*Dictionary, bool) {
	val, ok := d.Get([]byte(key))
	if !ok {
		return nil, false
	}
	return val.ToDict()
}

func (d *Dictionary) GetString(key string) (string, bool) {
	val, ok := d.Get([]byte(key))
	if !ok {
		return "", false
	}
	return string(val.Value().(Bytes)), ok
}

func (d *Dictionary) GetBytes(key string) (Bytes, bool) {
	val, ok := d.Get([]byte(key))
	if !ok {
		return Bytes{}, false
	}
	return val.ToBytes()
}

func (d *Dictionary) GetList(key string) (List, bool) {
	val, ok := d.Get([]byte(key))
	if !ok {
		return List{}, false
	}

	return val.ToList()
}

func (d *Dictionary) GetInteger(key string) (Integer, bool) {
	val, ok := d.Get([]byte(key))
	if !ok {
		return Integer(0), false
	}

	return val.ToInteger()
}
