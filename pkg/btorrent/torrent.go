package btorrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"path"
	"regexp"

	"github.com/namvu9/bencode"
)

// Torrent contains metadata for one or more files and wraps
// a bencoded dictionary.
type Torrent struct {
	dict *bencode.Dictionary
	files []File
}

type Pieces []struct {
	index int
	piece []byte
}

// VerifyInfoDict verifies that the SHA-1 hash of the
// torrent's info dictionary matches the torrent's info hash
func (t *Torrent) VerifyInfoDict() bool {
	info, ok := t.Info()
	if !ok {
		return false
	}

	data, err := bencode.Marshal(info)
	if err != nil {
		return false
	}

	refHash := t.InfoHash()
	hash := sha1.Sum(data)

	return bytes.Equal(refHash[:], hash[:])
}

// VerifyPiece returns true if the piece's SHA-1 hash equals
// the SHA-1 hash of the torrent piece at index i
func (t *Torrent) VerifyPiece(i int, piece []byte) bool {
	var (
		refHash = t.Pieces()[i]
		hash    = sha1.Sum(piece)
	)

	return bytes.Equal(hash[:], refHash)
}

func (t *Torrent) Bytes() []byte {
	data, err := bencode.Marshal(t.Dict())
	if err != nil {
		return []byte{}
	}

	return data
}

// Dict returns the torrent's underlying bencoded dictionary
func (t *Torrent) Dict() *bencode.Dictionary {
	return t.dict
}

func (t *Torrent) PieceLength() Size {
	info, ok := t.Info()
	if !ok {
		panic("torrent has no info dict")
	}

	pieceLength, ok := info.GetInteger("piece length")

	return Size(pieceLength)
}

// Pieces returns the hashes of the pieces that constitute
// the data identified by the torrent. Each piece is a
// 20-byte SHA-1 hash of a block of data defined by the
// value of the torrent's "piece length" field.
func (t *Torrent) Pieces() [][]byte {
	info, ok := t.Info()
	if !ok {
		return [][]byte{}
	}

	piecesBytes, ok := info.GetBytes("pieces")
	if !ok {
		return [][]byte{}
	}

	if len(piecesBytes)%20 != 0 {
		panic("malformed torrent data: 'pieces' array not multiple of 20")
	}

	// Group pieces according to piece length
	pieces := GroupBytes(piecesBytes, 20) // SHA1 hash
	return pieces
}

// Info returns the torrent's info dictionary and true if
// the dictionary exists, otherwise it returns nil and false
func (t *Torrent) Info() (*bencode.Dictionary, bool) {
	v, ok := t.dict.GetDict("info")
	if !ok {
		return nil, false
	}

	return v, true
}

// Name returns the name of the torrent if it exists.
// Returns the empty string otherwise
func (t *Torrent) Name() string {
	// First try to get the value of the 'name' field from the
	// info dictionary
	info, ok := t.Info()
	if ok {
		name, _ := info.GetString("name")
		return name
	}

	// Try the 'dn' field if the torrent was created from a
	// magnet link
	name, _ := t.dict.GetString("dn")
	return name
}

// InfoHash returns the SHA-1 hash of the bencoded value of the
// torrent's info field. The hash uniquely identifies the
// torrent.
func (t *Torrent) InfoHash() [20]byte {
	var out [20]byte
	b, ok := t.dict.GetBytes("info-hash")
	if ok {
		copy(out[:], b)
		return out
	}

	d, _ := t.Info()
	data, err := bencode.Marshal(d)
	if err != nil {
		return out
	}

	hash := sha1.Sum(data)
	t.dict.SetStringKey("info-hash", bencode.Bytes(hash[:]))
	return hash
}

// HexHash returns the hex-encoded SHA-1 hash of the
// bencoded value of the torrent's info field
func (t *Torrent) HexHash() string {
	hash := t.InfoHash()
	return hex.EncodeToString(hash[:])
}

// AnnounceList returns a list of lists of tracker URLs, as
// defined in BEP-12
func (t *Torrent) AnnounceList() [][]string {
	var out [][]string

	l, ok := t.dict.GetList("announce-list")
	if !ok {
		return out
	}

	for _, tier := range l {
		v, ok := tier.ToList()
		if !ok {
			continue
		}

		var trackers []string

		for _, s := range v {
			trackerURL, _ := s.ToBytes()
			trackers = append(trackers, string(trackerURL))
		}

		out = append(out, trackers)
	}

	return out
}

// TODO: TEST
func GroupBytes(data []byte, n int) [][]byte {
	var out [][]byte

	var group []byte

	for _, b := range data {
		if len(group) == n {
			out = append(out, group)
			group = make([]byte, 0)
		}

		group = append(group, b)
	}

	if len(group) != 0 {
		out = append(out, group)
	}

	return out
}

// Files returns a list of file metadata
func (t *Torrent) Files() []File {
	if len(t.files) > 0 {
		return t.files
	}

	info, ok := t.Info()
	if !ok {
		panic("torrent does not have an info dictionary")
	}

	pieces := t.Pieces()
	files, ok := info.GetList("files")

	// Single-file torrent
	if !ok {
		fileLength, _ := info.GetInteger("length")
		name, _ := info.GetString("name")

		return []File{
			{
				Name:     name,
				Length:   Size(fileLength),
				FullPath: name,
				Pieces:   pieces,
			},
		}
	}

	t.files = t.getFiles(files)

	return t.files
}

func (t *Torrent) getFiles(files bencode.List) []File {
	var out []File
	var pieces = t.Pieces()

	// Multi-file torrent
	sum := 0
	for _, file := range files {
		fDict, ok := file.ToDict()
		if !ok {
			continue
		}

		fileLength, _ := fDict.GetInteger("length")
		offset := sum % int(t.PieceLength())

		// TODO: BUG: Must consider offsets
		tf, overlap := getFileData(offset, *fDict, pieces, *t)
		out = append(out, tf)

		if overlap {
			pieces = pieces[len(tf.Pieces)-1:]
		} else {
			pieces = pieces[len(tf.Pieces):]
		}

		sum += int(fileLength)
	}

	return out
}

// Length returns the sum total size, in bytes, of the
// torrent files.  In the case of a single-file torrent, it
// is equal to the size of that file.
func (t *Torrent) Length() Size {
	var sum Size

	for _, file := range t.Files() {
		sum += Size(file.Length)
	}

	return sum
}

// Load a torrent into memory from either a magnet link or a
// file on disk
func Load(location string) (*Torrent, error) {
	p, err := url.PathUnescape(location)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(p)
	if err != nil {
		return nil, err
	}

	var t *Torrent
	if url.Scheme == "magnet" {
		t, err = LoadMagnetURL(url)
		if err != nil {
			return nil, err
		}
	} else {
		t, err = loadFromFile(location)
		if err != nil {
			return nil, err
		}
	}

	// A zero-value indicates that the torrent does not have
	// an info hash or, possibly, that it is incorrectly
	// encoded and hence was not parsed properly. A torrent
	// cannot be identified without a valid info hash and
	// indicates an error
	if t.InfoHash() == [20]byte{} {
		err := fmt.Errorf("file at %s does not have a valid info hash", location)
		return nil, err
	}

	return t, nil
}

func loadFromFile(path string) (*Torrent, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	d, err := bencode.UnmarshalDict(data)
	if err != nil {
		return nil, err
	}

	return &Torrent{dict: d}, nil
}

// TODO: Validate torrent
func Save(path string, t *Torrent) error {
	data, err := bencode.Marshal(t.Dict())
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path, data, 0755)
	if err != nil {
		return err
	}

	return nil
}

// LoadDir reads all files with the .torrent extension
// located at base directory 'dir'.
func LoadDir(dir string) ([]*Torrent, error) {
	var out []*Torrent
	var errs []error

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		name := file.Name()
		match, err := regexp.MatchString(`.+\.torrent`, name)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if match {
			t, err := Load(path.Join(dir, name))
			if err != nil {
				errs = append(errs, err)
				continue
			}

			out = append(out, t)
		}
	}

	return out, nil
}

// TODO: TEST
func (t Torrent) GetPieceIndex(hash []byte) int {
	for i, piece := range t.Pieces() {
		if bytes.Equal(piece, hash) {
			return i
		}
	}

	return -1
}

// GenFastSet computes a peer's Allowed Fast Set using the
// canonical algorithm defined in BEP-6.
//
// 'ip' is the IPv4 address of the peer
//
// 'k' is the desired size of the set
func (t Torrent) GenFastSet(ip net.IP, k int) []int {
	var out []int
	var infoHash = t.InfoHash()

	var hash []byte
	hash = ip[:3]
	hash = append(hash, infoHash[:]...)

	for len(out) < k {
		newHash := sha1.Sum(hash)
		hash = newHash[:]

		for i := 0; i < 5 && len(out) < k; i++ {
			j := i * 4
			y := binary.BigEndian.Uint32(hash[j : j+4])
			index := int(y) % len(t.Pieces())

			if pieceIn(index, out) {
				continue
			}

			out = append(out, index)
		}
	}

	return out
}

func New() *Torrent {
	return &Torrent{
		dict: &bencode.Dictionary{},
	}
}

func pieceIn(idx int, pieces []int) bool {
	for _, pieceIdx := range pieces {
		if idx == pieceIdx {
			return true
		}
	}

	return false
}
