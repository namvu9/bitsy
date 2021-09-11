package data

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/namvu9/bitsy/internal/assembler"
	"github.com/namvu9/bitsy/internal/pieces"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type ClientState int

const TOP_SPEED = 50 * btorrent.MiB

func maxPendingPieces(pieceSize int, topSpeed int) int {
	return topSpeed / pieceSize
}

const (
	STOPPED ClientState = iota
	PAUSED
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

	workers map[int]*worker

	pieceMgr  pieces.Service
	assembler assembler.Service
	err       error
}

func (c *Client) Stop() error {
	c.doneCh <- struct{}{}
	c.emit(StateChange{To: STOPPED})
	return nil
}

func (c *Client) Start(ctx context.Context) error {
	if c.state == DOWNLOADING || c.state == SEEDING {
		return nil
	}
	c.emit(StateChange{To: STARTING})

	if err := c.verifyPieces(); err != nil {
		c.emit(StateChange{To: ERROR, Msg: err.Error()})
		return err
	}

	err := c.assembler.Assemble(c.torrent.InfoHash(), c.pieces)
	if err != nil {
		fmt.Println("ERR", err)
	}

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
func (c *Client) nextNPieces(n int, exclude map[int]*worker) []int {
	var remaining []int

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

func (c *Client) clearCompletedPieces() {
	count := 0

	var out []DownloadCompleteEvent
	for idx, w := range c.workers {
		if w.isComplete() {
			delete(c.workers, idx)
			c.pieces.Set(idx)
			out = append(out, DownloadCompleteEvent{
				Index: idx,
				Hash:  c.torrent.InfoHash(),
			})
			count++
		}
	}

	for _, ev := range out {
		c.MsgOut <- ev
	}
}

func (c *Client) download() {
	var (
		ticker       = time.NewTicker(2 * time.Second)
		downloadRate = 0.0
		batch        = 0
		maxPieces    = maxPendingPieces(int(c.torrent.PieceLength()), TOP_SPEED)
	)

	c.downloadN(1)

	for {
		select {
		case msg := <-c.DataIn:
			done, err := c.handlePieceMessage(msg, c.workers)
			if err != nil {
				continue
			}

			if done {
				if len(c.workers) < maxPieces {
					c.downloadN(maxPieces)
				}
				continue
			}

			batch += len(msg.Piece)

		case <-ticker.C:
			c.clearCompletedPieces()
			if len(c.workers) < maxPieces {
				c.downloadN(1)
			}

			downloadRate = float64(batch) / 2.0

			batch = 0

			for _, w := range c.workers {
				if w.idle() {
					go w.restart()
				}
			}

			err := c.assembler.Assemble(c.torrent.InfoHash(), c.pieces)
			if err != nil {
				fmt.Println("ERR ASSEMBLING", err)
			}

			if c.done() {
				c.emit(StateChange{To: SEEDING})
			}

			c.DownloadRate = btorrent.Size(downloadRate)
		}
	}
}

func (c *Client) done() bool {
	return c.pieces.GetSum() == len(c.torrent.Pieces())
}

func (c *Client) downloadN(n int) {
	for _, pieceIdx := range c.nextNPieces(n, c.workers, c.piecePrio) {
		c.downloadPiece(uint32(pieceIdx), false)
	}
}

func (c *Client) listen() {
	for {
		select {
		case <-c.doneCh:
			return
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
	Hash  InfoHash
	Index int
}

func (c *Client) emit(e ClientEvent) {
	switch v := e.(type) {
	case StateChange:
		c.state = v.To
		if v.To == ERROR {
			c.err = errors.New(v.Msg)
		} else {
			c.err = nil
		}
	case DownloadEvent:
		c.Pending = v.Pending
	}
}

// downloadPiece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
func (c *Client) downloadPiece(index uint32, fast bool) *worker {
	if _, ok := c.workers[int(index)]; ok || c.ignoredPieces.Get(int(index)) {
		return nil
	}

	if len(c.workers) > maxPendingPieces(int(c.torrent.PieceLength()), TOP_SPEED) {
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

		pieceMgr: c.pieceMgr,
	}

	w.run()

	c.workers[int(index)] = w

	return w
}

// Verify which pieces the client has
func (c *Client) verifyPieces() error {
	for idx := range c.torrent.Pieces() {
		piece, err := c.pieceMgr.Load(c.torrent.InfoHash(), idx)
		if err != nil {
			continue
		}

		err = c.pieceMgr.Verify(c.torrent.InfoHash(), idx, piece)
		if err != nil {
			return err
		}

		c.pieces.Set(idx)
	}

	return nil
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

func New(t btorrent.Torrent, baseDir, outDir string, emitter chan interface{}, pieceMgr pieces.Service, assembler assembler.Service, options ...Option) *Client {
	nPieces := len(t.Pieces())
	c := &Client{
		baseDir:       baseDir,
		outDir:        outDir,
		state:         STOPPED,
		StateCh:       make(chan interface{}),
		emitCh:        make(chan interface{}, 32),
		MsgIn:         make(chan messageReceived, 32),
		DataIn:        make(chan peer.PieceMessage, 32),
		MsgOut:        emitter,
		SwarmCh:       make(chan interface{}, 32),
		doneCh:        make(chan struct{}),
		filesWritten:  make(map[string]bool),
		torrent:       t,
		ignoredPieces: bits.NewBitField(nPieces),
		pieces:        bits.NewBitField(nPieces),
		workers:       make(map[int]*worker),
		pieceMgr:      pieceMgr,
		assembler:     assembler,
	}

	for _, opt := range options {
		opt(c)
	}

	return c
}
