package btorrent_test

import (
	"testing"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

func TestMangetLink(t *testing.T) {
	magnet := "magnet:?xt=urn:btih:280327A7721470DB508C4B7888159F707F753CBB&dn=Loki.S01E03.WEB.x264-PHOENiX%5BTGx%5D&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=udp%3A%2F%2Fmovies.zsw.ca%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.dler.org%3A6969%2Fannounce&tr=udp%3A%2F%2Fopentracker.i2p.rocks%3A6969%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.0x.tf%3A6969%2Fannounce "

	torrent, err := btorrent.Load(magnet)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(torrent.AnnounceList())
	
}
