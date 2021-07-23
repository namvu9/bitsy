package bitsy

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"gitlab.com/NebulousLabs/go-upnp"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
)

type TimeoutErr error

// BoundedNet limits the number of open connections and
// satisfies and wraps the net.Dial and net.Listen functions
type BoundedNet struct {
	maxConns int
	nConns   int

	done    chan struct{}
	openCh  chan struct{}
	closeCh chan struct{}
}

func (bn *BoundedNet) Stop() error {
	bn.done <- struct{}{}

	return nil
}

func (bn *BoundedNet) Listen(network, addr string) (net.Listener, error) {
	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}

	return BoundedListener{
		Listener: listener,
		openCh:   bn.openCh,
		closeCh:  bn.closeCh,
	}, nil
}

func (bn *BoundedNet) Dial(network string, addr string) (net.Conn, error) {
	timeout := 2 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil, TimeoutErr(fmt.Errorf("Dial timed out"))
	case bn.openCh <- struct{}{}:
		conn, err := net.DialTimeout(network, addr, timeout)
		if err != nil {
			return nil, err
		}

		return Conn{conn, bn.closeCh}, nil
	}
}

type DialFunc func(string, string) (net.Conn, error)
type ListenFunc func(string, string) (net.Listener, error)

func NewBoundedNet(max int) BoundedNet {
	bn := BoundedNet{
		openCh:  make(chan struct{}, max),
		closeCh: make(chan struct{}, max),
		done:    make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-bn.done:
				return
			case <-bn.closeCh:
				<-bn.openCh
			}
		}
	}()

	return bn
}

type Conn struct {
	net.Conn
	closeCh chan struct{}
}

func (c Conn) Close() error {
	c.closeCh <- struct{}{}
	return c.Conn.Close()
}

type BoundedListener struct {
	net.Listener
	openCh  chan struct{}
	closeCh chan struct{}
}

func (bl BoundedListener) Accept() (net.Conn, error) {
	bl.openCh <- struct{}{}

	conn, err := bl.Listener.Accept()
	if err != nil {
		return nil, err
	}

	return Conn{conn, bl.closeCh}, nil
}

type upnpRes struct {
	externalIP string
	port       int
}

func forwardPorts(ports []uint16) (uint16, error) {
	var op errors.Op = "client.forwardPorts"

	// Discover UPnP-supporting routers
	d, err := upnp.Discover()
	if err != nil {
		return 0, errors.Wrap(err, op, errors.Network)
	}

	// forward a port
	for _, port := range ports {
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
func Handshake(conn net.Conn, infoHash [20]byte, peerID [20]byte) error {
	msg := btorrent.HandshakeMessage{
		InfoHash: infoHash,
		PeerID:   peerID,
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(msg.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func (s *Session) VerifyHandshake(msg btorrent.HandshakeMessage) error {
	if !bytes.Equal(msg.PStr, []byte("BitTorrent protocol")) {
		err := errors.Newf("expected pStr %s but got %s\n", "BitTorrent protocol", msg.PStr)
		return err
	}

	_, ok := s.swarms[msg.InfoHash]
	if !ok {
		err := errors.Newf("Unknown infoHash %v\n", msg.InfoHash)
		return err
	}

	return nil
}
