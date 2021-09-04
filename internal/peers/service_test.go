package peers

import (
	"testing"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

func TestAddPeer(t *testing.T) {
	var p peer.Peer
	p.ID = make([]byte, 20)
	copy(p.ID[8:], []byte{1, 3, 3, 7, 1, 3, 3, 7})
	var torrent = btorrent.New()
	torrent.Dict().SetStringKey("info-hash", bencode.Bytes{})
	s := &peerService{
		trackers: make(map[InfoHash][]*tracker.TrackerGroup),
		peers:    make(map[[20]byte]map[string]*peer.Peer),
	}

	s.Register(*torrent)
	s.Add([20]byte{}, &p)

	if got := len(s.peers[[20]byte{}]); got != 1 {
		t.Errorf("want %d got %d", 1, got)
	}

	s.Remove([20]byte{}, &p)

	if got := len(s.peers[[20]byte{}]); got != 0 {
		t.Errorf("want %d got %d", 0, got)
	}
}
