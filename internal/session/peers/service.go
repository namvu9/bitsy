package peers

import (
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

type GetRequest struct {
	Limit   int
	OrderBy func()
	Filter  func(*peer.Peer) bool
}

type GetResponse struct {
	Peers []*peer.Peer
}

type Service interface {
	Get(InfoHash, GetRequest) GetResponse
	Add(InfoHash, *peer.Peer)
	Remove(InfoHash, *peer.Peer)

	//Stat(InfoHash)
	Register(btorrent.Torrent)
	Unregister(InfoHash)

	// Return info for peers that aren't currently in the
	// swarm
	Discover(InfoHash, int) []tracker.PeerInfo
	Swarms() map[InfoHash]swarm.SwarmStat
}
type InfoHash = [20]byte

type peerService struct {
	emitter  chan interface{}
	trackers map[InfoHash][]*tracker.TrackerGroup
	peers    map[InfoHash]map[string]*peer.Peer

	port   uint16
	peerID [20]byte
}

// TODO: Shuffle if OrderBy == nil
func (service *peerService) Get(hash InfoHash, req GetRequest) GetResponse {
	var out []*peer.Peer
	for _, p := range service.peers[hash] {
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

func (service *peerService) Swarms() map[InfoHash]swarm.SwarmStat {
	out := make(map[InfoHash]swarm.SwarmStat)

	for hash, peers := range service.peers {
		stat := swarm.SwarmStat{
			Peers: len(peers),
		}

		for _, p := range peers {
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
	s, ok := service.peers[hash]
	if !ok {
		p.Close("unknown hash")
		return
	}

	s[p.RemoteAddr().String()] = p

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

func NewService(emitter chan interface{}, port uint16, peerID [20]byte) Service {
	return &peerService{
		trackers: make(map[InfoHash][]*tracker.TrackerGroup),
		peers:    make(map[InfoHash]map[string]*peer.Peer),
		emitter:  emitter,
		port:     port,
		peerID:   peerID,
	}
}
