package client

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/namvu9/bitsy/pkg/ch"
)

type ClientState int

const MAX_PENDING_PIECES = 100

const (
	STOPPED ClientState = iota
	STARTING
	DOWNLOADING
	DONE
	FETCHING_META
	SEEDING
	ERROR
)

func (cs ClientState) String() string {
	switch cs {
	case DOWNLOADING:
		return "Downloading"
	case ERROR:
		return "Error"
	case DONE:
		return "Done"
	case SEEDING:
		return "Seeding"
	case STARTING:
		return "Starting"
	case STOPPED:
		return "Stopped"
	default:
		return ""
	}
}

// Client represents the client in, and encapsulates
// interactions with, a swarm for a particular torrent
// the client is responsible for peer and piece selection
// strategy
type Client struct {
	state   ClientState
	baseDir string
	outDir  string

	// StateCh emits an event whenever the state of the client
	// changes
	StateCh chan interface{}
	emitCh  chan interface{}

	// SwarmCh
	pieces        bits.BitField
	ignoredPieces bits.BitField
	filesWritten  map[string]bool
	torrent       btorrent.Torrent

	// Out
	doneCh  chan struct{}
	SwarmCh chan interface{}
	MsgOut  chan interface{}

	MsgIn  chan messageReceived
	DataIn chan peer.PieceMessage

	Pending      int
	DownloadRate btorrent.Size
	Uploaded     int

	workers    map[int]*worker
	workerLock sync.RWMutex
}

func (c *Client) Stop() {
	c.doneCh <- struct{}{}
	c.emit(StateChange{To: STOPPED})
}

func (c *Client) Start(ctx context.Context) error {
	c.emit(StateChange{To: STARTING})

	if err := c.verifyPieces(); err != nil {
		c.emit(StateChange{To: ERROR, Msg: err.Error()})
		return err
	}

	for _, file := range c.torrent.Files() {
		filePath := path.Join(c.outDir, c.torrent.Name(), file.Name)
		if _, err := os.Stat(filePath); err == nil {
			c.filesWritten[file.Name] = true
		}
	}
	c.assembleTorrent(path.Join(c.outDir, c.torrent.Name()))

	ch.PipeFn(ctx, c.handleEvent, c.emitCh, c.StateCh)
	go c.listen()

	if !c.done() {
		go c.download()
		c.emit(StateChange{To: DOWNLOADING})
	} else {
		c.emit(StateChange{To: SEEDING})
	}

	return nil
}

// TODO: TEST
// This sometimes blocks the program...
func (c *Client) nextNPieces(n int, exclude map[int]*worker) []int {
	var remaining []int
	c.workerLock.RLock()
	defer c.workerLock.RUnlock()

	for i := range c.torrent.Pieces() {
		if c.pieces.Get(i) {
			continue
		}

		if c.ignoredPieces.Get(i) {
			continue
		}

		if exclude[i] != nil {
			continue
		}

		remaining = append(remaining, i)
	}
	count := min(n, len(remaining))
	var out []int

	for len(out) < count {
		i := int(rand.Int31n(int32(count)))
		out = append(out, remaining[i])
	}

	return out
}

func (c *Client) download() {
	var (
		ticker = time.NewTicker(2 * time.Second)
	)

	c.downloadN(5)

	var (
		downloadRate = 0.0
		batch        = 0
	)

	for {
		select {
		case msg := <-c.DataIn:
			done, err := c.handlePieceMessage(msg, c.workers)
			if err != nil {
				continue
			}

			if !done {
				batch += len(msg.Piece)
				continue
			}

			if done {
				c.emit(DownloadCompleteEvent{
					Index:        int(msg.Index),
					DownloadRate: btorrent.Size(downloadRate),
					Pending:      len(c.workers),
				})
				c.workerLock.Lock()
				delete(c.workers, int(msg.Index))
				c.workerLock.Unlock()
			}
			c.assembleTorrent(path.Join(c.outDir, c.torrent.Name()))

			if c.done() {
				c.emit(StateChange{To: SEEDING})
				return
			}

			if len(c.workers) < MAX_PENDING_PIECES {
				c.downloadN(1)
			}

		case <-ticker.C:
			go c.unchoke()
			go c.choke()

			downloadRate = float64(batch) / 2.0

			go c.emit(DownloadCompleteEvent{
				Index:        int(-1),
				DownloadRate: btorrent.Size(downloadRate),
				Pending:      len(c.workers),
			})
			batch = 0

			idleCount := 0
			for _, w := range c.workers {
				if w.idle() {
					idleCount++
					go w.restart()
				}
			}

			if idleCount > 0 && len(c.workers) < MAX_PENDING_PIECES {
				c.downloadN(1)
			}
		}
	}
}

func (c *Client) anyFileDone() bool {
	for i := range c.torrent.Files() {
		if c.fileDone(i) {
			return true
		}
	}

	return false
}

func (c *Client) fileDone(idx int) bool {
	file := c.torrent.Files()[idx]

	for _, piece := range file.Pieces {
		pcIdx := c.torrent.GetPieceIndex(piece)
		if pcIdx < 0 {
			return false
		}

		if !c.pieces.Get(pcIdx) {
			return false
		}
	}

	return true
}

func (c *Client) done() bool {
	return c.pieces.GetSum() == len(c.torrent.Pieces())
}

func (c *Client) downloadN(n int) {
	for _, pieceIdx := range c.nextNPieces(n, c.workers) {
		go c.downloadPiece(uint32(pieceIdx), false)
		go c.emit(DownloadEvent{
			Pending: len(c.workers),
		})
	}
}

func (c *Client) unchoke() {
	// Optimistic unchoke
	c.MsgOut <- swarm.MulticastMessage{
		Limit: 2,
		Filter: func(p *peer.Peer) bool {
			return p.Choked
		},
		Handler: func(p []*peer.Peer) {
			if len(p) == 0 {
				return
			}
			last := p[len(p)-1]
			last.Send(peer.UnchokeMessage{})
		},
	}
}

func (c *Client) choke() {
	// Choke the "worst" peer
	c.MsgOut <- swarm.MulticastMessage{
		OrderBy: func(p1, p2 *peer.Peer) int {
			// Rank peers by upload rate
			return int(p2.UploadRate - p1.UploadRate)
		},
		Limit: 5,
		Filter: func(p *peer.Peer) bool {
			return !p.Choked
		},
		Handler: func(p []*peer.Peer) {
			if len(p) == 0 {
				return
			}
			last := p[len(p)-1]
			last.Send(peer.ChokeMessage{})
		},
	}

}

func (c *Client) listen() {
	for {
		select {
		case <-c.doneCh:
			return
		case msg := <-c.SwarmCh:
			switch v := msg.(type) {
			case swarm.JoinEvent:
				p := v.Peer
				var haveMsg peer.Message
				var haveN = c.pieces.GetSum()

				if p.Extensions.IsEnabled(peer.EXT_FAST) && haveN == len(c.torrent.Pieces()) {
					haveMsg = peer.HaveAllMessage{}
				} else if p.Extensions.IsEnabled(peer.EXT_FAST) && haveN == 0 {
					haveMsg = peer.HaveNoneMessage{}
				} else {
					haveMsg = peer.BitFieldMessage{BitField: c.pieces}
				}
				go p.Send(haveMsg)
				addr := p.RemoteAddr().(*net.TCPAddr)

				if p.Extensions.IsEnabled(peer.EXT_FAST) {
					for _, pieceIdx := range c.torrent.GenFastSet(addr.IP, 10) {
						go p.Send(peer.AllowedFastMessage{Index: uint32(pieceIdx)})
					}
				}

				c.subscribe(p)
			}
		case msg := <-c.MsgIn:
			c.handleMessage(msg)
		}
	}
}

type ClientEvent interface{}
type StateChange struct {
	To  ClientState
	Msg string
}

type DownloadEvent struct {
	Pending int
}

type UploadEvent struct{}

type DownloadCompleteEvent struct {
	Index        int
	DownloadRate btorrent.Size
	Pending      int
}

func (c *Client) handleEvent(e interface{}) interface{} {
	switch v := e.(type) {
	case StateChange:
		c.state = v.To
	case DownloadCompleteEvent:
		c.DownloadRate = v.DownloadRate
		c.Pending = v.Pending
	case DownloadEvent:
		c.Pending = v.Pending
	}

	return c.Stat()
}

func (c *Client) emit(e ClientEvent) {
	c.emitCh <- e
}

// downloadPiece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
func (c *Client) downloadPiece(index uint32, fast bool) *worker {
	c.workerLock.Lock()
	defer c.workerLock.Unlock()

	if _, ok := c.workers[int(index)]; ok || c.ignoredPieces.Get(int(index)) {
		return nil
	}

	var (
		t           = c.torrent
		size        = t.Length()
		pieceLength = t.PieceLength()
	)

	if int(index) == len(t.Pieces())-1 {
		pieceLength = size % pieceLength
	}

	w := &worker{
		Started:      time.Now(),
		LastModified: time.Now(),

		timeout: 5 * time.Minute,
		stop:    make(chan struct{}, 1),
		in:      make(chan peer.PieceMessage, 32),

		index:       index,
		hash:        t.Pieces()[index],
		pieceLength: uint64(pieceLength),
		subpieces:   map[uint32][]byte{},
		torrent:     c.torrent,
		baseDir:     c.baseDir,

		out:  c.MsgOut,
		fast: fast,
	}

	w.run()

	c.workers[int(index)] = w

	return w
}

// Verify which pieces the client has
func (c *Client) verifyPieces() error {
	var (
		hexHash    = c.torrent.HexHash()
		torrentDir = path.Join(c.baseDir, hexHash)
	)

	err := os.MkdirAll(torrentDir, 0777)
	if err != nil {
		return err
	}

	files, err := os.ReadDir(torrentDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		var (
			matchString = fmt.Sprintf(`(\d+).part`)
			re          = regexp.MustCompile(matchString)
			name        = strings.ToLower(file.Name())
		)

		if re.MatchString(name) {
			var (
				matches = re.FindStringSubmatch(name)
				index   = matches[1]
			)
			n, err := strconv.Atoi(index)
			if err != nil {
				return err
			}

			piecePath := path.Join(torrentDir, file.Name())
			if ok := c.loadAndVerify(n, piecePath); ok {
				c.pieces.Set(n)
			}
		}
	}

	return nil
}

// Load a piece and verify its contents
func (c *Client) loadAndVerify(index int, location string) bool {
	piece, err := os.ReadFile(location)
	if err != nil {
		return false
	}

	if !c.torrent.VerifyPiece(index, piece) {
		return false
	}

	return true
}

type Option func(*Client)

func WithFiles(fileIdx ...int) Option {
	return func(c *Client) {
		var (
			bf    = bits.Ones(len(c.torrent.Pieces()))
			files = c.torrent.Files()
		)

		for _, i := range fileIdx {
			if i >= len(files) {
				panic("out of bounds")
			}

			file := files[i]
			for _, piece := range file.Pieces {
				if pcIdx := c.torrent.GetPieceIndex(piece); pcIdx >= 0 {
					bf.Unset(pcIdx)
				}
			}
		}

		c.ignoredPieces = bf
	}
}

func New(t btorrent.Torrent, baseDir, outDir string, options ...Option) *Client {
	nPieces := len(t.Pieces())
	c := &Client{
		baseDir:       baseDir,
		outDir:        outDir,
		state:         STOPPED,
		StateCh:       make(chan interface{}),
		emitCh:        make(chan interface{}, 32),
		MsgIn:         make(chan messageReceived, 32),
		DataIn:        make(chan peer.PieceMessage, 32),
		MsgOut:        make(chan interface{}, 32),
		SwarmCh:       make(chan interface{}, 32),
		filesWritten:  make(map[string]bool),
		torrent:       t,
		ignoredPieces: bits.NewBitField(nPieces),
		pieces:        bits.NewBitField(nPieces),
		workers:       make(map[int]*worker),
	}

	for _, opt := range options {
		opt(c)
	}

	return c
}
