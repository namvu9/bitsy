package session

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
	"github.com/rs/zerolog/log"
)

var PeerID = [20]byte{'-', 'B', 'T', '0', '0', '0', '0', '-'}
var Reserved = peer.NewExtensionsField([8]byte{})

const PStr = "BitTorrent protocol"

func init() {
	rand.Read(PeerID[8:])

	err := Reserved.Enable(peer.EXT_PROT)
	if err != nil {
		panic(err)
	}
}

// TorrentID is the SHA-1 hash of the torrent's bencoded
// info dictionary
type TorrentID [20]byte

// Session represents an instance of the client and manages
// the client's state.
type Session struct {
	startedAt time.Time

	torrent  btorrent.Torrent
	trackers []*tracker.TrackerGroup
	swarm    *swarm.Swarm
	client   *Client

	// Config
	peerID      [20]byte
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections
	msgIn       chan swarm.Event
}

type Event interface{}

func pipe(ctx context.Context, in, out chan swarm.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-in:
				out <- ev
			}
		}
	}()
}

func clear() {
	cmd := exec.Command("clear") //Linux example, its tested
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func Spread(ctx context.Context, in chan swarm.Event, out ...chan swarm.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-in:
				for _, outCh := range out {
					select {
					case outCh <- ev:
					default:
					}
				}
			}
		}
	}()
}

func (s *Session) Init() (func() error, error) {
	s.startedAt = time.Now()
	go s.client.Start()
	go s.listen()

	statCh := make(chan swarm.Event, 32)

	go func() {
		start := time.Now()
		for {
			msg := <-statCh
			var sb strings.Builder

			fmt.Fprintf(&sb, "--------\n%s\n-------\n", s.torrent.Name())
			fmt.Fprintf(&sb, "Session Length: %s\n", time.Now().Sub(start))
			fmt.Fprintf(&sb, "Info Hash: %s\n", s.torrent.HexHash())

			if v, ok := msg.(ClientStat); ok {
				fmt.Fprint(&sb, v)
			} else {
				fmt.Fprint(&sb, s.client.Stat())
			}
			fmt.Fprintf(&sb, "Peers: %d\n", s.swarm.Stat().Peers)
			fmt.Fprint(&sb, "--------")
			clear()
			fmt.Println(sb.String())
		}
	}()

	ctx := context.Background()

	Spread(ctx, s.swarm.OutCh, s.client.swarmCh, statCh)
	Spread(ctx, s.client.StateCh, statCh)

	pipe(ctx, s.client.msgOut, s.swarm.EventCh)

	return func() error { return nil }, nil
}

func (s *Session) listen() error {
	var (
		addr = fmt.Sprintf("%s:%d", s.ip, s.port)
		bn   = NewBoundedNet(30)
	)

	listener, err := bn.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			err = errors.Wrap(err)
			fmt.Println(err)
			continue
		}

		err = s.acceptHandshake(conn)
		if err != nil {
			log.Err(err).Msgf("Handshake failed: %s", conn.RemoteAddr())
			conn.Close()
		}
	}
}

func (s *Session) acceptHandshake(conn net.Conn) error {
	p := peer.New(conn)
	err := p.Init()
	if err != nil {
		return err
	}

	refhash := s.torrent.InfoHash()
	if !bytes.Equal(p.InfoHash[:], refhash[:]) {
		err := fmt.Errorf("Unknown info hash: %x %x", p.InfoHash, refhash)
		return err
	}

	err = Handshake(p, s.torrent.InfoHash(), s.peerID)
	if err != nil {
		return err
	}

	s.swarm.PeerCh <- p
	return nil
}

func New(cfg Config, t btorrent.Torrent) *Session {
	var trackers []*tracker.TrackerGroup

	for _, tier := range t.AnnounceList() {
		trackers = append(trackers, tracker.NewGroup(tier))
	}

	dialCfg := peer.DialConfig{
		InfoHash:   t.InfoHash(),
		Timeout:    500 * time.Millisecond,
		PeerID:     PeerID,
		Extensions: Reserved,
		PStr:       PStr,
	}
	msgIn := make(chan swarm.Event, 32)
	swarm := swarm.New(t, msgIn, trackers, dialCfg)
	go swarm.Init()

	c := &Session{
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		maxConns:    cfg.MaxConnections,
		peerID:      PeerID,
		port:        cfg.Port,

		torrent:  t,
		trackers: trackers,

		ip:    cfg.IP,
		msgIn: msgIn,
		client: NewClient(t, ClientConfig{
			InitState: STOPPED,
			BaseDir:   cfg.DownloadDir,
			OutDir:    cfg.DownloadDir,
			MaxPeers:  50,
		}),
		swarm: &swarm,
	}

	return c
}
