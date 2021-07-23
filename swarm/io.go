package swarm

import (
	"fmt"
	"io"
	"os"
	"path"
)

func (s *Swarm) AssembleTorrent(dstDir string) error {
	err := os.MkdirAll(dstDir, 0777)
	if err != nil {
		return err
	}

	files, err := s.Files()
	if err != nil {
		return err
	}

	offset := 0
	for _, file := range files {
		filePath := path.Join(dstDir, file.Name)
		outFile, err := os.Create(filePath)

		startIndex := offset / int(s.PieceLength())
		localOffset := offset % int(s.PieceLength())

		n, err := s.assembleFile(int(startIndex), uint64(localOffset), file.Length, outFile)
		if err != nil {
			return err
		}

		if n != int(file.Length) {
			return fmt.Errorf("expected file length to be %d but wrote %d\n", file.Length, n)
		}

		offset += n
	}

	return nil
}

func (s *Swarm) readPiece(index int) (*os.File, error) {
	path := path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", index))

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (s *Swarm) assembleFile(startIndex int, localOffset, fileSize uint64, w io.Writer) (int, error) {
	var totalWritten int

	index := startIndex
	file, err := s.readPiece(index)

	if err != nil {
		return 0, err
	}

	var buf []byte
	offset := localOffset
	for uint64(totalWritten) < fileSize {
		if left := fileSize - uint64(totalWritten); left < s.PieceLength() {
			buf = make([]byte, left)
		} else {
			buf = make([]byte, s.PieceLength())
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
		if index < len(s.Pieces()) {
			file, err = s.readPiece(index)
			if err != nil {
				return totalWritten, err
			}
			offset = 0
		}
	}

	return totalWritten, nil
}
