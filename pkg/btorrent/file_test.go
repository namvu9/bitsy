package btorrent_test

import (
	"bytes"
	"testing"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/btorrent"
)

func TestFiles(t *testing.T) {
	var (
		torrent     = btorrent.New()
		pieceLength = bencode.Integer(2)
		pieces      = []byte{
			// Piece 1
			1, 1, 1, 4, 5, 6, 7, 8, 9, 10,
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11,

			// Piece 2
			2, 2, 2, 4, 5, 6, 7, 8, 9, 10,
			4, 3, 4, 5, 6, 7, 8, 9, 10, 11,

			// Piece 3
			3, 3, 3, 4, 5, 6, 7, 8, 9, 10,
			4, 3, 4, 5, 6, 7, 8, 9, 10, 11,

			// Piece 4
			4, 4, 4, 4, 5, 6, 7, 8, 9, 10,
			4, 3, 4, 5, 6, 7, 8, 9, 10, 11,

			// Piece 5
			5, 5, 5, 4, 5, 6, 7, 8, 9, 10,
			4, 3, 4, 5, 6, 7, 8, 9, 10, 11,
		}

		info  bencode.Dictionary
		files bencode.List
	)
	info.SetStringKey("pieces", bencode.Bytes(pieces))
	info.SetStringKey("piece length", pieceLength)
	torrent.Dict().SetStringKey("info", &info)

	for _, file := range []struct {
		length int
	}{
		{length: 3}, // Piece 1-2
		{length: 2}, // Piece 2-3
		{length: 1}, // Piece 3
		{length: 2}, // Piece 4
		{length: 1}, // Piece 5
	} {
		var dict bencode.Dictionary
		dict.SetStringKey("length", bencode.Integer(file.length))
		files = append(files, &dict)
	}

	info.SetStringKey("files", files)

	tFiles := torrent.Files()
	if got := len(tFiles); got != 5 {
		t.Errorf("Before files: Want %d Got %d", 5, got)
	}

	file0 := tFiles[0]
	if got := len(file0.Pieces); got != 2 {
		t.Errorf("File %d: Want %d pieces, got %d", 0, 2, got)
	}
	if want := pieces[:20]; !bytes.Equal(file0.Pieces[0], want) {
		t.Errorf("File %d: Want %v; Got %v", 0, want, file0.Pieces[0])
	}
	if want := pieces[20:40]; !bytes.Equal(file0.Pieces[1], want) {
		t.Errorf("File %d: Want %v; Got %v", 0, want, file0.Pieces[1])
	}

	file1 := tFiles[1]
	if got := len(file1.Pieces); got != 2 {
		t.Errorf("File %d: Want %d pieces, got %d", 1, 2, got)
	}
	if want := pieces[20:40]; !bytes.Equal(file1.Pieces[0], want) {
		t.Errorf("File %d: Want %v; Got %v", 1, want, file1.Pieces[0])
	}
	if want := pieces[40:60]; !bytes.Equal(file1.Pieces[1], want) {
		t.Errorf("File %d: Want %v; Got %v", 1, want, file1.Pieces[1])
	}

	file2 := tFiles[2]
	if got := len(file2.Pieces); got != 1 {
		t.Errorf("File %d: Want %d pieces, got %d", 0, 1, got)
	}
	if want := pieces[40:60]; !bytes.Equal(file2.Pieces[0], want) {
		t.Errorf("File %d: Want %v; Got %v", 2, want, file2.Pieces[0])
	}

	file3 := tFiles[3]
	if got := len(file3.Pieces); got != 1 {
		t.Errorf("File %d: Want %d pieces, got %d", 0, 1, got)
	}
	if !bytes.Equal(file3.Pieces[0], pieces[60:80]) {
		t.Errorf("File %d: Want %v; Got %v", 0, file3.Pieces[0], pieces[60:80])
	}

	file4 := tFiles[4]
	if got := len(file4.Pieces); got != 1 {
		t.Errorf("File %d: Want %d pieces, got %d", 0, 1, got)
	}
	if want := pieces[80:100]; !bytes.Equal(file4.Pieces[0], want) {
		t.Errorf("File %d: Want %v; Got %v", 0, want, file4.Pieces[0])
	}
}
