package session

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path"
	"time"

	"github.com/namvu9/bitsy/internal/session/conn"
	"github.com/namvu9/bitsy/internal/session/data"
	"github.com/namvu9/bitsy/internal/session/peers"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
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

	// Config
	baseDir  string
	peerID   [20]byte
	port     uint16 // the port to listen on
	cmd      chan interface{}
	torrents map[[20]byte]btorrent.Torrent

	data  data.Service
	conn  conn.Service
	peers peers.Service
}

func (s *Session) handleEvents(ctx context.Context) {
	for {
		cmd := <-s.cmd
		switch v := cmd.(type) {
		case PauseCmd:
			s.data.Stop(v.Hash)
		case RegisterCmd:
			s.register(v.t, v.opts)
		case conn.NewConnEvent:
			s.handleNewConn(v)
		case conn.ConnCloseEvent:
			s.handleCloseConn(v)
		case peers.MessageReceived:
			s.handlePeerMessage(v)
		case data.RequestMessage:
			s.handleRequestMessage(v)
		case data.DownloadCompleteEvent:
			s.handleDownloadCompleteEvent(v)
		}
	}
}

func (s *Session) handleCloseConn(ev conn.ConnCloseEvent) {
	s.peers.Remove(ev.Hash, ev.Peer)
}

func (s *Session) handleDownloadCompleteEvent(ev data.DownloadCompleteEvent) {
	res := s.peers.Get(ev.Hash, peers.GetRequest{})
	for _, p := range res.Peers {
		go p.Send(peer.HaveMessage{Index: uint32(ev.Index)})
	}
}

func (s *Session) handleRequestMessage(msg data.RequestMessage) {
	res := s.peers.Get(msg.Hash, peers.GetRequest{
		Limit: 2,
		Filter: func(p *peer.Peer) bool {
			// OR FAST
			return p.HasPiece(int(msg.Index)) &&
				!p.Blocking &&
				!p.IsServing(msg.Index, msg.Offset)
		},
	})

	for _, p := range res.Peers {
		go p.Send(msg.RequestMessage)
	}
}

var pieceFreq = make(map[int]int)

func (s *Session) handlePeerMessage(ev peers.MessageReceived) {
	pieces, err := s.data.Pieces(ev.Hash)
	if err != nil {
		fmt.Println(err)
		return
	}

	switch msg := ev.Msg.(type) {
	case peer.AllowedFastMessage:
		// TODO: IMPLEMENT
	case peer.BitFieldMessage:
		t := s.torrents[ev.Hash]
		for idx := range t.Pieces() {
			if ev.Peer.HasPiece(idx) {
				pieceFreq[idx]++
			}

			if !pieces.Get(idx) {
				ev.Peer.Send(peer.InterestedMessage{})
			}
		}
	case peer.HaveMessage:
		pieceFreq[int(msg.Index)]++
		if !pieces.Get(int(msg.Index)) {
			ev.Peer.Send(peer.InterestedMessage{})
		}
	case peer.RequestMessage:
		data, err := s.data.GetPiece(ev.Hash, msg)
		if err != nil {
			return
		}

		ev.Peer.Send(peer.PieceMessage{
			Index:  msg.Index,
			Offset: msg.Offset,
			Piece:  data,
		})
	}
}

func (s *Session) fillSwarms() {
	for hash, stat := range s.peers.Swarms() {
		if stat.Peers < 10 {
			want := 10 - stat.Peers
			pInfo := s.peers.Discover(hash, 30)
			cfg := peer.DialConfig{
				PStr:       PStr,
				InfoHash:   hash,
				PeerID:     PeerID,
				Extensions: Reserved,
				Timeout:    500 * time.Millisecond,
			}
			for _, p := range pInfo {
				if want == 0 {
					return
				}

				addr := &net.TCPAddr{IP: p.IP, Port: int(p.Port)}
				_, err := s.conn.Dial(addr, cfg)
				if err != nil {
					continue
				}
				want--
			}
		}
	}
}

func (s *Session) handleNewConn(ev conn.NewConnEvent) {
	p := ev.Peer
	status, err := s.data.Status(ev.Hash)
	if err != nil {
		p.Close(err.Error())
		return
	}

	if status == data.PAUSED || status == data.ERROR {
		return
	}

	if status == data.STOPPED {
		s.data.Init(context.Background())
	}

	err = s.data.AddDataStream(ev.Hash, p)
	if err != nil {
		p.Close(err.Error())
		return
	}

	bitField, err := s.data.Pieces(ev.Hash)
	if err != nil {
		p.Close(err.Error())
		return
	}

	s.peers.Add(ev.Hash, p)
	p.Send(peer.BitFieldMessage{BitField: bitField})

	pieces, _ := s.data.Pieces(ev.Hash)
	var haveMsg peer.Message
	var haveN = pieces.GetSum()

	t := s.torrents[ev.Hash]
	if p.Extensions.IsEnabled(peer.EXT_FAST) && haveN == len(t.Pieces()) {
		haveMsg = peer.HaveAllMessage{}
	} else if p.Extensions.IsEnabled(peer.EXT_FAST) && haveN == 0 {
		haveMsg = peer.HaveNoneMessage{}
	} else {
		haveMsg = peer.BitFieldMessage{BitField: pieces}
	}
	go p.Send(haveMsg)
	addr := p.RemoteAddr().(*net.TCPAddr)

	if p.Extensions.IsEnabled(peer.EXT_FAST) {
		for _, pieceIdx := range t.GenFastSet(addr.IP, 10) {
			go p.Send(peer.AllowedFastMessage{Index: uint32(pieceIdx)})
		}
	}
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
	go s.heartbeat(ctx)

	return nil
}

func (s *Session) heartbeat(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	var lastInterval time.Time

	go func() {
		for {
			s.fillSwarms()
			time.Sleep(5 * time.Second)
		}
	}()

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

func (s *Session) stat() {
	for hash, torrent := range s.torrents {
		fmt.Println(torrent.Name())

		clientStat := s.data.Stat(hash)
		fmt.Printf("State: %s\n", clientStat.State)
		fmt.Printf("Uploaded: %s\n", clientStat.Uploaded)
		fmt.Printf("Downloaded: %s / %s\n", min(clientStat.Downloaded, torrent.Length()), torrent.Length())
		fmt.Printf("Download rate: %s / s\n", clientStat.DownloadRate)
		fmt.Printf("Pending pieces: %d\n", clientStat.Pending)

		for _, file := range clientStat.Files {
			if file.Ignored {
				continue
			}

			fmt.Printf("%s (%s/%s)\n", file.Name, file.Downloaded, file.Size)
		}

		fmt.Println(s.peers.Swarms()[hash])

	}

}

func min(a, b btorrent.Size) btorrent.Size {
	if a < b {
		return a
	}
	return b
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
	s.cmd <- PauseCmd{Hash: hash}

	return nil
}

func (s *Session) Register(t btorrent.Torrent, opts ...data.Option) {
	go func() {
		s.cmd <- RegisterCmd{t: t, opts: opts}
	}()
}

func New(cfg Config) *Session {
	cmd := make(chan interface{}, 32)

	s := &Session{
		peerID: PeerID,

		port:    cfg.Port,
		baseDir: cfg.BaseDir,

		cmd: cmd,

		torrents: make(map[[20]byte]btorrent.Torrent),

		data:  data.NewService(cfg.BaseDir, cfg.DownloadDir, cmd),
		conn:  conn.NewService(cfg.IP, cfg.Port, PeerID, cmd, cfg.MaxConnections, PStr, Reserved),
		peers: peers.NewService(cmd, cfg.Port, PeerID),
	}

	return s
}
