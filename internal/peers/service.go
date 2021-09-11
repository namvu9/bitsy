package peers

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

type OrderByFunc func(*peer.Peer, *peer.Peer) bool
type GetRequest struct {
	Limit   int
	OrderBy OrderByFunc
	Filter  func(*peer.Peer) bool
}

type GetResponse struct {
	Peers []*peer.Peer
}

type Stat struct {
	Downloaded uint64
}

type Service interface {
	Get(InfoHash, GetRequest) GetResponse
	Add(InfoHash, *peer.Peer)
	Remove(InfoHash, *peer.Peer)

	Register(btorrent.Torrent)
	Unregister(InfoHash)

	// Return info for peers that aren't currently in the
	// swarm
	Discover(InfoHash, int, Stat) []tracker.PeerInfo
	Swarms() map[InfoHash]SwarmStat
}
type InfoHash = [20]byte

type peerService struct {
	emitter  chan interface{}
	trackers map[InfoHash][]*tracker.TrackerGroup
	peers    map[InfoHash]map[string]*peer.Peer

	port   uint16
	peerID [20]byte
	lock   sync.Mutex
}

func (svc *peerService) getPeersSorted(hash InfoHash, orderBy OrderByFunc) []*peer.Peer {
	var cpy []*peer.Peer

	for _, peer := range svc.peers[hash] {
		cpy = append(cpy, peer)
	}

	if orderBy != nil {
		sort.SliceStable(cpy, func(i, j int) bool {
			return orderBy(cpy[i], cpy[j])
		})
	}

	return cpy
}

func (svc *peerService) Get(hash InfoHash, req GetRequest) GetResponse {
	svc.lock.Lock()
	defer svc.lock.Unlock()

	sorted := svc.getPeersSorted(hash, req.OrderBy)

	var out []*peer.Peer
	for _, p := range sorted {
		if req.Limit > 0 && len(out) == req.Limit {
			break
		}

		if req.Filter != nil && !req.Filter(p) {
			continue
		}

		out = append(out, p)
	}

	return GetResponse{Peers: out}
}

func (svc *peerService) Swarms() map[InfoHash]SwarmStat {
	out := make(map[InfoHash]SwarmStat)

	for hash, peers := range svc.peers {
		stat := SwarmStat{}

		for _, p := range peers {
			if p.Closed() {
				continue
			}

			stat.Peers = append(stat.Peers, PeerStat{
				IP:         p.RemoteAddr().String(),
				UploadRate: int(p.UploadRate),
				Uploaded:   int(p.Uploaded),
				Downloaded: int(p.Downloaded),
			})

			if p.Choked {
				stat.Choked++
			}

			if p.Blocking {
				stat.Blocking++
			}

			if p.Interested {
				stat.Interested++
			}

			if p.Interesting {
				stat.Interesting++
			}
		}

		out[hash] = stat
	}

	return out
}

func (svc *peerService) Add(hash InfoHash, p *peer.Peer) {
	svc.lock.Lock()
	defer svc.lock.Unlock()

	if bytes.Equal(svc.peerID[:], p.ID) {
		fmt.Println("TRIED ADDING MYSELF", string(svc.peerID[:]), string(p.ID))
		//p.Close("TRIED ADDING MYSELF")
		//return
	}

	if p.Closed() {
		return
	}

	s, ok := svc.peers[hash]
	if !ok {
		return
	}

	if _, ok := s[p.RemoteAddr().String()]; !ok {
		s[p.RemoteAddr().String()] = p
	}

	go func() {
		for msg := range p.Msg {
			svc.emitter <- MessageReceived{
				Peer: p,
				Hash: hash,
				Msg:  msg,
			}
		}
	}()
}

func (svc *peerService) Remove(hash InfoHash, p *peer.Peer) {
	svc.lock.Lock()
	defer svc.lock.Unlock()
	s, ok := svc.peers[hash]
	if !ok {
		return
	}

	delete(s, p.RemoteAddr().String())
}

func (svc *peerService) Register(t btorrent.Torrent) {
	_, ok := svc.peers[t.InfoHash()]
	if ok {
		return
	}

	svc.peers[t.InfoHash()] = make(map[string]*peer.Peer)

	for _, tier := range t.AnnounceList() {
		svc.trackers[t.InfoHash()] = append(svc.trackers[t.InfoHash()], tracker.NewGroup(tier))
	}
}

func (svc *peerService) Unregister(InfoHash) {}

func (svc *peerService) Discover(hash InfoHash, limit int, stat Stat) []tracker.PeerInfo {
	tg, ok := svc.trackers[hash]
	if !ok {
		return []tracker.PeerInfo{}
	}

	var out []tracker.PeerInfo
	group := tg[0]
	for _, p := range group.Announce(tracker.NewRequest(hash, svc.port, svc.peerID, stat.Downloaded)) {
		out = append(out, p)
	}

	shuffle(out)

	return out
}

func shuffle(a []tracker.PeerInfo) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
}

type Config struct {
	Port   uint16
	PeerID [20]byte
}

func NewService(cfg Config, emitter chan interface{}) Service {
	return &peerService{
		trackers: make(map[InfoHash][]*tracker.TrackerGroup),
		peers:    make(map[InfoHash]map[string]*peer.Peer),
		emitter:  emitter,
		port:     cfg.Port,
		peerID:   cfg.PeerID,
	}
}
