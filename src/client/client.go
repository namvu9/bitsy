package client

import (
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/gateway"
	"github.com/namvu9/bitsy/src/messaging"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
	"github.com/rs/zerolog/log"
	"gitlab.com/NebulousLabs/go-upnp"
)

// TODO: Move listener here.
// TODO: Call Announce from here
// TODO: Swarms do not connect to peers, we do it here
// instead and send the resulting peer connection to the
// appropriate swarm
// TODO: Single session main loop, only Session can modify
// itself or call any methods that might modify it. There
// should be no need for locks

// TorrentID is the SHA-1 hash of the torrent's bencoded
// info dictionary
type TorrentID [20]byte

type Trackers []tracker.TrackerStat

// Revert to session
// Session represents an instance of the client and manages
// the client's state.
type Session struct {
	// Stats
	startedAt time.Time
	conns     int

	// state
	swarms   map[TorrentID]*messaging.Swarm
	torrents map[TorrentID]torrent.Torrent
	trackers map[TorrentID][]Trackers

	// Config
	peerID      [20]byte
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections

	// TODO: REMOVE
	preferredPorts []uint16
	m              *messaging.Messenger
}

type upnpRes struct {
	externalIP string
	port       int
}

func (c *Session) Stat() []string {
	var sb strings.Builder

	sb.WriteString("\n-----\nClient Stats\n-----\n")
	sb.WriteString(fmt.Sprintf("Session Length: %s\n", time.Now().Sub(c.startedAt)))

	var out []string
	out = append(out, sb.String())
	out = append(out, c.m.Stat()...)
	return out
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

// TODO: Break up this function a bit
func (c *Session) Init() (func() error, error) {
	var op errors.Op = "(*Client).Init"

	log.Info().
		Str("op", op.String()).
		Msg("Initializing client")

	c.startedAt = time.Now()

	port, err := forwardPorts(append(c.preferredPorts, DEFAULTPORTS...))
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	c.port = port

	err = c.loadTorrents()
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	// TODO: Remove the gateways
	var (
		trackerGateway = gateway.New(c.maxConns)
		swarmGateway = gateway.New(c.maxConns)
		m            = messaging.New(c.ip, c.port, &swarmGateway, c.peerID, c.downloadDir)
	)

	err = m.Listen()
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	c.m = m

	for _, torrent := range c.torrents {
		tr := tracker.New(torrent, &trackerGateway, c.port, c.peerID)
		stat, _ := tr.Listen()

		m.AddSwarm(&torrent, stat)
	}

	var (
		stopCh    = make(chan struct{})
		stopErrCh = make(chan error)
	)

	go func() {
		<-stopCh
		log.Info().Msg("Client shutting down...")

		// Cleanup code
		// ...

		stopErrCh <- nil
	}()

	return func() error {
		stopCh <- struct{}{}
		return <-stopErrCh
	}, nil
}

// NewTorrent loads a torrent into the current session and
// writes it to disk
func (c *Session) NewTorrent(location string) error {
	var op errors.Op = "(*Client).NewTorrent"

	t, err := c.LoadTorrent(location)
	if err != nil {
		return errors.Wrap(err, op)
	}

	filePath := fmt.Sprintf("%s.torrent", path.Join(c.baseDir, t.Name()))
	log.Printf("Wrote new torrent %s to %s\n", t.Name(), filePath)

	err = torrent.Save(filePath, t)
	if err != nil {
		return errors.Wrap(err, op)
	}

	return nil
}

// LoadTorrent loads a torrent either from a torrent file on
// disk or a magnet link
func (c *Session) LoadTorrent(location string) (*torrent.Torrent, error) {
	var op errors.Op = "(*Client).LoadTorrent"

	t, err := torrent.Load(location)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	log.Printf("Loaded %s", t.Name())

	c.torrents[t.InfoHash()] = *t

	return t, nil
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
		torrents:       make(map[TorrentID]torrent.Torrent),
	}

	if cfg.IP == "" {
		c.ip = "127.0.0.1"
	} else {
		c.ip = cfg.IP
	}

	return c
}

func (c *Session) loadTorrents() error {
	var op errors.Op = "(*Client).loadTorrents"

	files, err := ioutil.ReadDir(c.baseDir)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	var errs errors.Errors
	for _, file := range files {
		name := file.Name()
		match, err := regexp.MatchString(`.+\.torrent`, name)
		if err != nil {
			errs = append(errs, errors.Wrap(err, op))
			continue
		}

		if match {
			_, err := c.LoadTorrent(path.Join(c.baseDir, name))
			if err != nil {
				errs = append(errs, errors.Wrap(err, op))
				continue
			}
		}
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

// TODO: Announce from session
// Send results to s.peerCh
//case announce := <-s.announceCh:
//s.leechers = int(announce.NLeechers)
//s.seeders = int(announce.NSeeders)

//var peers []tracker.PeerInfo
//for _, peer := range announce.Peers {
//if peer.IP.IsUnspecified() {
//continue
//}

//peers = append(peers, peer)
//}

//s.peerInfo = append(s.peerInfo, peers...)
// TODO: Do this at session-level
// TODO: Connect to multiple new peers
//func (s *Swarm) connectNewPeer() error {
//var op errors.Op = "(*Swarm).connectNewPeer"

//if len(s.peerInfo) > 0 {
//peerInfo := s.peerInfo[0]
//s.peerInfo = s.peerInfo[1:]

//addr := fmt.Sprintf("%s:%d", peerInfo.IP, peerInfo.Port)

//log.Info().Str("op", op.String()).Msgf("Initiating connection to %s\n", addr)

//conn, err := s.d.Dial("tcp", addr)
//if err != nil {
//return errors.Wrap(err, op, errors.Network)
//}

//peerConn, err := Handshake(conn, s.InfoHash(), s.peerID)
//if err != nil {
//return errors.Wrap(err, op, errors.Network)
//}

//s.peerCh <- peerConn

//log.Info().Str("op", op.String()).Msgf("Connected to %s\n", peerConn.RemoteAddr())
//}

//return nil
//}
