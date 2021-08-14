package bencode

import (
	"bytes"
	"fmt"
	"io"
)

type Marshaler struct {
	w io.Writer
}

func (m *Marshaler) Marshal(v Value) (int, error) {
	if i, ok := v.ToInteger(); ok {
		return m.encodeInt(i)
	}

	if b, ok := v.ToBytes(); ok {
		return m.encodeBytes(b)
	}

	if l, ok := v.ToList(); ok {
		return m.encodeList(l)
	}

	if d, ok := v.ToDict(); ok {
		return m.encodeDict(d)
	}

	return 0, fmt.Errorf("InvalidMarshalError")
}

func (m *Marshaler) encodeInt(i Integer) (int, error) {
	s := fmt.Sprintf("i%de", i)
	return m.w.Write([]byte(s))
}

func (m *Marshaler) encodeBytes(b []byte) (int, error) {
	s := fmt.Sprintf("%d:%s", len(b), b)
	return m.w.Write([]byte(s))
}

func (m *Marshaler) encodeList(v List) (int, error) {
	var nBytes int

	n, err := m.w.Write([]byte("l"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	for _, item := range v {
		n, err := m.Marshal(item)
		nBytes += n
		if err != nil {
			return nBytes, err
		}
	}

	n, err = m.w.Write([]byte("e"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	return nBytes, nil
}

func (m *Marshaler) encodeDict(d *Dictionary) (int, error) {
	if d == nil {
		return 0, nil
	}
	var nBytes int

	n, err := m.w.Write([]byte("d"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	for i, k := range d.keys {
		if d.values[i].Value() == nil {
			continue
		}

		n, err := m.encodeBytes(k)
		nBytes += n
		if err != nil {
			return nBytes, err
		}

		n, err = m.Marshal(d.values[i])
		nBytes += n
		if err != nil {
			return nBytes, err
		}
	}

	n, err = m.w.Write([]byte("e"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	return nBytes, nil
}

type InvalidMarshalError struct {
	err     error
	message string
}

func (e *InvalidMarshalError) Error() string {
	return fmt.Errorf("InvalidMarshalError: %s %w", e.message, e.err).Error()
}

func NewMarshaler(w io.Writer) *Marshaler {
	return &Marshaler{w: w}
}

func Marshal(v Value) ([]byte, error) {
	var buf bytes.Buffer
	m := NewMarshaler(&buf)

	_, err := m.Marshal(v)

	return buf.Bytes(), err
}
