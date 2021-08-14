package btorrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"io/ioutil"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/internal/errors"
)

// Torrent contains metadata for one or more files and wraps
// a bencoded dictionary.
type Torrent struct {
	dict *bencode.Dictionary
}

type Pieces []struct {
	index int
	piece []byte
}

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

func (t *Torrent) Stat() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["name"] = t.Name()
	stats["pieceLength"] = t.PieceLength()
	stats["infoHash"] = t.HexHash()
	stats["trackers"] = t.AnnounceList()

	var fileStats []map[string]interface{}
	for _, file := range t.Files() {
		fileStat := make(map[string]interface{})
		fileStat["name"] = file.Name
		fileStat["length"] = file.Length
		fileStat["path"] = file.FullPath

		fileStats = append(fileStats, fileStat)
	}

	stats["files"] = fileStats

	return stats
}

func (t *Torrent) Verify(pieces Pieces) bool {
	verifiedPieces := make(map[int]bool)

	size := 0

	for _, p := range pieces {
		if !t.VerifyPiece(p.index, p.piece) {
			return false
		}

		size += len(p.piece)

		verifiedPieces[p.index] = true
	}

	for index := range t.Pieces() {
		if _, ok := verifiedPieces[index]; !ok {
			return false
		}
	}

	if t.Length() != Size(size) {
		return false
	}

	return true
}

// TODO: TEST
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
		return 0
	}
	pieceLength, ok := info.GetInteger("piece length")

	return Size(pieceLength)
}

type PieceHash []byte

// TODO: TEST
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

// Announce returns the url of the torrent's tracker
func (t *Torrent) Announce() string {
	s, ok := t.dict.GetString("announce")
	if !ok {
		return ""
	}

	return s
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
	var out []File

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

		out = append(out, File{Name: name, Length: Size(fileLength), FullPath: name, Pieces: pieces})
	}

	// Multi-file torrent
	for _, file := range files {
		tf, overlap := getFileData(file, *t)
		out = append(out, tf)

		if overlap {
			pieces = pieces[len(tf.Pieces)-1:]
		} else {
			pieces = pieces[len(tf.Pieces):]
		}
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
	var op errors.Op = "torrent.Load"
	p, err := url.PathUnescape(location)
	if err != nil {
		return nil, err
	}

	url, err := url.Parse(p)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	var t *Torrent
	if url.Scheme == "magnet" {
		t, err = LoadMagnetURL(url)
		if err != nil {
			return nil, errors.Wrap(err, op)
		}
	} else {
		t, err = loadFromFile(location)
		if err != nil {
			return nil, errors.Wrap(err, op)
		}
	}

	// A zero-value indicates that the torrent does not have
	// an info hash or, possibly, that it is incorrectly
	// encoded and hence was not parsed properly. A torrent
	// cannot be identified without a valid info hash and
	// indicates an error
	if t.InfoHash() == [20]byte{} {
		err := errors.Newf("file at %s does not have a valid info hash", location)
		return nil, errors.Wrap(err, op)
	}

	return t, nil
}

func LoadMagnetURL(u *url.URL) (*Torrent, error) {
	var dict bencode.Dictionary

	queries := u.Query()

	var trackerTier bencode.List
	trs, ok := queries["tr"]
	if !ok || len(trs) == 0 {
		// DHT is currently not supported
		return nil, errors.New("magnet link must specify at least 1 tracker")
	}

	for _, tracker := range queries["tr"] {
		trackerTier = append(trackerTier, bencode.Bytes(tracker))
	}

	dict.SetStringKey("announce", trackerTier[0])
	dict.SetStringKey("announce-list", bencode.List{trackerTier})

	var (
		xt       = strings.Split(queries.Get("xt"), ":")
		protocol = xt[1]
		urn      = xt[2]
	)
	if protocol != "btih" {
		return nil, errors.New("Only BitTorrent URNs (btih) are supported")
	}
	hash, err := hex.DecodeString(urn)
	if err != nil {
		return nil, err
	}

	dict.SetStringKey("info-hash", bencode.Bytes(hash))
	dict.SetStringKey("dn", bencode.Bytes(queries.Get("dn")))

	return &Torrent{
		dict: &dict,
	}, nil
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
	var op errors.Op = "torrent.Save"

	data, err := bencode.Marshal(t.Dict())
	if err != nil {
		return errors.Wrap(err, op)
	}

	err = ioutil.WriteFile(path, data, 0755)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
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

func (t Torrent) GetPieceIndex(hash []byte) int {
	for i, piece := range t.Pieces() {
		if bytes.Equal(piece, hash) {
			return i
		}
	}

	return -1
}

func New() *Torrent {
	return &Torrent{
		&bencode.Dictionary{},
	}
}
