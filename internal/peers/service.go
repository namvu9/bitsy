package peers

import (
	"sort"
	"sync"

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

type Service interface {
	Get(InfoHash, GetRequest) GetResponse
	Add(InfoHash, *peer.Peer)
	Remove(InfoHash, *peer.Peer)

	Register(btorrent.Torrent)
	Unregister(InfoHash)

	// Return info for peers that aren't currently in the
	// swarm
	Discover(InfoHash, int) []tracker.PeerInfo
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

func (service *peerService) getPeersSorted(hash InfoHash, orderBy OrderByFunc) []*peer.Peer {
	var cpy []*peer.Peer

	for _, peer := range service.peers[hash] {
		cpy = append(cpy, peer)
	}

	if orderBy != nil {
		sort.SliceStable(cpy, func(i, j int) bool {
			return orderBy(cpy[i], cpy[j])
		})
	}

	return cpy
}

func (service *peerService) Get(hash InfoHash, req GetRequest) GetResponse {
	sorted := service.getPeersSorted(hash, req.OrderBy)

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

func (service *peerService) Swarms() map[InfoHash]SwarmStat {
	out := make(map[InfoHash]SwarmStat)

	for hash, peers := range service.peers {
		sorted := service.getPeersSorted(hash, func(p1, p2 *peer.Peer) bool {
			if p1.UploadRate > p2.UploadRate {
				return true
			}

			if p1.UploadRate == p2.UploadRate && p1.Uploaded > p2.Uploaded {
				return true
			}

			return len(p1.Requests) > len(p2.Requests)
		})

		stat := SwarmStat{}

		if len(sorted) < 5 {
			stat.TopPeers = sorted
		} else {
			stat.TopPeers = sorted[:5]
		}

		sorted = service.getPeersSorted(hash, func(p1, p2 *peer.Peer) bool {
			return p1.Downloaded > p2.Downloaded
		})

		if len(sorted) < 5 {
			stat.TopDownloaders = sorted
		} else {
			stat.TopDownloaders = sorted[:5]
		}

		for _, p := range peers {
			if p.Closed() {
				continue
			}

			stat.Peers++
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

func (service *peerService) Add(hash InfoHash, p *peer.Peer) {
	service.lock.Lock()
	defer service.lock.Unlock()

	if p.Closed() {
		return
	}

	s, ok := service.peers[hash]
	if !ok {
		return
	}

	if _, ok := s[p.RemoteAddr().String()]; !ok {
		s[p.RemoteAddr().String()] = p
	}

	go func() {
		for msg := range p.Msg {
			service.emitter <- MessageReceived{
				Peer: p,
				Hash: hash,
				Msg:  msg,
			}
		}
	}()
}

func (service *peerService) Remove(hash InfoHash, p *peer.Peer) {
	service.lock.Lock()
	defer service.lock.Unlock()
	s, ok := service.peers[hash]
	if !ok {
		return
	}

	delete(s, p.RemoteAddr().String())
}

func (service *peerService) Register(t btorrent.Torrent) {
	_, ok := service.peers[t.InfoHash()]
	if ok {
		return
	}

	service.peers[t.InfoHash()] = make(map[string]*peer.Peer)

	for _, tier := range t.AnnounceList() {
		service.trackers[t.InfoHash()] = append(service.trackers[t.InfoHash()], tracker.NewGroup(tier))
	}
}

func (service *peerService) Unregister(InfoHash) {}

func (service *peerService) Discover(hash InfoHash, limit int) []tracker.PeerInfo {
	tg, ok := service.trackers[hash]
	if !ok {
		return []tracker.PeerInfo{}
	}

	var out []tracker.PeerInfo
	group := tg[0]
	for _, p := range group.Announce(tracker.NewRequest(hash, service.port, service.peerID)) {
		out = append(out, p)
	}

	return out
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
