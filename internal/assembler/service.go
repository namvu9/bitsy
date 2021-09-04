package assembler

import (
	"fmt"
	"io"
	"os"
	"path"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

type Service interface {
	Register(btorrent.Torrent)
	Assemble([20]byte) error
	Init() error
}

type Config struct {
	BaseDir     string
	DownloadDir string
}
type assembler struct {
	torrents    map[[20]byte]btorrent.Torrent
	baseDir     string
	downloadDir string

	written map[string]bool
}

func (a *assembler) Init() error {
	// Read BaseDir and Create a bitfield of pieces

	// Read DownloadDir to check which files have been written

	return nil
}

func NewService(cfg Config) Service {
	return &assembler{
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		torrents:    make(map[[20]byte]btorrent.Torrent),
		written:     make(map[string]bool),
	}
}

func (a *assembler) getTorrent(hash [20]byte) (btorrent.Torrent, bool) {
	t, ok := a.torrents[hash]
	return t, ok
}

func (a *assembler) fileDone(t btorrent.Torrent, i int) bool {
	file := t.Files()[i]

	for _, piece := range file.Pieces {
		pcIdx := t.GetPieceIndex(piece)
		if pcIdx < 0 {
			return false
		}
	}

	return true
}

func (a *assembler) Register(t btorrent.Torrent) {
	a.torrents[t.InfoHash()] = t
}

func (ass *assembler) Assemble(hash [20]byte) error {
	t, ok := ass.torrents[hash]
	if !ok {
		return fmt.Errorf("unknown hash: %x", hash)
	}

	return ass.assembleTorrent(t, ass.downloadDir)
}

func (ass *assembler) assembleTorrent(t btorrent.Torrent, dstDir string) error {
	err := os.MkdirAll(dstDir, 0777)
	if err != nil {
		return err
	}

	offset := 0
	for i, file := range t.Files() {
		if ass.written[file.Name] || !ass.fileDone(t, i) {
			offset += int(file.Length)
			continue
		}

		filePath := path.Join(dstDir, file.Name)
		outFile, err := os.Create(filePath)

		startIndex := offset / int(t.PieceLength())
		localOffset := offset % int(t.PieceLength())

		n, err := ass.assembleFile(t, int(startIndex), uint64(localOffset), uint64(file.Length), outFile)
		if err != nil {
			fmt.Println("ERR ASSEMBLE", err)
			return err
		}

		if n != int(file.Length) {
			return fmt.Errorf("expected file length to be %d but wrote %d", file.Length, n)
		}

		offset += n
		ass.written[file.Name] = true
	}

	return nil
}

func (c *assembler) readPiece(t btorrent.Torrent, index int) (*os.File, error) {
	path := path.Join(c.baseDir, t.HexHash(), fmt.Sprintf("%d.part", index))

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (c *assembler) assembleFile(t btorrent.Torrent, startIndex int, localOffset, fileSize uint64, w io.Writer) (int, error) {
	var totalWritten int

	index := startIndex
	file, err := c.readPiece(t, index)

	if err != nil {
		return 0, err
	}

	var buf []byte
	offset := localOffset
	for uint64(totalWritten) < fileSize {
		if left := fileSize - uint64(totalWritten); left < uint64(t.PieceLength()) {
			buf = make([]byte, left)
		} else {
			buf = make([]byte, t.PieceLength())
		}

		n, err := file.ReadAt(buf, int64(offset))
		if err != nil && err != io.EOF {
			return totalWritten, err
		}

		n, err = w.Write(buf[:n])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		err = file.Close()
		if err != nil {
			return totalWritten, err
		}
		
		// load next piece
		index++
		if index < len(t.Pieces()) {
			file, err = c.readPiece(t, index)
			if err != nil {
				return totalWritten, err
			}
			offset = 0
		}
	}

	return totalWritten, nil
}
