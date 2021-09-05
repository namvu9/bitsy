package session

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/namvu9/bitsy/internal/assembler"
	"github.com/namvu9/bitsy/internal/conn"
	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/internal/peers"
	"github.com/namvu9/bitsy/internal/pieces"
	"github.com/namvu9/bitsy/internal/ports"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
)

var PeerID = [20]byte{'-', 'B', 'T', '0', '0', '0', '0', '-'}
var Reserved = peer.NewExtensionsField([8]byte{})

const PStr = "BitTorrent protocol"

func init() {
	rand.Read(PeerID[8:])

	err := Reserved.Enable(peer.EXT_PROT)
	if err != nil {
		panic(err)
	}

	err = Reserved.Enable(peer.EXT_FAST)
	if err != nil {
		panic(err)
	}
}

// Session represents an instance of the client and manages
// the client's state.
type Session struct {
	startedAt time.Time

	baseDir  string
	eventsIn chan interface{}
	torrents map[[20]byte]btorrent.Torrent

	data  data.Service
	conn  conn.Service
	peers peers.Service
}

func (s *Session) handleEvents(ctx context.Context) {
	for {
		event := <-s.eventsIn
		switch v := event.(type) {
		case PauseCmd:
			s.data.Stop(v.Hash)
		case RegisterCmd:
			s.register(v.t, v.opts)
		case conn.NewConnEvent:
			s.handleNewConn(v)
		case conn.ConnCloseEvent:
			s.peers.Remove(v.Hash, v.Peer)
		case peers.MessageReceived:
			s.handlePeerMessage(v)
		case data.RequestMessage:
			s.handleRequestMessage(v)
		case data.DownloadCompleteEvent:
			s.handleDownloadCompleteEvent(v)
		}
	}
}

func (s *Session) loadTorrents() error {
	torrents, err := btorrent.LoadDir(s.baseDir)
	if err != nil {
		return err
	}

	for _, t := range torrents {
		s.register(*t, []data.Option{})
	}

	return nil
}

func (s *Session) Init() error {
	ctx := context.Background()
	s.startedAt = time.Now()

	err := s.loadTorrents()
	if err != nil {
		return err
	}

	err = s.conn.Init(ctx)
	if err != nil {
		return err
	}

	err = s.data.Init(ctx)
	if err != nil {
		return err
	}

	go s.handleEvents(ctx)
	go s.checkSwarmHealth(ctx)
	go s.heartbeat(ctx)

	return nil
}

func (s *Session) register(t btorrent.Torrent, opts []data.Option) {
	location := path.Join(s.baseDir, fmt.Sprintf("%s.torrent", t.HexHash()))

	if _, err := os.Stat(location); errors.Is(err, os.ErrNotExist) {
		err := btorrent.Save(location, &t)
		if err != nil {
			panic(err)
		}
	}

	s.torrents[t.InfoHash()] = t

	s.conn.Register(t)
	s.peers.Register(t)
	s.data.Register(t, opts...)
}

func (s *Session) Register(t btorrent.Torrent, opts ...data.Option) {
	go func() {
		s.eventsIn <- RegisterCmd{t: t, opts: opts}
	}()
}

func (s *Session) checkSwarmHealth(ctx context.Context) {
	for {
		s.fillSwarms()
		time.Sleep(5 * time.Second)
	}
}

func (s *Session) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	var lastInterval time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if time.Now().Sub(lastInterval) > 5*time.Second {
				s.unchoke()
				lastInterval = time.Now()
			}

			s.stat()
		}
	}
}

func (s *Session) fillSwarms() {
	for hash, stat := range s.peers.Swarms() {
		if stat.Peers < 30 {
			stat := s.data.Stat(hash)
			pInfo := s.peers.Discover(hash, 100, peers.Stat{
				Downloaded: uint64(stat.Downloaded),
			})
			cfg := peer.DialConfig{
				PStr:       PStr,
				InfoHash:   hash,
				PeerID:     PeerID,
				Extensions: Reserved,
				Timeout:    500 * time.Millisecond,
			}
			for _, p := range pInfo {
				go func(p tracker.PeerInfo) {
					addr := &net.TCPAddr{IP: p.IP, Port: int(p.Port)}
					_, err := s.conn.Dial(addr, cfg)
					if err != nil {
						return
					}
				}(p)

			}
		}
	}
}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// Optimistic unchoke
func (s *Session) unchoke() {
	for hash, stat := range s.peers.Swarms() {
		res := s.peers.Get(hash, peers.GetRequest{
			Limit: 1,
			Filter: func(p *peer.Peer) bool {
				return p.Choked && p.Interested
			},
		})

		for _, p := range res.Peers {
			p.Send(peer.UnchokeMessage{})
		}

		if stat.Peers-stat.Choked >= 4 {
			res := s.peers.Get(hash, peers.GetRequest{
				Limit: 1,
				Filter: func(p *peer.Peer) bool {
					return !p.Choked
				},
			})

			for _, p := range res.Peers {
				p.Send(peer.ChokeMessage{})
			}
		}

	}
}

func (s *Session) Pause(hash [20]byte) error {
	s.eventsIn <- PauseCmd{Hash: hash}

	return nil
}

func (s *Session) Start(hash [20]byte) error {
	s.eventsIn <- StartCmd{Hash: hash}

	return nil
}

func New(cfg Config) *Session {
	eventsIn := make(chan interface{}, 100)

	var portsService = ports.NewService()
	port, err := portsService.ForwardMany(cfg.Ports)
	if err != nil {
		panic(err)
	}

	var connCfg = conn.Config{
		IP:             cfg.IP,
		Port:           port,
		PeerID:         PeerID,
		MaxConnections: cfg.MaxConnections,
		PStr:           PStr,
		Reserved:       Reserved,
	}
	connService := conn.NewService(connCfg, eventsIn)

	var piecesCfg = pieces.Config{
		BaseDir: cfg.BaseDir,
	}
	pieceMgr := pieces.NewService(piecesCfg)

	var assemblerCfg = assembler.Config{
		BaseDir:     cfg.BaseDir,
		DownloadDir: cfg.DownloadDir,
		PieceMgr:    pieceMgr,
	}
	assemblerService := assembler.NewService(assemblerCfg)

	var dataCfg = data.Config{
		BaseDir:     cfg.BaseDir,
		DownloadDir: cfg.DownloadDir,
		Assembler:   assemblerService,
		PieceMgr:    pieceMgr,
	}
	dataService := data.NewService(dataCfg, eventsIn)

	var peersCfg = peers.Config{
		Port:   port,
		PeerID: PeerID,
	}
	peersService := peers.NewService(peersCfg, eventsIn)

	return &Session{
		eventsIn: eventsIn,
		baseDir:  cfg.BaseDir,
		torrents: make(map[[20]byte]btorrent.Torrent),
		data:     dataService,
		conn:     connService,
		peers:    peersService,
	}
}
