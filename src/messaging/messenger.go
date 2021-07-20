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
}

// Messenger listens for incoming connections
// TODO: Messenger feels a bit weird
// TODO: MOVE TO SESSION
// ListenForPeers
type Messenger struct {
	torrent.Torrent

	baseDir string
	port    uint16
	ip      string
	peerID  [20]byte

	swarms map[[20]byte]*Swarm
}

// Add an on-close event handler for peers

func (m *Messenger) Stat() []string {
	var out []string
	for _, swarm := range m.swarms {
		out = append(out, swarm.Stat())
	}

	return out
}

func (m *Messenger) Listen() error {
	var op errors.Op = "(*Messenger).Listen"

	var (
		addr          = fmt.Sprintf("%s:%d", m.ip, m.port)
		listener, err = net.Listen("tcp", addr)
	)
	if err != nil {
		return errors.Wrap(err, op, errors.Network)
	}

	log.Info().
		Str("name", "Messenger").
		Str("op", op.String()).
		Msgf("Listening on %s\n", addr)

	go func() {
		var op errors.Op = "(*Messenger).Listen.GoRoutine"

		for {
			conn, err := listener.Accept()

			if err != nil {
				err = errors.Wrap(err, op)
				log.Err(err).Strs("trace", errors.Ops(err)).Msg(err.Error())
				continue
			}

			go func(conn net.Conn) {
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

				peer, err := Handshake(conn, msg.infoHash, m.peerID)
				if err != nil {
					err = errors.Wrap(err, op)
					log.Error().Msg(err.Error())
					close(peer.in)
					return
				}

				swarm.peerCh <- peer

			}(conn)
		}
	}()

	return nil
}

func (m *Messenger) AddSwarm(t *torrent.Torrent, stat chan tracker.UDPAnnounceResponse) {
	var (
		hash  = t.InfoHash()
		swarm = NewSwarm(*t, stat, m.peerID, m.baseDir)
	)

	go swarm.Listen()

	m.swarms[hash] = &swarm
}

func New(host string, port uint16, conn *gateway.Gateway, peerID [20]byte, baseDir string) *Messenger {
	return &Messenger{
		ip:      host,
		port:    port,
		peerID:  peerID,
		swarms:  make(map[[20]byte]*Swarm),
		baseDir: baseDir,
	}
}
