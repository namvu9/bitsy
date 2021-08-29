package conn

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type Service interface {
	Dial(net.Addr, peer.DialConfig) (*peer.Peer, error)
	Register(btorrent.Torrent)
	Unregister(btorrent.Torrent)

	Init(context.Context) error
}

type connectionService struct {
	torrents map[[20]byte]btorrent.Torrent
	port     uint16
	ip       string
	peerID   [20]byte
	emitter  chan<- interface{}
	conns    chan struct{}

	Reserved *peer.Extensions
	pstr     string
}

func (cs *connectionService) Dial(addr net.Addr, cfg peer.DialConfig) (*peer.Peer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p, err := peer.Dial(ctx, addr, cfg)
	if err != nil {
		return nil, err
	}
	p.OnClose(func(p *peer.Peer) {
		if len(cs.conns) == 0 {
			return
		}
		cs.emitter <- ConnCloseEvent{
			Peer: p,
			Hash: cfg.InfoHash,
		}
		<-cs.conns
	})

	select {
	case cs.conns <- struct{}{}:
	default:
		err := fmt.Errorf("exceeded max conns")
		p.Close(err.Error())
		return nil, err
	}

	go p.Listen(context.Background())

	cs.emitter <- NewConnEvent{Peer: p, Hash: cfg.InfoHash}
	return nil, nil
}
func (cs *connectionService) Register(t btorrent.Torrent) {
	cs.torrents[t.InfoHash()] = t
}

func (cs *connectionService) Unregister(t btorrent.Torrent) {
	delete(cs.torrents, t.InfoHash())
}
func (cs *connectionService) Init(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", cs.ip, cs.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				err = errors.Wrap(err)
				continue
			}

			err = cs.acceptHandshake(conn)
			if err != nil {
				conn.Close()
				continue
			}

			cs.conns <- struct{}{}
		}
	}()

	return nil
}

func (cs *connectionService) acceptHandshake(conn net.Conn) error {
	p := peer.New(conn)
	err := p.Init()
	if err != nil {
		return err
	}

	torrent, ok := cs.torrents[p.InfoHash]
	if !ok {
		err := fmt.Errorf("Unknown info hash: %x", p.InfoHash)
		return err
	}

	p.Pieces = bits.NewBitField(len(torrent.Pieces()))

	err = Handshake(p, torrent.InfoHash(), cs.peerID, cs.Reserved.ReservedBytes(), cs.pstr)
	if err != nil {
		return err
	}

	p.OnClose(func(p *peer.Peer) {
		if len(cs.conns) == 0 {
			return
		}
		cs.emitter <- ConnCloseEvent{
			Peer: p,
			Hash: torrent.InfoHash(),
		}
		<-cs.conns
	})

	go p.Listen(context.Background())
	cs.emitter <- NewConnEvent{Peer: p, Hash: torrent.InfoHash()}

	return nil
}

type Config struct {
	IP             string
	Port           uint16
	PeerID         [20]byte
	PStr           string
	Reserved       *peer.Extensions
	MaxConnections int
}

func NewService(cfg Config, emitter chan<- interface{}) Service {
	return &connectionService{
		torrents: make(map[[20]byte]btorrent.Torrent),
		conns:    make(chan struct{}, cfg.MaxConnections),
		ip:       cfg.IP,
		port:     cfg.Port,
		peerID:   cfg.PeerID,
		emitter:  emitter,

		Reserved: cfg.Reserved,
		pstr:     cfg.PStr,
	}
}

type upnpRes struct {
	externalIP string
	port       int
}

// Handshake attempts to perform a handshake with the given
// client at addr with infoHash.
func Handshake(p *peer.Peer, infoHash [20]byte, peerID [20]byte, reserved [8]byte, PStr string) error {
	msg := peer.HandshakeMessage{
		PStr:     PStr,
		PStrLen:  byte(len(PStr)),
		InfoHash: infoHash,
		PeerID:   peerID,
		Reserved: reserved,
	}

	p.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := p.Conn.Write(msg.Bytes())
	if err != nil {
		return err
	}

	return nil
}
