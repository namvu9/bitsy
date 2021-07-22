package bencode

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"strconv"
)

type Unmarshaler struct {
	offset int64
	io.Reader
	io.ByteScanner
	r interface {
		io.Reader
		io.ByteScanner
	}
}

// Unmarshal parses the bencoded data and stores the result
// in the value pointed to by v.
func (u *Unmarshaler) Unmarshal(v *Value) error {
	b, err := u.r.ReadByte()
	if err != nil {
		return err
	}

	if v == nil {
		return &InvalidUnmarshalError{message: "Cannot unmarshal into nil pointer"}
	}

	switch b {
	case 'i':
		return u.unmarshalInt(v)
	case 'l':
		return u.unmarshalList(v)
	case 'd':
		return u.unmarshalDict(v)
	default:
		err := u.r.UnreadByte()
		if err != nil {
			return err
		}
		return u.unmarshalBytes(v)
	}
}

func (u *Unmarshaler) unmarshalInt(i *Value) error {
	var bs []byte

	for {
		b, err := u.r.ReadByte()
		if err != nil {
			return err
		}

		if b == 'e' {
			input := string(bs)
			if match, err := regexp.MatchString(`(^-?0\d+$)|(^-0$)`, input); match || err != nil {
				return fmt.Errorf("Invalid Integer: %s", input)
			}

			n, err := strconv.ParseInt(input, 0, 0)
			if err != nil {
				return err
			}

			*i = Integer(n)
			return nil
		}

		bs = append(bs, b)
	}
}

func (u *Unmarshaler) unmarshalBytes(v *Value) error {
	var buf []byte

	for {
		b, err := u.r.ReadByte()
		if err != nil {
			return err
		}

		if b == ':' {
			n := string(buf)
			if n == "-0" {
				return fmt.Errorf("Syntax error: -0")
			}

			if n == "0" {
				*v = Bytes([]byte{})
				return nil
			}

			number, err := strconv.ParseInt(n, 0, 0)
			if err != nil {
				return err
			}

			var out = make([]byte, number)
			nread, err := u.r.Read(out)
			if nread < int(number) {
				return fmt.Errorf("Unexpected EOF")
			}
			if err != nil {
				return err
			}

			*v = Bytes(out)

			return nil
		}

		buf = append(buf, b)
	}

}

func (u *Unmarshaler) unmarshalList(v *Value) error {
	var out []Value

	for {
		b, err := u.r.ReadByte()
		if err != nil {
			return err
		}

		if b == 'e' {
			*v = List(out)
			return nil
		}

		err = u.r.UnreadByte()
		if err != nil {
			return err
		}

		var item Value
		err = u.Unmarshal(&item)
		if err != nil {
			return err
		}

		out = append(out, item)
	}
}

func (u *Unmarshaler) unmarshalDict(v *Value) error {
	var dict Dictionary

	for {
		b, err := u.r.ReadByte()
		if err != nil {
			return err
		}

		if b == 'e' {
			*v = &dict
			return nil
		}

		err = u.r.UnreadByte()
		if err != nil {
			return err
		}

		var key Value
		err = u.Unmarshal(&key)
		if err != nil {
			return err
		}

		var val Value
		err = u.Unmarshal(&val)
		if err != nil {
			return err
		}

		dict.Set(key.Value().(Bytes), val)
	}
}

// Unmarshal parses the bencoded data and stores the result
// in the value pointed to by v.
func Unmarshal(data []byte, v *Value) error {
	u := Unmarshaler{r: bytes.NewReader(data)}
	return u.Unmarshal(v)
}

func NewUnmarshaler(r io.Reader) *Unmarshaler {
	return &Unmarshaler{r: bufio.NewReader(r)}
}

type InvalidUnmarshalError struct {
	Type    reflect.Type
	message string
}

func (e *InvalidUnmarshalError) Error() string {
	return e.message
}
