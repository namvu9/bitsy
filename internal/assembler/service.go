package assembler

import (
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

		filePath := path.Join(asb.downloadDir, t.Name(),file.Name)
		outFile, err := os.Create(filePath)

		n, err := asb.assembleFile(t, idx, outFile)
		if err != nil {
			fmt.Println("ERR ASSEMBLE", err)
			continue
		}

		if n != int(file.Length) {
			return fmt.Errorf("expected file length to be %d but wrote %d", file.Length, n)
		}

		asb.written[file.Name] = true
	}

	return nil
}

func (asb *assembler) assembleFile(t btorrent.Torrent, fileIdx int, w io.Writer) (int, error) {
	var (
		totalWritten int
		file         = t.Files()[fileIdx]
		index        = t.GetPieceIndex(file.Pieces[0])
		offset       = uint64(asb.getFileOffset(t, fileIdx))
		pieceLen     = uint64(t.PieceLength())
		fileSize     = uint64(file.Length)
	)

	for uint64(totalWritten) < fileSize {
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

		index++
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
