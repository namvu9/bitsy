package data

import (
	"context"
	"fmt"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type InfoHash [20]byte

type Service interface {
	Init(context.Context) error
	Register(btorrent.Torrent, ...Option)
	Stop(InfoHash) error
	Status(InfoHash) (ClientState, error)
	Pieces(InfoHash) (bits.BitField, error)

	Stat(InfoHash) ClientStat

	GetPiece(InfoHash, peer.RequestMessage) ([]byte, error)

	AddDataStream(InfoHash, *peer.Peer) error

	// TODO: Implement piece priority:
	// e.g., AllowedFast? Rare piece?
	SetPriority(InfoHash, []int) error
}

type dataService struct {
	clients  map[InfoHash]*Client
	torrents map[InfoHash]btorrent.Torrent
	ranks    map[InfoHash][]int

	baseDir     string
	downloadDir string
	emitter     chan interface{}
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
	if _, ok := ds.clients[t.InfoHash()]; !ok {
		c := New(t, ds.baseDir, ds.downloadDir, ds.emitter, opts...)
		ds.clients[t.InfoHash()] = c
		ds.torrents[t.InfoHash()] = t
		ds.ranks[t.InfoHash()] = make([]int, len(t.Pieces()))
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

	return c.state, nil
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

func NewService(baseDir, downloadDir string, emitter chan interface{}) Service {
	return &dataService{
		clients:     make(map[InfoHash]*Client),
		torrents:    make(map[InfoHash]btorrent.Torrent),
		ranks:       make(map[InfoHash][]int),
		baseDir:     baseDir,
		downloadDir: downloadDir,
		emitter:     emitter,
	}
}
