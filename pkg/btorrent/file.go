package btorrent

import (
	"path"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/btorrent/size"
)

// A File represents contains the metadata describing a
// particular file of a torrent
type File struct {
	Name     string
	Length   size.Size
	FullPath string

	// The SHA-1 hashes of the pieces that constitute the file
	// data. Note that pieces may overlap file boundaries and
	// may contain data from other files. They may
	// nevertheless be useful in determining when a particular
	// file has been downloaded completely.
	Pieces [][]byte
}

func getFileData(offset int, file bencode.Dictionary, pieces [][]byte, t Torrent) (File, bool) {
	var (
		fileLength, _    = file.GetInteger("length")
		fPieces, overlap = getPieces(offset, int64(fileLength), pieces, int64(t.PieceLength()))

		segments, _ = file.GetList("path")
		p           = getFilePath(segments)
	)

	return File{
		Name:     path.Base(p),
		Length:   size.Size(fileLength),
		FullPath: p,
		Pieces:   fPieces,
	}, overlap
}

// TODO: TEST
func getPieces(offset int, fileLength int64, pieces [][]byte, pieceLength int64) ([][]byte, bool) {
	var (
		out        [][]byte
		fileLeft   = int(fileLength)
		offsetLeft = offset
		idx        int
	)
	for {
		pieceLeft := int(pieceLength) - offsetLeft
		offsetLeft = 0

		fileLeft -= pieceLeft
		out = append(out, pieces[idx])

		if fileLeft <= 0 {
			return out, fileLeft != 0
		}

		idx++
	}
}

// TODO: TEST
func getFilePath(segments bencode.List) string {
	var p string

	for _, segment := range segments {
		s, _ := segment.ToBytes()
		p = path.Join(p, string(s))
	}

	return p
}

