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

// Client represents an instance of the client and manages
// the client's state.
type Client struct {
	peerID   [20]byte
	torrents []torrent.Torrent

	// Config
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections

	preferredPorts []uint16

	m         *messaging.Messenger
	startedAt time.Time
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

func (c *Client) Stat() []string {
	var sb strings.Builder

	sb.WriteString("\n-----\nClient Stats\n-----\n")
	sb.WriteString(fmt.Sprintf("Session Length: %s\n", time.Now().Sub(c.startedAt)))

	var out []string
	out = append(out, sb.String())
	out = append(out, c.m.Stat()...)
	return out
}

// Init initializes the client by starting its event loop
// TODO: Break up this function a bit
// TODO: Not really an event loop, though is it?
func (c *Client) Init() (func() error, error) {
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

	var (
		trackerGateway = gateway.New(c.maxConns)
		swarmGateway   = gateway.New(c.maxConns)
		m              = messaging.New(c.ip, c.port, &swarmGateway, c.peerID, c.downloadDir)
	)

	err = m.Listen()
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	c.m = m

	for _, torrent := range c.torrents {
		tr := tracker.New(torrent, &trackerGateway, c.port, c.peerID)
		out, in := tr.Listen()

		m.AddSwarm(&torrent, out, in)
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
func (c *Client) NewTorrent(location string) error {
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
func (c *Client) LoadTorrent(location string) (*torrent.Torrent, error) {
	var op errors.Op = "(*Client).LoadTorrent"

	t, err := torrent.Load(location)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	log.Printf("Loaded %s", t.Name())

	// The torrent cannot be identified without an info hash
	hash := t.InfoHash()
	if hash == nil {
		err := fmt.Errorf("torrent %s has no info hash", t.Name())
		return nil, errors.Wrap(err, op)
	}

	c.torrents = append(c.torrents, *t)

	return t, nil
}

func New(cfg Config) *Client {
	// TODO: Generate a proper peerID. This is the peerID used
	// by the 'Transmission' BitTorrent client
	peerID := [20]byte{0x2d, 0x54, 0x52, 0x33, 0x30, 0x30, 0x30, 0x2d, 0x6c, 0x62, 0x76, 0x30, 0x65, 0x65, 0x75, 0x35, 0x34, 0x31, 0x36, 0x37}

	c := &Client{
		baseDir:        cfg.BaseDir,
		downloadDir:    cfg.DownloadDir,
		maxConns:       cfg.MaxConnections,
		peerID:         peerID,
		preferredPorts: cfg.Port,
	}

	if cfg.IP == "" {
		c.ip = "127.0.0.1"
	} else {
		c.ip = cfg.IP
	}

	return c
}

func (c *Client) loadTorrents() error {
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
