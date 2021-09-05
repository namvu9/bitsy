package conn

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type Service interface {
	Dial(net.Addr, peer.DialConfig) (*peer.Peer, error)
	Register(btorrent.Torrent)
	Unregister(btorrent.Torrent)

	Init(context.Context) error
	Stat() int
}

type connectionService struct {
	torrents map[[20]byte]btorrent.Torrent
	port     uint16
	ip       string
	extIP    string
	peerID   [20]byte
	emitter  chan<- interface{}
	conns    chan struct{}

	Reserved *peer.Extensions
	pstr     string

	hosts map[net.Addr]bool

	lock sync.Mutex
}

func (cs *connectionService) closeConn(p *peer.Peer) {
	cs.lock.Lock()
	defer cs.lock.Unlock()

	if !cs.hosts[p.RemoteAddr()] {
		return
	}

	delete(cs.hosts, p.RemoteAddr())
	<-cs.conns
}

func (cs *connectionService) Dial(addr net.Addr, cfg peer.DialConfig) (*peer.Peer, error) {
	if strings.Contains(addr.String(), cs.extIP) {
		return nil, fmt.Errorf("cannot dial self")
	}

	if _, ok := cs.hosts[addr]; ok {
		return nil, fmt.Errorf("already dialed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	p, err := peer.Dial(ctx, addr, cfg)
	if err != nil {
		return nil, err
	}

	select {
	case cs.conns <- struct{}{}:
		cs.lock.Lock()
		cs.hosts[p.RemoteAddr()] = true
		cs.lock.Unlock()
		p.OnClose(cs.closeConn)
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
				continue
			}

			select {
			case cs.conns <- struct{}{}:
				cs.lock.Lock()
				cs.hosts[conn.RemoteAddr()] = true
				cs.lock.Unlock()
				err = cs.acceptHandshake(conn)
				if err != nil {
					conn.Close()
					<-cs.conns
					continue
				}
			default:
				conn.Close()
			}
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

	p.OnClose(cs.closeConn)

	go p.Listen(context.Background())
	// TODO: Don't create peer here, inject dependency to
	// peers service
	cs.emitter <- NewConnEvent{Peer: p, Hash: torrent.InfoHash()}

	return nil
}

func (cs *connectionService) Stat() int {
	return len(cs.conns)
}

type Config struct {
	IP             string
	ExtIP          string
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
		extIP:    cfg.ExtIP,
		port:     cfg.Port,
		peerID:   cfg.PeerID,
		emitter:  emitter,
		hosts:    make(map[net.Addr]bool),

		Reserved: cfg.Reserved,
		pstr:     cfg.PStr,
	}
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
