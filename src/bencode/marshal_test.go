package bencode_test

import (
	"bytes"
	"testing"

	"github.com/namvu9/bitsy/src/bencode"
)

func TestEncode(t *testing.T) {
	t.Run("Simple types", func(t *testing.T) {
		for i, test := range []struct {
			input bencode.Value
			want  []byte
		}{
			{input: bencode.Integer(42), want: []byte("i42e")},
			{input: bencode.Integer(0), want: []byte("i0e")},
			{input: bencode.Integer(-42), want: []byte("i-42e")},

			{input: bencode.Bytes(""), want: []byte("0:")},
			{input: bencode.Bytes("spam"), want: []byte("4:spam")},
			{input: bencode.Bytes("1234"), want: []byte("4:1234")},
		} {
			var dst bytes.Buffer

			enc := bencode.NewMarshaler(&dst)
			n, _ := enc.Marshal(test.input)

			if n != len(test.want) {
				t.Errorf("%d: Bytes written, want=%d got=%d", i, n, len(test.want))
			}

			if got := dst.Bytes(); !bytes.Equal(got, test.want) {
				t.Errorf("Write bytes: want %s got %s", test.want, got)
			}
		}
	})

	t.Run("List", func(t *testing.T) {
		for i, test := range []struct {
			input bencode.Value
			want  []byte
			err   bool
		}{
			{
				input: bencode.List{bencode.Integer(13), bencode.Integer(42)},
				want:  []byte("li13ei42ee"),
			},
			{
				input: bencode.List{bencode.Bytes("spam"), bencode.Integer(42)},
				want:  []byte("l4:spami42ee"),
			},
			{
				input: bencode.List{bencode.Bytes("spam"), bencode.Bytes("nam"), bencode.Bytes("lol")},
				want:  []byte("l4:spam3:nam3:lole"),
			},
		} {
			var dst bytes.Buffer
			enc := bencode.NewMarshaler(&dst)
			n, err := enc.Marshal(test.input)

			if test.err && err == nil || !test.err && err != nil {
				t.Errorf("%d: %s", i, err)
			}

			if n != len(test.want) {
				t.Errorf("%d: Bytes written, want=%d got=%d", i, n, len(test.want))
			}

			if got := dst.Bytes(); !bytes.Equal(got, test.want) {
				t.Errorf("Write bytes: want %s got %s", test.want, got)
			}
		}
	})

	t.Run("Dictionaries", func(t *testing.T) {
		var nilDict *bencode.Dictionary
		for i, test := range []struct {
			keys   [][]byte
			values []bencode.Value
			want   []byte
		}{
			{
				keys:   [][]byte{[]byte("bar"), []byte("foo")},
				values: []bencode.Value{bencode.Bytes("spam"), bencode.Integer(42)},
				want:   []byte("d3:bar4:spam3:fooi42ee"),
			},
			{
				keys:   [][]byte{[]byte("bar"), []byte("foo"), []byte("baz"), []byte("nil")},
				values: []bencode.Value{bencode.Bytes("spam"), bencode.Integer(42), bencode.List{bencode.Integer(100)}, nilDict},
				want:   []byte("d3:bar4:spam3:bazli100ee3:fooi42ee"),
			},
		} {
			var dst bytes.Buffer
			var d bencode.Dictionary

			for i, k := range test.keys {
				err := d.Set(k, test.values[i])
				if err != nil {
					t.Errorf("BLABLA")
				}
			}

			enc := bencode.NewMarshaler(&dst)
			n, _ := enc.Marshal(bencode.Value(&d))

			if n != len(test.want) {
				t.Errorf("%d: Bytes written, want=%d got=%d", i, n, len(test.want))
			}

			if got := dst.Bytes(); !bytes.Equal(got, test.want) {
				t.Errorf("Write bytes: want %s got %s", test.want, got)
			}
		}
	})

	t.Run("Nil Dict pointer", func(t *testing.T) {
		var d *bencode.Dictionary
		data, err := bencode.Marshal(d)

		if err != nil {
			t.Fatal(err)
		}

		if len(data) != 0 {
			t.Errorf("Expected 0 bytes written, got %d", len(data))
		}
	})
}
