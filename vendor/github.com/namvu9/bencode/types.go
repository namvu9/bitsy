package bencode

// Value represents a bencoded value with one of the
// following concrete types: Dictionary, List, Integer, and
// Bytes
type Value interface {
	Type() Type
	Value() interface{}

	ToDict() (*Dictionary, bool)
	ToList() (List, bool)
	ToInteger() (Integer, bool)
	ToBytes() (Bytes, bool)
}

type Type int

const (
	TypeDict Type = iota
	TypeInteger
	TypeList
	TypeBytes
)

func IsDict(v Value) bool {
	return v.Type() == TypeDict
}

func IsList(v Value) bool {
	return v.Type() == TypeList
}

func IsInteger(v Value) bool {
	return v.Type() == TypeInteger
}

func IsBytes(v Value) bool {
	return v.Type() == TypeBytes
}

// Bytes represents a bencoded string of bytes
type Bytes []byte

func (b Bytes) Value() interface{} { return b }
func (b Bytes) Type() Type         { return TypeBytes }

func (b Bytes) ToDict() (*Dictionary, bool) { return nil, false }
func (b Bytes) ToList() (List, bool)        { return List{}, false }
func (b Bytes) ToInteger() (Integer, bool)  { return Integer(0), false }
func (b Bytes) ToBytes() (Bytes, bool)      { return b, true }

// List represents a bencoded list
type List []Value

func (l List) Value() interface{} { return l }
func (l List) Type() Type         { return TypeList }

func (l List) ToDict() (*Dictionary, bool) { return &Dictionary{}, false }
func (l List) ToInteger() (Integer, bool)  { return Integer(0), false }
func (l List) ToBytes() (Bytes, bool)      { return Bytes{}, false }
func (l List) ToList() (List, bool)        { return l, true }

// Integer represents a bencoded integer
type Integer int

func (i Integer) Value() interface{} { return i }
func (i Integer) Type() Type         { return TypeInteger }

func (i Integer) ToBytes() (Bytes, bool)      { return Bytes{}, false }
func (i Integer) ToDict() (*Dictionary, bool) { return &Dictionary{}, false }
func (i Integer) ToList() (List, bool)        { return List{}, false }
func (i Integer) ToInteger() (Integer, bool)  { return i, true }
