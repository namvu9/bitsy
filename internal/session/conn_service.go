package session

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"gitlab.com/NebulousLabs/go-upnp"
)

type ConnectionService interface {
	Call(net.Addr, btorrent.Torrent) (*peer.Peer, error)
	Register(btorrent.Torrent)
	Unregister(btorrent.Torrent)

	Init(context.Context) error
}

type connectionService struct {
	torrents map[[20]byte]btorrent.Torrent
	port     uint16
	peerID   [20]byte
	emit     func(interface{})
	conns    chan struct{}
}

func (cs *connectionService) Call(net.Addr, btorrent.Torrent) (*peer.Peer, error) {
	return nil, nil
}
func (cs *connectionService) Register(t btorrent.Torrent) {
	cs.torrents[t.InfoHash()] = t
}

func (cs *connectionService) Unregister(t btorrent.Torrent) {
	delete(cs.torrents, t.InfoHash())
}
func (cs *connectionService) Init(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", cs.port))
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				err = errors.Wrap(err)
				fmt.Println(err)
				continue
			}

			err = cs.acceptHandshake(conn)
			if err != nil {
				conn.Close()
				fmt.Println(err)
				continue
			}

			cs.conns <- struct{}{}
			fmt.Println("CONNS", len(cs.conns))
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
		fmt.Println(err)
		return err
	}

	p.Pieces = bits.NewBitField(len(torrent.Pieces()))

	err = Handshake(p, torrent.InfoHash(), cs.peerID, Reserved.ReservedBytes())
	if err != nil {
		return err
	}

	p.OnClose(func(p *peer.Peer) {
		<-cs.conns
		fmt.Println("CONNS", len(cs.conns))
	})

	go p.Listen(context.Background())
	cs.emit(swarm.JoinEvent{Peer: p, Hash: torrent.InfoHash()})

	return nil
}

func NewConnectionService(port uint16, peerID [20]byte, emitter func(interface{}), max int) ConnectionService {
	return &connectionService{
		torrents: make(map[[20]byte]btorrent.Torrent),
		conns:    make(chan struct{}, max),
		port:     port,
		peerID:   peerID,
		emit:     emitter,
	}
}

type upnpRes struct {
	externalIP string
	port       int
}

func ForwardPorts(from, to uint16) (uint16, error) {
	var op errors.Op = "client.forwardPorts"

	// Discover UPnP-supporting routers
	d, err := upnp.Discover()
	if err != nil {
		return 0, errors.Wrap(err, op, errors.Network)
	}

	// forward a port
	for port := from; port <= to; port++ {
		err = d.Forward(port, "Bitsy BitTorrent client")
		if err != nil {
			continue
		}

		return port, err
	}

	err = fmt.Errorf("could not forward any of the specified ports")
	return 0, errors.Wrap(err, op, errors.Network)
}

// Handshake attempts to perform a handshake with the given
// client at addr with infoHash.
func Handshake(conn net.Conn, infoHash [20]byte, peerID [20]byte, reserved [8]byte) error {
	msg := peer.HandshakeMessage{
		PStr:     PStr,
		PStrLen:  byte(len(PStr)),
		InfoHash: infoHash,
		PeerID:   peerID,
		Reserved: reserved,
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(msg.Bytes())
	if err != nil {
		return err
	}

	return nil
}
