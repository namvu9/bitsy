package btorrent_test

import (
	"bytes"
	"testing"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/btorrent"
)

func TestPieces(t *testing.T) {
	pieces := []byte{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
		3, 2, 3, 4, 5, 6, 7, 8, 9, 10,
		4, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	}
	var info bencode.Dictionary
	info.SetStringKey("pieces", bencode.Bytes(pieces))
	torrent := btorrent.New()
	torrent.Dict().SetStringKey("info", &info)

	res := torrent.Pieces()

	if len(res) != 2 {
		t.Errorf("Want %d; Got %d", 2, len(res))
	}

	if !bytes.Equal(
		[]byte{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
		},
		res[0],
	) {
		t.Errorf("Want %v got %v", pieces[:20], res[0])
	}

	if !bytes.Equal(
		[]byte{
			3, 2, 3, 4, 5, 6, 7, 8, 9, 10,
			4, 3, 4, 5, 6, 7, 8, 9, 10, 11,
		},
		res[1],
	) {
		t.Errorf("Want %v got %v", pieces[20:40], res[1])
	}
}

//func TestVerifyInfoDict()
//func TestVerifyPiece()
//func TestGroupBytes()
//func TestFiles()
//func TestLoadFromFile()
//func TestGetPieceIndex()
//func TestGenFastSet()

func TestMangetLink(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:280327A7721470DB508C4B7888159F707F753CBB&dn=Loki.S01E03.WEB.x264-PHOENiX%5BTGx%5D&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=udp%3A%2F%2Fmovies.zsw.ca%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.dler.org%3A6969%2Fannounce&tr=udp%3A%2F%2Fopentracker.i2p.rocks%3A6969%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.0x.tf%3A6969%2Fannounce "

	torrent, err := btorrent.Load(magnet)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(torrent.AnnounceList())
}

func TestTorrentFiles(t *testing.T) {
	// PieceLength
	// FileSize
	// Hashes
}
