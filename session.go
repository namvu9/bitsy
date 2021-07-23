package bitsy

import (
	"encoding/hex"
	"fmt"
	"net"
	"path"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
	"github.com/namvu9/bitsy/swarm"
	"github.com/namvu9/bitsy/tracker"
	"github.com/rs/zerolog/log"
)

// TorrentID is the SHA-1 hash of the torrent's bencoded
// info dictionary
type TorrentID [20]byte

// Session represents an instance of the client and manages
// the client's state.
type Session struct {
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

	swarmCh chan swarm.Event
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

	var trackers = map[string]interface{}{}
	for hash, tracker := range s.trackers {
		hex := hex.EncodeToString(hash[:])
		trackers[hex] = tracker.Stat()
	}
	stats["trackers"] = trackers

	var swarms []map[string]interface{}
	for _, swarm := range s.swarms {
		swarms = append(swarms, swarm.Stat())
	}
	stats["swarms"] = swarms

	return stats
}

func (s *Session) Init() (func() error, error) {
	var op errors.Op = "(*Client).Init"

	log.Info().
		Str("op", op.String()).
		Msg("Initializing client")

	s.startedAt = time.Now()

	port, err := forwardPorts(append(s.preferredPorts, DEFAULTPORTS...))
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	s.port = port

	dir := path.Join(s.baseDir, "torrents")
	torrents, err := btorrent.LoadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	log.Printf("Loaded %d torrents from %s\n", len(torrents), dir)
	for _, t := range torrents {
		s.torrents[t.InfoHash()] = *t
		tr := tracker.New(*t, s.port, s.peerID)

		s.trackers[t.InfoHash()] = tr

		swarm := swarm.New(*t, s.swarmCh, s.peerID, path.Join(s.baseDir, s.downloadDir))
		err := swarm.Init()
		if err != nil {
			log.Print(err.Error())
			delete(s.trackers, t.InfoHash())
			// indicate torrent error
			continue
		}

		s.swarms[t.InfoHash()] = &swarm
	}

	go s.announce()
	go s.listen()

	return func() error { return nil }, nil
}

func (s *Session) announce() {
	for {
		go func() {
			for hash, tr := range s.trackers {
				var state = s.swarms[hash].Stat()
				var (
					torrent = s.torrents[hash]
					swarm   = s.swarms[hash]

					have  = state["have"].(int)
					stats = state["stats"].(map[string]interface{})

					downloaded = stats["downloaded"].(uint64)
					uploaded   = stats["uploaded"].(uint64)
					left       = uint64(len(torrent.Pieces())-have) * torrent.PieceLength()
				)

				req := tracker.Request{
					Hash:       hash,
					Port:       s.port,
					PeerID:     s.peerID,
					Left:       left,
					Downloaded: downloaded,
					Uploaded:   uploaded,
					Want:       -1,
				}

				res := tr.Announce(req)
				swarm.AnnounceCh <- res
			}
		}()

		time.Sleep(5 * time.Second)
	}
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

	// Distribute connections among the swarms
	go func() {
		for {
			event := <-s.swarmCh
			switch event.(type) {
			case swarm.PeerRequest:
				// Handle Peer Requests
			}
		}
	}()

	for {
		conn, err := listener.Accept()

		if err != nil {
			err = errors.Wrap(err)
			log.Err(err).Strs("trace", errors.Ops(err)).Msg(err.Error())
			continue
		}

		go func() {
			err := s.AcceptHandshake(conn)
			if err != nil {
				log.Err(err).Msgf("Handshake failed: %s", conn.RemoteAddr())
				conn.Close()
			}
		}()

	}
}

func (s *Session) InitiateHandshake(conn net.Conn, infoHash [20]byte) error {
	var op errors.Op = "(*Messenger).Listen.GoRoutine.GoRoutine"
	var msg btorrent.HandshakeMessage

	log.Printf("Initiating handshake: %s", conn.RemoteAddr())

	err := Handshake(conn, infoHash, s.peerID)
	if err != nil {
		err = errors.Wrap(err, op)
		log.Error().Msg(err.Error())
		return err
	}

	err = btorrent.UnmarshalHandshake(conn, &msg)
	if err != nil {
		err = errors.Wrap(err, op)
		log.Err(err).Strs("trace", errors.Ops(err)).Msg("Bad handshake")
		return err
	}

	err = s.VerifyHandshake(msg)
	if err != nil {
		return err
	}

	log.Printf("Handshake complete: %s", conn.RemoteAddr())

	s.swarms[msg.InfoHash].PeerCh <- conn
	return nil
}

func (s *Session) AcceptHandshake(conn net.Conn) error {
	var op errors.Op = "(*Messenger).Listen.GoRoutine.GoRoutine"
	var msg btorrent.HandshakeMessage

	err := btorrent.UnmarshalHandshake(conn, &msg)
	if err != nil {
		err = errors.Wrap(err, op)
		log.Err(err).Strs("trace", errors.Ops(err)).Msg("Bad handshake")
		return err
	}

	err = s.VerifyHandshake(msg)
	if err != nil {
		return err
	}

	err = Handshake(conn, msg.InfoHash, s.peerID)
	if err != nil {
		err = errors.Wrap(err, op)
		log.Error().Msg(err.Error())
		return err
	}

	log.Printf("Handshake complete: %s", conn.RemoteAddr())

	if s.swarms[msg.InfoHash].Done() {
		conn.Close()
		return nil
	}

	s.swarms[msg.InfoHash].PeerCh <- conn

	return nil
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
		swarmCh:        make(chan swarm.Event, 32),
	}

	if cfg.IP == "" {
		c.ip = "127.0.0.1"
	} else {
		c.ip = cfg.IP
	}

	return c
}
