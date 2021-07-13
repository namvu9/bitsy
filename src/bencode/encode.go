package bencode

import (
	"encoding/hex"
	"fmt"
	"io"
)

type Marshaler struct {
	w io.Writer
}

type InvalidMarshalError struct {
	err     error
	message string
}

func (e *InvalidMarshalError) Error() string {
	return fmt.Errorf("InvalidMarshalError: %s %w", e.message, e.err).Error()
}

func encodeInt(i Integer, w io.Writer) (int, error) {
	s := fmt.Sprintf("i%de", i)
	return w.Write([]byte(s))
}

func encodeBytes(b []byte, w io.Writer) (int, error) {
	s := fmt.Sprintf("%d:%s", len(b), b)
	return w.Write([]byte(s))
}

func encodeList(v List, w io.Writer, e *Marshaler) (int, error) {
	var nBytes int

	n, err := w.Write([]byte("l"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	for _, item := range v {
		n, err := e.Marshal(item)
		nBytes += n
		if err != nil {
			return nBytes, err
		}
	}

	n, err = w.Write([]byte("e"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	return nBytes, nil
}

func encodeDicts(d *Dictionary, w io.Writer, e *Marshaler) (int, error) {
	var nBytes int

	n, err := w.Write([]byte("d"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	for i, k := range d.keys {
		hexKey, err := hex.DecodeString(k)
		if err != nil {
			return n, err
		}
		n, err := encodeBytes(hexKey, w)
		nBytes += n
		if err != nil {
			return nBytes, err
		}

		n, err = e.Marshal(d.values[i])
		nBytes += n
		if err != nil {
			return nBytes, err
		}
	}

	n, err = w.Write([]byte("e"))
	nBytes += n
	if err != nil {
		return nBytes, err
	}

	return nBytes, nil
}

func (e *Marshaler) Marshal(v Value) (int, error) {
	switch v.Type() {
	case "Integer":
		i, ok := v.Value().(Integer)
		if !ok {
			return 0, &InvalidMarshalError{}
		}
		return encodeInt(i, e.w)
	case "Bytes":
		b, ok := v.(Bytes)
		if !ok {
			return 0, &InvalidMarshalError{}
		}
		return encodeBytes(b, e.w)
	case "List":
		l, ok := v.(List)
		if !ok {
			return 0, &InvalidMarshalError{}
		}
		return encodeList(l, e.w, e)
	case "Dictionary":
		d, ok := v.(*Dictionary)
		if !ok {
			return 0, &InvalidMarshalError{}
		}
		return encodeDicts(d, e.w, e)
	}

	return 0, fmt.Errorf("InvalidMarshalError")
}

func NewMarshaler(w io.Writer) *Marshaler {
	return &Marshaler{w: w}
}
