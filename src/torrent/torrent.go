package torrent

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"path"

	"github.com/namvu9/bitsy/src/bencode"
)

// Quit channel
// A channel only for notifying that to quit

// NewPeers channel
// Incoming connections

//func New()

// Torrent contains metadata for one or more files and wraps
// a bencoded dictionary.
type Torrent struct {
	dict *bencode.Dictionary
}

// VerifyPiece returns true if the piece's SHA-1 hash equals
// the SHA-1 hash of the torrent piece at index i
func (t *Torrent) VerifyPiece(i int, piece []byte) bool {
	refHash := t.Pieces()[i]
	hash := sha1.Sum(piece)

	return bytes.Equal(hash[:], refHash)
}

// Dict returns the torrent's underlying bencoded dictionary
func (t *Torrent) Dict() *bencode.Dictionary {
	return t.dict
}

func (t *Torrent) PieceLength() uint64 {
	info, ok := t.Info()
	if !ok {
		return 0
	}
	pieceLength, ok := info.GetInteger("piece length")

	return uint64(pieceLength)
}

// Pieces returns the s
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
func (t *Torrent) InfoHash() *[20]byte {
	var out = new([20]byte)
	b, ok := t.dict.GetBytes("info-hash")
	if ok {
		copy(out[:], b)
		return out
	}

	d, _ := t.Info()
	data, err := bencode.Marshal(d)
	if err != nil {
		return nil
	}

	hash := sha1.Sum(data)
	t.dict.SetStringKey("info-hash", bencode.Bytes(hash[:]))
	return &hash
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
	Length uint64
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

	piecesBytes, ok := info.GetBytes("pieces")
	if !ok {
		return out, fmt.Errorf("field 'pieces' does not exist or is incorrectly encoded")
	}

	// Group pieces according to piece length
	pieces := GroupBytes(piecesBytes, 20) // SHA1 hash

	files, ok := info.GetList("files")
	if !ok {
		fileLength, _ := info.GetInteger("length")
		name, _ := info.GetString("name")

		out = append(out, File{Name: name, Length: uint64(fileLength), Path: name, Pieces: pieces})
	}

	for _, file := range files {
		fDict, _ := file.ToDict()
		fileLength, _ := fDict.GetInteger("length")
		paths, _ := fDict.GetList("path")

		var strPath string

		for _, p := range paths {
			s, _ := p.ToBytes()
			strPath += path.Join(string(s))
		}

		name, _ := paths[len(paths)-1].ToBytes()
		fPieces, overlap := getPieces(int64(fileLength), pieces, int64(pieceLength))
		tf := File{
			Name:   string(name),
			Length: uint64(fileLength),
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
