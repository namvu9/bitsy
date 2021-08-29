package data

import (
	"fmt"
	"io"
	"os"
	"path"
)

func (c *Client) assembleTorrent(dstDir string) error {
	t := c.torrent
	err := os.MkdirAll(dstDir, 0777)
	if err != nil {
		return err
	}

	offset := 0
	for i, file := range c.torrent.Files() {
		if c.filesWritten[file.Name] || !c.fileDone(i) {
			offset += int(file.Length)
			continue
		}

		filePath := path.Join(dstDir, file.Name)
		outFile, err := os.Create(filePath)

		startIndex := offset / int(t.PieceLength())
		localOffset := offset % int(t.PieceLength())

		n, err := c.assembleFile(int(startIndex), uint64(localOffset), uint64(file.Length), outFile)
		if err != nil {
			return err
		}

		if n != int(file.Length) {
			return fmt.Errorf("expected file length to be %d but wrote %d", file.Length, n)
		}

		offset += n
		c.filesWritten[file.Name] = true
	}

	return nil
}

func (c *Client) readPiece(index int) (*os.File, error) {
	t := c.torrent
	path := path.Join(c.baseDir, t.HexHash(), fmt.Sprintf("%d.part", index))

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (c *Client) assembleFile(startIndex int, localOffset, fileSize uint64, w io.Writer) (int, error) {
	t := c.torrent
	var totalWritten int

	index := startIndex
	file, err := c.readPiece(index)

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

		// load next piece
		index++
		if index < len(t.Pieces()) {
			file, err = c.readPiece(index)
			if err != nil {
				return totalWritten, err
			}
			offset = 0
		}
	}

	return totalWritten, nil
}
