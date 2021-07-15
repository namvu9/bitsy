package session

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"sync"
	"time"

	"github.com/namvu9/bitsy/src/peer"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
)

type contextKey string

// Session represents an instance of the client and manages
// the client's state.
type Session struct {
	stats map[[20]byte]*TorrentState
	conns []*peer.Conn // Open connections
	peers []tracker.PeerInfo

	// Handle newly discovered peers
	announceCh chan *tracker.AnnounceResp

	peerID [20]byte

	// Config
	baseDir     string
	downloadDir string
	ip          string // The ip address to listen on
	port        uint16 // the port to listen on
	maxConns    int    // Max open peer connections

	stopCh chan struct{}
}

func (s *Session) Init() error {
	err := s.loadTorrents()
	if err != nil {
		return err
	}

	ticker := time.NewTicker(10 * time.Second)
	go s.tick(ticker)
	go s.listen()

	return nil
}

// NewTorrent loads a torrent into the current session and
// writes it to disk
func (s *Session) NewTorrent(location string) error {
	t, err := s.LoadTorrent(location)
	if err != nil {
		return err
	}

	filePath := fmt.Sprintf("%s.torrent", path.Join(s.baseDir, t.Name()))
	log.Printf("Wrote new torrent %s to %s\n", t.Name(), filePath)

	return torrent.Save(filePath, t)
}

// LoadTorrent loads a torrent either from a torrent file on
// disk or a magnet link
func (s *Session) LoadTorrent(location string) (*torrent.Torrent, error) {
	t, err := torrent.Load(location)
	if err != nil {
		return nil, err
	}

	hash := t.InfoHash()
	if hash == nil {
		return nil, fmt.Errorf("Torrent %s has no info hash\n", t.Name())
	}

	var trackers []tracker.Tracker

	for _, tier := range t.AnnounceList() {
		for _, tr := range tier {
			url, _ := url.Parse(tr)
			tracker, err := tracker.New(url)
			if err == nil {
				trackers = append(trackers, tracker)
			}
		}
	}

	s.stats[*hash] = &TorrentState{
		Location: location,
		Torrent:  t,
		Status:   STARTED,
		Trackers: trackers,
	}

	return t, nil
}

func (s *Session) Stat() []*TorrentState {
	var out []*TorrentState
	for _, t := range s.stats {
		out = append(out, t)
	}

	return out
}

// HasInfoHash reports whether the client is interested in
// the torrent identified by the given Info hash
func (s *Session) HasInfoHash(hash [20]byte) bool {
	_, ok := s.stats[hash]
	return ok
}

type Config struct {
	BaseDir        string
	DownloadDir    string
	IP             string
	MaxConnections int
	NatPMP         bool
	Port           int
}

func New(cfg Config) *Session {
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		log.Println("Failed to generate random peer ID")
	}

	return &Session{
		baseDir:     cfg.BaseDir,
		downloadDir: cfg.DownloadDir,
		ip:          cfg.IP,
		maxConns:    cfg.MaxConnections,
		port:        uint16(cfg.Port),

		peerID:     peerID,
		stats:      make(map[[20]byte]*TorrentState),
		announceCh: make(chan *tracker.AnnounceResp, 30),
		stopCh:     make(chan struct{}),
	}
}

// Teardown/Cleanup
func (s *Session) Terminate() error {
	log.Println("Shutting down...")

	//s.stopCh <- struct{}{}
	for _, c := range s.conns {
		err := c.Close()
		if err != nil {
			return err
		}
	}

	log.Println("Shutting down... Done")
	os.Exit(0)
	return nil
}

// Resume existing torrents
func (s *Session) loadTorrents() error {
	files, err := ioutil.ReadDir(s.baseDir)
	if err != nil {
		return err
	}

	var count int = len(s.stats)
	for _, file := range files {
		name := file.Name()
		match, err := regexp.MatchString(`.+\.torrent`, name)
		if err != nil {
			return err
		}

		if match {
			_, err := s.LoadTorrent(path.Join(s.baseDir, name))
			if err != nil {
				log.Println(err)
			}
			count++
		}

	}

	log.Printf("Loaded %d torrents: \n", count)

	return nil
}

func (s *Session) discoverPeers(ctx context.Context) {
	for hash, torrent := range s.stats {

		for _, tr := range torrent.Trackers {

			go func(tr tracker.Tracker) {
				log.Printf("Announce (%s): asking %s for more peers\n", torrent.HexHash(), tr.Host())
				if !tr.ShouldAnnounce() {
					return
				}

				res, err := tr.Announce(tracker.AnnounceRequest{
					Hash:       hash,
					PeerID:     s.peerID,
					Downloaded: torrent.Downloaded,
					Left:       torrent.Left,
					Uploaded:   torrent.Uploaded,
					Want:       -1, // Default
					Port:       s.port,
				})

				if err != nil {
					log.Printf("Announce (%s): Error: Tracker %s; %s\n", torrent.HexHash(), tr.Host(), err)
					return
				}

				log.Printf("Discovered %d peers for %s\n", len(res.Peers), torrent.Name())
				s.announceCh <- res

			}(tr)
		}

	}

}

func (s *Session) tick(t *time.Ticker) {
	ctx, cancel := context.WithCancel(context.Background())

	for {
		select {
		case <-s.stopCh:
			cancel()
			t.Stop()
		case tick := <-t.C:
			tickCtx := context.WithValue(ctx, contextKey("timestamp"), tick)
			go s.discoverPeers(tickCtx)
			// TODO: Move this into its own function
		case announceResp := <-s.announceCh:
			hash := announceResp.InfoHash

			var wg sync.WaitGroup
			for _, p := range announceResp.Peers {
				s.peers = append(s.peers, p)
				wg.Add(1)
				go func(p tracker.PeerInfo) {
					defer wg.Done()

					conn, err := net.DialTimeout("tcp", p.String(), 2*time.Second)
					if err != nil {
						log.Println(err)
						return
					}

					peerConn, err := peer.Handshake(conn, p.String(), hash, s.peerID)
					if err != nil {
						log.Println(err)
						return
					}

					s.conns = append(s.conns, peerConn)

				}(p)
			}

			wg.Wait()
		}
	}
}

// Listen for incoming connections
func (s *Session) listen() {
	addr := fmt.Sprintf("%s:%d", s.ip, s.port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Println(err)
		s.Terminate()
		os.Exit(1)
	}

	log.Printf("Listening on %s\n", addr)

	for {
		log.Printf("Current open connections: %d\n", len(s.conns))

		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept incoming (%s): %s\n", addr, err)
			conn.Close()
			continue
		}

		if len(s.conns) < s.maxConns {
			peerConn, err := peer.RespondHandshake(context.Background(), conn, s.HasInfoHash, s.peerID)
			if err != nil {
				log.Println(err)
			}

			s.conns = append(s.conns, peerConn)
		}

	}
}
