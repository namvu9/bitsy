package bitsy

import (
	"fmt"
	"path"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
	"github.com/namvu9/bitsy/swarm"
	"github.com/namvu9/bitsy/tracker"
	"github.com/rs/zerolog/log"
)

// TODO: Call Announce from here
// TODO: Swarms do not connect to peers, we do it here
// instead and send the resulting peer connection to the
// appropriate swarm

// TorrentID is the SHA-1 hash of the torrent's bencoded
// info dictionary
type TorrentID [20]byte

type Trackers []tracker.TrackerStat

// Session represents an instance of the client and manages
// the client's state.
type Session struct {
	// Stats
	startedAt time.Time

	// state
	swarms   map[TorrentID]*swarm.Swarm
	torrents map[TorrentID]btorrent.Torrent
	trackers map[TorrentID]*tracker.TrackerGroup

	// Config
	peerID      [20]byte
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections

	preferredPorts []uint16
}

func New(cfg Config) *Session {
	// TODO: Generate a proper peerID. This is the peerID used
	// by the 'Transmission' BitTorrent client
	peerID := [20]byte{0x2d, 0x54, 0x52, 0x33, 0x30, 0x30, 0x30, 0x2d, 0x6c, 0x62, 0x76, 0x30, 0x65, 0x65, 0x75, 0x35, 0x34, 0x31, 0x36, 0x37}

	c := &Session{
		baseDir:        cfg.BaseDir,
		downloadDir:    cfg.DownloadDir,
		maxConns:       cfg.MaxConnections,
		peerID:         peerID,
		preferredPorts: cfg.Port,
		torrents:       make(map[TorrentID]btorrent.Torrent),
		trackers:       make(map[TorrentID]*tracker.TrackerGroup),
		swarms:         make(map[TorrentID]*swarm.Swarm),
	}

	if cfg.IP == "" {
		c.ip = "127.0.0.1"
	} else {
		c.ip = cfg.IP
	}

	return c
}

func (s *Session) Stat() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["name"] = "session"
	stats["sessionLength"] = time.Now().Sub(s.startedAt)

	var torrents = []map[string]interface{}{}
	for _, torrent := range s.torrents {
		torrents = append(torrents, torrent.Stat())
	}

	stats["torrents"] = torrents

	var swarms []map[string]interface{}
	for _, swarm := range s.swarms {
		swarms = append(swarms, swarm.Stat())
	}
	stats["swarms"] = swarms

	return stats
}

func (sess *Session) Init() (func() error, error) {
	var op errors.Op = "(*Client).Init"

	log.Info().
		Str("op", op.String()).
		Msg("Initializing client")

	sess.startedAt = time.Now()

	port, err := forwardPorts(append(sess.preferredPorts, DEFAULTPORTS...))
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	sess.port = port

	dir := path.Join(sess.baseDir, "torrents")
	torrents, err := btorrent.LoadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	log.Printf("<<<<<<<<<Loaded %d torrents from %s\n", len(torrents), dir)
	for _, t := range torrents {
		net := NewBoundedNet(20)

		sess.torrents[t.InfoHash()] = *t
		tr := tracker.NewTracker(*t, sess.port, sess.peerID)
		stat, _ := tr.Listen(&net)

		sess.trackers[t.InfoHash()] = tr

		swarm := swarm.New(*t, stat, sess.peerID, path.Join(sess.baseDir, sess.downloadDir))
		err := swarm.Init()
		if err != nil {
			log.Print(err.Error())
		}
		
		sess.swarms[t.InfoHash()] = &swarm
	}

	go sess.listen()

	return func() error { return nil }, nil
}

func (s *Session) listen() error {
	var (
		addr = fmt.Sprintf("%s:%d", s.ip, s.port)
		net  = NewBoundedNet(s.maxConns)
	)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()

		if err != nil {
			err = errors.Wrap(err)
			log.Err(err).Strs("trace", errors.Ops(err)).Msg(err.Error())
			continue
		}

		go func() {
			var op errors.Op = "(*Messenger).Listen.GoRoutine.GoRoutine"
			var msg btorrent.HandshakeMessage

			err = btorrent.UnmarshalHandshake(conn, &msg)
			if err != nil {
				err = errors.Wrap(err, op)
				log.Err(err).Strs("trace", errors.Ops(err)).Msg("Bad handshake")
				conn.Close()
				return
			}

			err = s.VerifyHandshake(msg)
			if err != nil {
				return
			}

			err := Handshake(conn, msg.InfoHash, s.peerID)
			if err != nil {
				err = errors.Wrap(err, op)
				log.Error().Msg(err.Error())
				conn.Close()
				return
			}

			log.Printf("Handshake complete: %s", conn.RemoteAddr())

			s.swarms[msg.InfoHash].PeerCh <- conn
		}()
	}
}
