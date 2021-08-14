package btorrent

import (
	"fmt"
	"math"
	"path"

	"github.com/namvu9/bencode"
)

// A File represents contains the metadata describing a
// particular file of a torrent
type File struct {
	Name     string
	Length   Size
	FullPath string

	// The SHA-1 hashes of the pieces that constitute the file
	// data. Note that pieces may overlap file boundaries and
	// may contain data from other files. They may
	// nevertheless be useful in determining when a particular
	// file has been downloaded completely.
	Pieces [][]byte
}

func getFileData(f bencode.Value, pieces [][]byte, t Torrent) (File, bool) {
	var (
		fDict, _ = f.ToDict()

		fileLength, _    = fDict.GetInteger("length")
		fPieces, overlap = getPieces(int64(fileLength), pieces, int64(t.PieceLength()))

		segments, _ = fDict.GetList("path")
		p           = getFilePath(segments)
	)

	return File{
		Name:     path.Base(p),
		Length:   Size(fileLength),
		FullPath: p,
		Pieces:   fPieces,
	}, overlap
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

func getFilePath(segments bencode.List) string {
	var p string

	for _, segment := range segments {
		s, _ := segment.ToBytes()
		p = path.Join(p, string(s))
	}

	return p
}

// Size is the size of the file if bytes
type Size uint64

// KiB returns the size of the file in Kibibytes (fs / 1024)
func (fs Size) KiB() float64 {
	return float64(fs) / 1024
}

// MiB returns the size of the file in Mebibytes (fs /
// 1024^2)
func (fs Size) MiB() float64 {
	return float64(fs) / (1024 * 1024)
}

// GiB returns the size of the file in Gibibytes (fs /
// 1024^3)
func (fs Size) GiB() float64 {
	return float64(fs) / (1024 * 1024 * 1024)
}

func (fs Size) String() string {
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
