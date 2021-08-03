package tracker

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/url"
	"sync"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/rs/zerolog/log"
)

// UDP Tracker Actions
const (
	CONNECT  uint32 = 0
	ANNOUNCE uint32 = 1
	SCRAPE   uint32 = 2
	ERROR    uint32 = 3
)

type Tracker interface {
	Announce(Request) (*Response, error)
	ShouldAnnounce() bool
	Err() error
	Stat() map[string]interface{}
}

type TrackerGroup struct {
	btorrent.Torrent

	outCh chan Response
	inCh  chan map[string]interface{}

	trackers [][]Tracker
	peerID   [20]byte

	Port uint16
}

func (tg *TrackerGroup) Stat() []interface{} {
	var trackers []interface{}
	for _, tier := range tg.trackers {
		for _, tracker := range tier {
			trackers = append(trackers, tracker.Stat())
		}
	}

	return trackers
}

func (tg *TrackerGroup) Announce(req Request) []PeerInfo {
	var out []PeerInfo

	var nSuccess int
	var wg sync.WaitGroup
	var resCh = make(chan *Response, 30)

	for _, tier := range tg.trackers {
		for _, tracker := range tier {
			wg.Add(1)
			go func(tracker Tracker) {
				if !tracker.ShouldAnnounce() {
					wg.Done()
					return
				}

				res, err := tracker.Announce(req)
				if err != nil {
					log.Err(err).Msg("Announce failed")
					wg.Done()
					return
				}

				select {
				case resCh <- res:
				default:
				}
				wg.Done()
			}(tracker)
		}
	}

	wg.Wait()
	close(resCh)
	
	for res := range resCh {
		out = append(out, res.Peers...)
		nSuccess++
	}

	if nSuccess >= 5 {
		return out
	}

	log.Printf("Discovered %d peers\n", len(out))
	return out
}

type PeerInfo struct {
	IP   net.IP
	Port uint16
}

type Response struct {
	Action    uint32
	TxID      uint32
	Interval  uint32
	NLeechers uint32
	NSeeders  uint32
	Peers     []PeerInfo
}

func (r *Response) Bytes() []byte {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, r.Action)
	binary.Write(&buf, binary.BigEndian, r.TxID)
	binary.Write(&buf, binary.BigEndian, r.Interval)
	binary.Write(&buf, binary.BigEndian, r.NLeechers)
	binary.Write(&buf, binary.BigEndian, r.NSeeders)

	for _, peer := range r.Peers {
		buf.Write(peer.IP.To4())
		binary.Write(&buf, binary.BigEndian, peer.Port)
	}

	return buf.Bytes()
}

type Request struct {
	Hash   [20]byte
	PeerID [20]byte

	Downloaded uint64
	Left       uint64
	Uploaded   uint64
	Event      uint32 // 0: None
	IP         uint32 // Default: 0
	Key        uint32
	Want       int32 // Default: -1
	Port       uint16
}

type UDPRequest struct {
	ConnID uint64
	Action uint32
	TxID   uint32

	Request
}

func NewRequest(hash [20]byte, port uint16, peerID [20]byte) Request {
	return Request{
		Want:   -1,
		PeerID: peerID,
		Hash:   hash,
		Port:   port,
	}
}

func New(t btorrent.Torrent, port uint16, peerID [20]byte) *TrackerGroup {
	var trackerTiers [][]Tracker

	for _, tier := range t.AnnounceList() {
		var trackers []Tracker
		for _, trackerURL := range tier {
			url, err := url.Parse(trackerURL)
			if err != nil {
				continue
			}

			if url.Scheme == "udp" {
				trackers = append(trackers, &UDPTracker{
					URL: url,
				})
			}
		}

		if len(trackers) > 0 {
			trackerTiers = append(trackerTiers, trackers)
		}
	}

	return &TrackerGroup{
		Torrent:  t,
		trackers: trackerTiers,
		Port:     port,
		peerID:   peerID,
	}
}
