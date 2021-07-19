package messaging

import (
	"bytes"
	"fmt"
	"net"

	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/gateway"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
	"github.com/rs/zerolog/log"
)

type DialListener interface {
	Dial(string, string) (net.Conn, error)
	Listen(string, string) (net.Listener, error)
	CanDial() bool
}

// TODO: Messenger feels a bit weird
// Messenger listens for incoming connections
// Performs handshake
// If OK, sends the peer connection to the appropriate swarm
type Messenger struct {
	torrent.Torrent

	baseDir string

	d      DialListener
	Port   uint16
	HostIP string

	swarms map[[20]byte]*Swarm
	peerID [20]byte
}

func (m *Messenger) Stat() []string {
	var out []string
	for _, swarm := range m.swarms {
		out = append(out, swarm.Stat())
	}

	return out
}

func (m *Messenger) Listen() error {
	var op errors.Op = "(*Messenger).Listen"

	addr := fmt.Sprintf("%s:%d", m.HostIP, m.Port)
	listener, err := m.d.Listen("tcp", addr)
	if err != nil {
		return errors.Wrap(err, op, errors.Network)
	}

	log.Info().
		Str("name", "Messenger").
		Str("op", op.String()).
		Msgf("Listening on %s\n", addr)

	go func() {
		var op errors.Op = "(*Messenger).Listen.GoRoutine"

		n := 0

		for {
			conn, err := listener.Accept()
			if err != nil {
				err = errors.Wrap(err, op)
				log.Err(err).Strs("trace", errors.Ops(err)).Msg(err.Error())
				continue
			}

			fmt.Println("ACCEPTED")

			n++

			var op errors.Op = "(*Messenger).Listen.GoRoutine.GoRoutine"
			var msg HandShakeMessage
			err = unmarshalHandshake(conn, &msg)
			if err != nil {
				err = errors.Wrap(err, op)
				log.Err(err).Strs("trace", errors.Ops(err)).Msg("Bad handshake")
				conn.Close()
				return
			}

			if !bytes.Equal(msg.pStr, []byte("BitTorrent protocol")) {
				err := errors.Newf("expected pStr %s but got %s\n", "BitTorrent protocol", msg.pStr)
				log.Err(err).Strs("trace", []string{string(op)}).Msg("Bad handshake")
				conn.Close()
				return
			}

			swarm, ok := m.swarms[msg.infoHash]
			if !ok {
				err := errors.Newf("Unknown infoHash %v\n", msg.infoHash)
				log.Error().Msg(err.Error())
				conn.Close()
				return
			}

			peerConn, err := Handshake(conn, msg.infoHash, m.peerID)
			if err != nil {
				fmt.Println("HANDSHAKE FAILED", err, op)
				err = errors.Wrap(err, op)
				log.Error().Msg(err.Error())
				swarm.closeCh <- peerConn
				return
			}

			swarm.peerCh <- peerConn
		}
	}()

	return nil
}

func (m *Messenger) AddSwarm(t *torrent.Torrent, stat chan tracker.UDPAnnounceResponse, out chan map[string]interface{}) {
	hash := *t.InfoHash()
	swarm := NewSwarm(*t, stat, m.d, m.peerID, m.baseDir, out)

	go swarm.Listen()

	m.swarms[hash] = &swarm
}

func New(host string, port uint16, conn *gateway.Gateway, peerID [20]byte, baseDir string) *Messenger {
	return &Messenger{
		d:       conn,
		HostIP:  host,
		Port:    port,
		peerID:  peerID,
		swarms:  make(map[[20]byte]*Swarm),
		baseDir: baseDir,
	}
}
