package data

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/namvu9/bitsy/internal/assembler"
	"github.com/namvu9/bitsy/internal/pieces"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type InfoHash [20]byte

type Service interface {
	AddDataStream(InfoHash, *peer.Peer) error
	GetPiece(InfoHash, peer.RequestMessage) ([]byte, error)
	Init(context.Context) error
	Pieces(InfoHash) (bits.BitField, error)
	Register(btorrent.Torrent, ...Option)
	RequestPiece(InfoHash, int) error
	SetPriority(InfoHash, []int) error
	Stat(InfoHash) ClientStat
	Status(InfoHash) (ClientState, error)
	Stop(InfoHash) error
}

type dataService struct {
	clients  map[InfoHash]*Client
	torrents map[InfoHash]btorrent.Torrent
	ranks    map[InfoHash][]int

	baseDir     string
	downloadDir string
	emitter     chan interface{}

	assembler assembler.Service
	pieceMgr  pieces.Service
}

func (ds *dataService) GetPiece(hash InfoHash, req peer.RequestMessage) ([]byte, error) {
	c, _ := ds.clients[hash]
	return c.handleRequestMessage(req)
}

func (ds *dataService) Stat(hash InfoHash) ClientStat {
	c, _ := ds.clients[hash]

	return c.Stat()
}

func (ds *dataService) Init(ctx context.Context) error {
	for _, t := range ds.torrents {
		torrentDir := path.Join(ds.baseDir, t.HexHash())
		err := os.MkdirAll(torrentDir, 0777)
		if err != nil {
			return err
		}
	}

	err := ds.assembler.Init()
	if err != nil {
		return err
	}

	err = ds.pieceMgr.Init()
	if err != nil {
		return err
	}

	for _, c := range ds.clients {
		if c.state == PAUSED {
			continue
		}

		c.Start(ctx)
	}

	return nil
}

func (ds *dataService) AddDataStream(hash InfoHash, p *peer.Peer) error {
	c, ok := ds.clients[hash]
	if !ok {
		return fmt.Errorf("no active client for %v", hash)
	}

	go func() {
		for {
			msg, ok := <-p.DataStream
			if !ok {
				return
			}

			c.MsgIn <- messageReceived{p, msg}
		}
	}()

	return nil
}

func (ds *dataService) Register(t btorrent.Torrent, opts ...Option) {
	if client, ok := ds.clients[t.InfoHash()]; !ok {
		ds.assembler.Register(t)
		ds.pieceMgr.Register(t)

		c := New(t, ds.baseDir, ds.downloadDir, ds.emitter, ds.pieceMgr, ds.assembler, opts...)
		ds.clients[t.InfoHash()] = c
		ds.torrents[t.InfoHash()] = t
		ds.ranks[t.InfoHash()] = make([]int, len(t.Pieces()))
	} else if len(opts) > 0 {
		for _, opt := range opts {
			opt(client)
		}
	}
}

func (ds *dataService) Stop(hash InfoHash) error {
	c, ok := ds.clients[hash]
	if !ok {
		return fmt.Errorf("no client found for %v", hash)
	}

	return c.Stop()
}

func (ds *dataService) Pieces(hash InfoHash) (bits.BitField, error) {
	c, ok := ds.clients[hash]
	if !ok {
		return nil, fmt.Errorf("no active client for %v", hash)
	}

	return c.pieces, nil
}

func (ds *dataService) Status(hash InfoHash) (ClientState, error) {
	c, ok := ds.clients[hash]
	if !ok {
		return ERROR, fmt.Errorf("unknown info hash %v", hash)
	}

	return c.state, c.err
}

func (ds *dataService) SetPriority(hash InfoHash, ranks []int) error {
	t, ok := ds.torrents[hash]

	if !ok {
		return fmt.Errorf("unknown info hash")
	}

	if len(ranks) != len(t.Pieces()) {
		panic("invalid length for ranks")
	}

	return nil
}

func (ds *dataService) RequestPiece(hash InfoHash, idx int) error {
	c, _ := ds.clients[hash]

	if !c.pieces.Get(idx) {
		c.downloadPiece(uint32(idx), true)
	}

	return nil
}

type Config struct {
	BaseDir     string
	DownloadDir string

	Assembler assembler.Service
	PieceMgr  pieces.Service
}

func NewService(cfg Config, emitter chan interface{}) Service {
	return &dataService{
		clients:     make(map[InfoHash]*Client),
		torrents:    make(map[InfoHash]btorrent.Torrent),
		ranks:       make(map[InfoHash][]int),
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		emitter:     emitter,
		assembler:   cfg.Assembler,
		pieceMgr:    cfg.PieceMgr,
	}
}
