package tracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
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
	Stat() TrackerStat
}

type TrackerGroup struct {
	btorrent.Torrent

	outCh chan Response
	inCh  chan map[string]interface{}

	trackers []Tracker
	peerID   [20]byte

	Port uint16

	lastResponse map[string]PeerInfo
}

type TrackerStat struct {
	Url          *url.URL
	Peers        peerList
	Seeders      int
	Leechers     int
	NextAnnounce time.Time
	Err          error
}

type peerList []net.Addr

func (tg *TrackerGroup) Stat() []TrackerStat {
	var out []TrackerStat
	for _, tracker := range tg.trackers {
		out = append(out, tracker.Stat())
	}

	return out
}

func (tg *TrackerGroup) Len() int {
	return len(tg.trackers)
}

func (tg *TrackerGroup) Scrape() []TrackerStat {
	var out []TrackerStat
	return out
}

func AnnounceS(ctx context.Context, urls []string, req Request) chan TrackerStat {
	return NewGroup(urls).AnnounceS(ctx, req)
}

// AnnounceS is like Announce, but returns a stream instead
// of a slice
func (tg *TrackerGroup) AnnounceS(ctx context.Context, req Request) chan TrackerStat {
	out := make(chan TrackerStat, len(tg.trackers))

	go func() {
		var wg sync.WaitGroup

		for _, tracker := range tg.trackers {
			wg.Add(1)
			go func(tracker Tracker, wg *sync.WaitGroup) {
				defer wg.Done()

				if !tracker.ShouldAnnounce() {
					return
				}

				_, err := tracker.Announce(req)
				if err != nil {
					return
				}

				out <- tracker.Stat()
			}(tracker, &wg)
		}

		wg.Wait()
		close(out)
	}()

	return out
}

func (tg *TrackerGroup) Announce(req Request) map[string]PeerInfo {
	out := make(map[string]PeerInfo)

	var wg sync.WaitGroup
	var resCh = make(chan *Response, 30)

	for _, tracker := range tg.trackers {
		wg.Add(1)
		go func(tracker Tracker) {
			if !tracker.ShouldAnnounce() {
				wg.Done()
				return
			}

			res, err := tracker.Announce(req)
			if err != nil {
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

	wg.Wait()
	close(resCh)

	var count int
	for res := range resCh {
		for _, peer := range res.Peers {
			count++
			out[string(peer.IP.To16())] = peer
		}
	}

	if len(out) == 0 {
		return tg.lastResponse
	}

	if len(out) > len(tg.lastResponse) {
		tg.lastResponse = out
	}

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

func (req Request) String() string {
	var sb strings.Builder

	fmt.Fprintln(&sb, "UDP Announce Request")
	fmt.Fprintf(&sb, "%x\n", req.Hash)
	fmt.Fprintf(&sb, "PeerID: %s\n", req.PeerID)
	fmt.Fprintf(&sb, "Port: %d\n", req.Port)
	fmt.Fprintf(&sb, "Downloaded: %d\n", req.Downloaded)

	return sb.String()
}

type UDPRequest struct {
	ConnID uint64
	Action uint32
	TxID   uint32

	Request
}

func NewRequest(hash [20]byte, port uint16, peerID [20]byte, downloaded uint64) Request {
	return Request{
		Want:       -1,
		PeerID:     peerID,
		Hash:       hash,
		Port:       port,
		Downloaded: downloaded,
	}
}

func NewGroup(addrs []string) *TrackerGroup {
	var trackers []Tracker

	for _, addr := range addrs {
		url, err := url.Parse(addr)
		if err != nil {
			continue
		}

		if url.Scheme == "udp" {
			trackers = append(trackers, NewUDPTracker(url))
		}
	}

	return &TrackerGroup{trackers: trackers}
}
