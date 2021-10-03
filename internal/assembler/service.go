package assembler

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/namvu9/bitsy/internal/pieces"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
)

type Service interface {
	Register(btorrent.Torrent)
	Assemble([20]byte, bits.BitField) error
	Init() error
}

type assembler struct {
	torrents    map[[20]byte]btorrent.Torrent
	baseDir     string
	downloadDir string

	written  map[string]bool
	pieceMgr pieces.Service
}

func (a *assembler) Init() error {
	return nil
}

func (a *assembler) Register(t btorrent.Torrent) {
	a.torrents[t.InfoHash()] = t
}

func (ass *assembler) Assemble(hash [20]byte, pieces bits.BitField) error {
	t, ok := ass.torrents[hash]
	if !ok {
		return fmt.Errorf("unknown hash: %x", hash)
	}

	return ass.assembleTorrent(t, pieces)
}

func (a *assembler) fileDone(t btorrent.Torrent, idx int, pieces bits.BitField) bool {
	file := t.Files()[idx]
	for _, hash := range file.Pieces {
		pieceIdx := t.GetPieceIndex(hash)
		if pieceIdx < 0 {
			return false
		}

		if !pieces.Get(pieceIdx) {
			return false
		}
	}

	return true
}

func (asb *assembler) getFileOffset(t btorrent.Torrent, fileIdx int) int {
	offset := 0
	for idx, file := range t.Files() {
		if idx == fileIdx {
			break
		}

		offset += int(file.Length)
	}

	return offset % int(t.PieceLength())
}

func (asb *assembler) assembleTorrent(t btorrent.Torrent, pieces bits.BitField) error {
	for idx, file := range t.Files() {
		if asb.written[file.Name] || !asb.fileDone(t, idx, pieces) {
			continue
		}

		filePath := path.Join(asb.downloadDir, t.Name(), file.Name)
		outFile, err := os.Create(filePath)
		if err != nil {
			return err
		}

		n, err := asb.assembleFile(t, idx, outFile)
		if err != nil {
			continue
		}

		if n != int(file.Length) {
			err := fmt.Errorf("%s: expected file length to be %d but wrote %d", file.Name, file.Length, n)
			fmt.Println(err)
			continue
		}

		asb.written[file.Name] = true
		fmt.Println("wrote", file.Name)
	}

	return nil
}

func pieceIn(hashes [][]byte, piece []byte) bool {
	hashedPiece := sha1.Sum(piece)
	for _, h := range hashes {
		if bytes.Equal(h, hashedPiece[:]) {
			return true
		}
	}

	return false
}

func (asb *assembler) assembleFile(t btorrent.Torrent, fileIdx int, w io.Writer) (int, error) {
	var (
		totalWritten int
		file         = t.Files()[fileIdx]
		offset       = uint64(asb.getFileOffset(t, fileIdx))
		pieceLen     = uint64(t.PieceLength())
		fileSize     = uint64(file.Length)
	)

	fmt.Printf("File %s %d, first piece idx: %d, last: %d\n", file.Name, fileIdx, t.GetPieceIndex(file.Pieces[0]), t.GetPieceIndex(file.Pieces[len(file.Pieces)-1]))

	for _, hash := range file.Pieces {
		index := t.GetPieceIndex(hash)
		piece, err := asb.pieceMgr.Load(t.InfoHash(), index)
		if err != nil {
			return 0, err
		}

		var left = fileSize - uint64(totalWritten)
		var data []byte
		if left < pieceLen {
			data = piece[offset : offset+left]
		} else {
			data = piece[offset:]
		}

		n, err := w.Write(data)
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		offset = 0
	}

	return totalWritten, nil
}

type Config struct {
	BaseDir     string
	DownloadDir string
	PieceMgr    pieces.Service
}

func NewService(cfg Config) Service {
	return &assembler{
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		torrents:    make(map[[20]byte]btorrent.Torrent),
		written:     make(map[string]bool),
		pieceMgr:    cfg.PieceMgr,
	}
}
