package tracker_test

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/namvu9/bitsy/pkg/bencode"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/tracker"
)

func CreateAnnounceList(tiers ...[]string) bencode.List {
	var out bencode.List
	for _, tier := range tiers {
		var tierList bencode.List

		for _, url := range tier {
			tierList = append(tierList, bencode.Bytes(url))
		}

		out = append(out, tierList)
	}

	return out
}

func LoadMockTorrent(t *testing.T) *btorrent.Torrent {
	t.Helper()
	path := "../testdata/torrents/mock.torrent"

	flag := true
	if flag {
		torrent := btorrent.New()

		torrent.Dict().SetStringKey("announce-list", CreateAnnounceList(
			[]string{
				"udp://localhost:8888/announce",
				"udp://localhost:8888/announce",
				"udp://localhost:8888/announce",
				"udp://localhost:8888/announce",
				"udp://localhost:8888/announce",
			},
			[]string{"udp://awesometracker:9090/announce"},
			[]string{"udp://awesometracker:9090/announce"},
			[]string{"udp://awesometracker:9090/announce"},
			[]string{"udp://awesometracker:9090/announce"},
			[]string{"udp://awesometracker:9090/announce"},
		))

		err := btorrent.Save(path, torrent)
		if err != nil {
			t.Fatal(err)
		}
	}

	torr, err := btorrent.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	return torr
}

type MockTracker struct {
	t           *testing.T
	connections map[uint64]bool
	conn        net.PacketConn

	stopCh chan interface{}
}

func (tr *MockTracker) Stop() {
	tr.stopCh <- nil
}

func (tr *MockTracker) Serve(data []byte, addr net.Addr) {
	connID := binary.BigEndian.Uint64(data[:8])
	if _, ok := tr.connections[connID]; !ok {
		tr.t.Fatal("UNRECOGNIZED CONNID")
	}

	var (
		action = binary.BigEndian.Uint32(data[8:12])
		txID   = binary.BigEndian.Uint32(data[12:16])
	)

	switch action {
	case tracker.CONNECT:
		var buf bytes.Buffer

		connID := uint64(rand.Uint32())
		resp := tracker.ConnMessage{
			Action: tracker.CONNECT,
			TxID:   txID,
			ConnID: connID,
		}

		tr.connections[connID] = true
		err := binary.Write(&buf, binary.BigEndian, resp)
		if err != nil {
			tr.t.Fatal(err)
		}

		tr.conn.WriteTo(buf.Bytes(), addr)
	case tracker.ANNOUNCE:
		resp := tracker.Response{
			Action:    tracker.ANNOUNCE,
			TxID:      txID,
			Interval:  123123,
			NLeechers: 3,
			NSeeders:  1,
			Peers: []tracker.PeerInfo{
				{
					IP:   net.IPv4(192, 168, 0, 1),
					Port: 8080,
				},
				{
					IP:   net.IPv4(192, 168, 0, 2),
					Port: 8888,
				},
			},
		}

		tr.conn.WriteTo(resp.Bytes(), addr)
	}
}

func (tr *MockTracker) Listen(t *testing.T, nReq int) {
	t.Helper()

	tr.t = t
	tr.connections = map[uint64]bool{
		tracker.UDP_PROTOCOL_ID: true,
	}

	conn, err := net.ListenPacket("udp", ":8888")
	if err != nil {
		t.Fatal(err)
	}

	conn.SetDeadline(time.Now().Add(2 * time.Second))
	tr.conn = conn

	buf := make([]byte, 1024)
	for {
		select {
		case <-tr.stopCh:
			conn.Close()
			return
		default:
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				conn.Close()
				return
			}

			tr.Serve(buf[:n], addr)
			buf = make([]byte, 1024)
		}
	}
}

func NewMockTracker() *MockTracker {
	return &MockTracker{
		stopCh: make(chan interface{}, 2),
	}
}

func TestTracker(t *testing.T) {
	server := NewMockTracker()
	defer server.Stop()
	go server.Listen(t, 100)

	t.Run("UDPAnnounce", func(t *testing.T) {

		url, err := url.Parse("udp://localhost:8888/announce")
		if err != nil {
			t.Fatal(err)
		}

		var hash, peerID [20]byte

		tr := tracker.UDPTracker{URL: url}
		if !tr.ShouldAnnounce() {
			t.Error("ShouldAnnounce should return true before first announce")
		}

		res, err := tr.Announce(tracker.Request{
			Hash:       hash,
			PeerID:     peerID,
			Downloaded: 0,
			Uploaded:   1,
			Port:       6999,
		})

		if err != nil {
			t.Error(err)
		}

		if got, want := res.Interval, 123123; got != uint32(want) {
			t.Errorf("Interval, want %d got %d", want, got)
		}

		if got, want := res.NLeechers, 3; got != uint32(want) {
			t.Errorf("Interval, want %d got %d", want, got)
		}

		if got, want := res.NSeeders, 1; got != uint32(want) {
			t.Errorf("Interval, want %d got %d", want, got)
		}

		if got, want := len(res.Peers), 2; got != want {
			t.Errorf("Want %d peer, got %d", want, got)
		}

		if tr.ShouldAnnounce() {
			t.Error("ShouldAnnounce should return false")
		}
	})

	t.Run("TrackerGroup", func(t *testing.T) {
		var hash, peerID [20]byte
		hash[0] = 19
		peerID[0] = 20
		tr := tracker.New(*LoadMockTorrent(t), 6881, peerID)

		res := tr.Announce(tracker.Request{
			Hash:   hash,
			PeerID: peerID,
		})

		// 2 peers x 5 trackers
		if got := len(res); got != 10 {
			t.Errorf("Expected %d, got %d", 3, got)
		}
	})

}
