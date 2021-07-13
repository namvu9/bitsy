package bencode_test

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/namvu9/bitsy/src/bencode"
)

func TestDecodeInteger(t *testing.T) {
	for i, test := range []struct {
		input string
		want  bencode.Value
		err   bool
	}{
		{"i42e", bencode.Integer(42), false},
		{"i0e", bencode.Integer(0), false},
		{"i-42e", bencode.Integer(-42), false},
		{"i-0e", bencode.Integer(0), true},
		{"i-ab0e", bencode.Integer(0), true},
	} {
		var got bencode.Value
		err := bencode.Unmarshal([]byte(test.input), &got)
		if test.err && err == nil || !test.err && err != nil {
			t.Errorf("%d: %s", i, err)
		}

		if !test.err && !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: Want %d got %d", i, test.want, got.Value())
		}
	}
}

func TestDecodeBytes(t *testing.T) {
	for i, test := range []struct {
		input string
		want  bencode.Value
		err   bool
	}{
		{
			"3:foo",
			bencode.Bytes([]byte("foo")),
			false,
		},
		{
			"4:foo",
			bencode.Bytes([]byte("foo")),
			true,
		},
		{
			"0:foo",
			bencode.Bytes{},
			false,
		},
	} {
		var got bencode.Value
		err := bencode.Unmarshal([]byte(test.input), &got)

		if test.err && err == nil || !test.err && err != nil {
			t.Errorf("%d: %s", i, err)
		}

		if !test.err && !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: Want %d got %d", i, test.want, got.Value())
		}
	}
}

func TestUnmarshalList(t *testing.T) {
	for i, test := range []struct {
		input string
		want  bencode.Value
		err   bool
	}{
		{
			"l3:loli100ee",
			bencode.List{bencode.Bytes("lol"), bencode.Integer(100)},
			false,
		},
	} {
		var got bencode.Value
		err := bencode.Unmarshal([]byte(test.input), &got)

		if test.err && err == nil || !test.err && err != nil {
			t.Errorf("%d: %s", i, err)
		}

		if !test.err && !reflect.DeepEqual(got, test.want) {
			t.Errorf("%d: Want %d got %d", i, test.want, got.Value())
		}
	}
}

func TestUnmarshalDict(t *testing.T) {
	for i, test := range []struct {
		input  string
		keys   [][]byte
		values []bencode.Value
		err    bool
	}{
		{
			"d3:fooi42e4:dudeli99eee",
			[][]byte{[]byte("foo"), []byte("dude")},
			[]bencode.Value{bencode.Integer(42), bencode.List([]bencode.Value{bencode.Integer(99)})},
			false,
		},
	} {
		var got bencode.Value
		err := bencode.Unmarshal([]byte(test.input), &got)

		if test.err && err == nil || !test.err && err != nil {
			t.Errorf("%d: %s", i, err)
		}

		var want bencode.Dictionary
		for i, key := range test.keys {
			want.Set(key, test.values[i])
		}

		if !test.err && !reflect.DeepEqual(got.Value(), want.Value()) {
			t.Errorf("%d: Want %v got %v", i, want, got)
		}
	}
}

func TestUnmarshalTorrentFile(t *testing.T) {
	data, err := ioutil.ReadFile("../../testdata/test.torrent")
	if err != nil {
		t.Fatal(err)
	}

	var got bencode.Value
	err = bencode.Unmarshal(data, &got)
	if err != nil {
		t.Fatal(err)
	}

	d, ok := got.(*bencode.Dictionary)
	if !ok {
		t.Fatal("BLABLA")
	}

	wantKeys := []string{"announce", "announce-list", "comment", "created by", "creation date", "info", "url-list"}

	for _, keyStr := range wantKeys {
		val, ok := d.Get([]byte(keyStr))
		if !ok {
			t.Errorf("Expected to find key %s", keyStr)
		}

		t.Log(keyStr, val.Type())
	}

	info, _ := d.Get([]byte("info"))
	dInfo, _ := info.(*bencode.Dictionary)

	for _, k := range dInfo.Keys() {
		t.Log(">>>", string(k))
	}
}
