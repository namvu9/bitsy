package btorrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
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
	files, _ := t.Files()
	for _, file := range files {
		fileStat := make(map[string]interface{})
		fileStat["name"] = file.Name
		fileStat["length"] = file.Length
		fileStat["path"] = file.Path

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

	if t.Length() != FileSize(size) {
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
	data, _ := bencode.Marshal(t.Dict())

	return data
}

// Dict returns the torrent's underlying bencoded dictionary
func (t *Torrent) Dict() *bencode.Dictionary {
	return t.dict
}

func (t *Torrent) Description() string {
	info, ok := t.Info()
	if !ok {
		return ""
	}
	s, _ := info.GetString("description")
	return s
}

func (t *Torrent) PieceLength() FileSize {
	info, ok := t.Info()
	if !ok {
		return 0
	}
	pieceLength, ok := info.GetInteger("piece length")

	return FileSize(pieceLength)
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

type File struct {
	Name   string
	Length FileSize
	Path   string

	// The SHA-1 hashes of the pieces that constitute the file
	// data. Note that pieces may overlap file boundaries and
	// may contain data from other files. They may
	// nevertheless be useful in determining when a file has
	// been downloaded completely.
	Pieces [][]byte
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

// TODO: TEST
func getPieces(fileLength int64, pieces [][]byte, pieceLength int64) ([][]byte, bool) {
	var out [][]byte
	overlap := fileLength%pieceLength != 0

	// Off-by-1 error?
	nPieces := math.Ceil(float64(fileLength) / float64(pieceLength))

	for i, piece := range pieces {
		if i == int(nPieces) {
			return out, overlap
		}

		out = append(out, piece)
	}

	return out, overlap
}

// Files returns a list of file metadata as bencoded
// dictionaries
func (t *Torrent) Files() ([]File, error) {
	var out []File

	info, ok := t.Info()
	if !ok {
		return out, fmt.Errorf("info dictionary does not exist or is incorrectly encoded")
	}

	pieceLength, ok := info.GetInteger("piece length")
	if !ok {
		return out, fmt.Errorf("field piece length does not exist or is incorrectly encoded")
	}

	pieces := t.Pieces()
	files, ok := info.GetList("files")
	if !ok {
		fileLength, _ := info.GetInteger("length")
		name, _ := info.GetString("name")

		out = append(out, File{Name: name, Length: FileSize(fileLength), Path: name, Pieces: pieces})
	}

	for _, file := range files {
		var (
			fDict, _      = file.ToDict()
			fileLength, _ = fDict.GetInteger("length")
			paths, _      = fDict.GetList("path")
			segments      []string
		)

		for _, segment := range paths {
			s, _ := segment.ToBytes()
			segments = append(segments, string(s))
		}

		var (
			strPath          = path.Join(segments...)
			name, _          = paths[len(paths)-1].ToBytes()
			fPieces, overlap = getPieces(int64(fileLength), pieces, int64(pieceLength))
		)

		tf := File{
			Name:   string(name),
			Length: FileSize(fileLength),
			Path:   strPath,
			Pieces: fPieces,
		}

		out = append(out, tf)

		if overlap {
			pieces = pieces[len(tf.Pieces)-1:]
		} else {
			pieces = pieces[len(tf.Pieces):]
		}
	}

	return out, nil
}

// Length returns the sum total size, in bytes, of the
// torrent files.  In the case of a single-file torrent, it
// is equal to the size of that file.
func (t *Torrent) Length() FileSize {
	var sum FileSize
	files, err := t.Files()
	if err != nil {
		return 0
	}

	for _, file := range files {
		sum += FileSize(file.Length)
	}

	return sum
}

// FileSize is the size of the file if bytes
type FileSize uint64

// KiB returns the size of the file in Kibibytes (fs / 1024)
func (fs FileSize) KiB() float64 {
	return float64(fs) / 1024
}

// MiB returns the size of the file in Mebibytes (fs /
// 1024^2)
func (fs FileSize) MiB() float64 {
	return float64(fs) / (1024 * 1024)
}

// GiB returns the size of the file in Gibibytes (fs /
// 1024^3)
func (fs FileSize) GiB() float64 {
	return float64(fs) / (1024 * 1024 * 1024)
}

func (fs FileSize) String() string {
	if fs < 1024 {
		return fmt.Sprintf("%d B", fs)
	}

	if fs < 1024*1024 {
		return fmt.Sprintf("%.2f KiB", fs.KiB())
	}

	if fs < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MiB", fs.MiB())
	}

	return fmt.Sprintf("%.2f GiB", fs.GiB())
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
	var op errors.Op = "torrent.loadFromMagnetLink"
	var dict bencode.Dictionary

	queries := u.Query()

	var trackerTier bencode.List
	trs, ok := queries["tr"]
	if !ok || len(trs) == 0 {
		// DHT is currently not supported
		err := errors.New("magnet link must specify at least 1 tracker")
		return nil, errors.Wrap(err, op, errors.BadArgument)
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
		err := errors.New("Only BitTorrent URNs (btih) are supported")
		return nil, errors.Wrap(err, op, errors.BadArgument)
	}
	hash, err := hex.DecodeString(urn)
	if err != nil {
		return nil, errors.Wrap(err, op)
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

type Errors []error

func (e Errors) Error() string {
	var sb strings.Builder
	for _, err := range e {
		sb.WriteString(err.Error())
		sb.WriteString(", ")
	}

	return sb.String()
}

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

func New() *Torrent {
	return &Torrent{
		&bencode.Dictionary{},
	}
}

