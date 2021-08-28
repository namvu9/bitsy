package session

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/namvu9/bitsy/internal/session/client"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
	"github.com/namvu9/bitsy/pkg/ch"
)

var PeerID = [20]byte{'-', 'B', 'T', '0', '0', '0', '0', '-'}
var Reserved = peer.NewExtensionsField([8]byte{})

const MAX_CONNS = 30

const PStr = "BitTorrent protocol"

func init() {
	rand.Read(PeerID[8:])

	err := Reserved.Enable(peer.EXT_PROT)
	if err != nil {
		panic(err)
	}

	err = Reserved.Enable(peer.EXT_FAST)
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
	client   *client.Client

	// Config
	peerID      [20]byte
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections
	msgIn       chan interface{}
}

type Event interface{}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func fmtDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (s *Session) stat(statCh <-chan interface{}) {
	start := time.Now()
	var stat client.ClientStat
	for {
		var sb strings.Builder
		sessLen := time.Now().Sub(start)

		fmt.Fprintf(&sb, "--------\n%s\n-------\n", s.torrent.Name())
		fmt.Fprintf(&sb, "Goroutines: %d\n", runtime.NumGoroutine())
		fmt.Fprintf(&sb, "Session Length: %s\n", fmtDuration(sessLen))
		fmt.Fprintf(&sb, "Info Hash: %s\n", s.torrent.HexHash())
		select {
		case msg := <-statCh:
			if v, ok := msg.(client.ClientStat); ok {
				stat = v
			}
		default:
			stat = s.client.Stat()
		}

		fmt.Fprint(&sb, stat)
		fmt.Fprint(&sb, s.swarm.Stat())
		fmt.Fprint(&sb, "--------")
		clear()
		fmt.Println(sb.String())
		time.Sleep(time.Second)
	}
}

func (s *Session) Init() (func() error, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s.startedAt = time.Now()

	conn := NewConnectionService(s.port, s.peerID, func(i interface{}) {
		switch v := i.(type) {
		case swarm.JoinEvent:
			s.swarm.EventCh <- v
		}
	}, MAX_CONNS)
	conn.Register(s.torrent)
	err := conn.Init(ctx)
	if err != nil {
		panic(err)
	}

	go s.swarm.Init(ctx, s.port)
	go s.client.Start(ctx)

	statCh := make(chan interface{}, 32)
	go s.stat(statCh)

	ch.Spread(ctx, s.swarm.OutCh, s.client.SwarmCh, statCh)
	ch.Spread(ctx, s.client.StateCh, statCh)
	ch.Pipe(ctx, s.client.MsgOut, s.swarm.EventCh)

	return func() error {
		cancel()
		return nil
	}, nil
}

func New(cfg Config, t btorrent.Torrent) *Session {
	var trackers []*tracker.TrackerGroup

	for _, tier := range t.AnnounceList() {
		trackers = append(trackers, tracker.NewGroup(tier))
	}

	var (
		dialCfg = peer.DialConfig{
			InfoHash:   t.InfoHash(),
			Timeout:    500 * time.Millisecond,
			PeerID:     PeerID,
			Extensions: Reserved,
			PStr:       PStr,
		}
		msgIn = make(chan interface{}, 32)
		swarm = swarm.New(t, msgIn, trackers, dialCfg)
	)

	opts := []client.Option{}
	if len(cfg.Files) > 0 {
		opts = append(opts, client.WithFiles(cfg.Files...))
	}

	c := &Session{
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		maxConns:    cfg.MaxConnections,
		peerID:      PeerID,
		port:        cfg.Port,

		torrent:  t,
		trackers: trackers,

		ip:     cfg.IP,
		msgIn:  msgIn,
		client: client.New(t, cfg.BaseDir, cfg.DownloadDir, opts...),
		swarm:  &swarm,
	}

	return c
}
