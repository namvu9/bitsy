package session

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/rs/zerolog/log"
)

type ClientState int

const (
	ERROR ClientState = iota
	DONE
	FETCHING_META
	SEEDING
	STARTED
	STARTING
	STOPPED
)

// Client represents the client in, and encapsulates
// interactions with, a swarm for a particular torrent
// the client is responsible for peer and piece selection
// strategy
type Client struct {
	state   ClientState
	baseDir string
	outDir  string
	// stateCh emits an event whenever the state of the client
	// changes
	stateCh chan ClientEvent

	// SwarmCh
	pieces  bits.BitField
	torrent btorrent.Torrent

	// Out
	doneCh  chan struct{}
	swarmCh chan swarm.Event
	msgOut  chan swarm.Event

	MsgIn  chan messageReceived
	dataIn chan peer.PieceMessage
}

type messageReceived struct {
	sender *peer.Peer
	msg    peer.Message
}

func (c *Client) Stop() {
	c.doneCh <- struct{}{}
	c.emit(StateChange{To: STOPPED})
}

func (c *Client) Start() error {
	go c.emit(StateChange{To: STARTING})

	if err := c.verifyPieces(); err != nil {
		c.emit(StateChange{To: ERROR})
		return err
	}

	fmt.Println("VERIFIED PIECES", c.pieces.GetSum(), len(c.torrent.Pieces()))

	// Load and Verify pieces
	// If not ok, set state to ERROR
	go c.emit(StateChange{To: STARTED})
	go c.listen()

	dataIn := make(chan peer.PieceMessage, 32)
	c.dataIn = dataIn
	go func() {
		time.Sleep(5 * time.Second)
		workers := make(map[int]*Worker)
		nextPiece := 0
		count := 0
		for i := 0; count < 10; i++ {
			nextPiece++
			if nextPiece == len(c.torrent.Pieces()) {
				break
			}
			if c.pieces.Get(nextPiece) {
				continue
			}
			count++
			w := c.DownloadPiece(uint32(nextPiece))
			go w.Run()
			workers[nextPiece] = w
		}

		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case msg := <-c.dataIn:
				w, ok := workers[int(msg.Index)]
				if !ok {
					continue
				}
				if ok && !w.IsComplete() {
					w.in <- msg
				}
				if w.IsComplete() {
					c.pieces.Set(int(w.index))
					delete(workers, int(w.index))
					fmt.Printf("COMPLETED %d (%d / %d)\n", msg.Index, c.pieces.GetSum(), len(c.torrent.Pieces()))

					c.msgOut <- swarm.MulticastMessage{
						Filter: func(p *peer.Peer) bool {
							return !p.HasPiece(int(w.index))
						},
						Handler: func(peers []*peer.Peer) {
							for _, p := range peers {
								p.Send(peer.HaveMessage{Index: w.index})
							}
						},
					}

					if c.pieces.GetSum() == len(c.torrent.Pieces()) {
						fmt.Println("ASSEMBLING")
						err := c.AssembleTorrent(path.Join(c.outDir, c.torrent.Name()))
						if err != nil {
							fmt.Println("COULDNT ASSEMBLE torrent files")
						}
						fmt.Println("ASSEMBLE COMPLETE")

						return
					}
					if nextPiece == len(c.torrent.Pieces()) {
						break
					}

					for c.pieces.Get(nextPiece) {
						// Skip until next missing piece
						nextPiece++
					}
					fmt.Printf("DOWNLOADING %d\n", nextPiece)
					w := c.DownloadPiece(uint32(nextPiece))
					w.Run()
					workers[nextPiece] = w
					nextPiece++
				}
			case <-ticker.C:
				fmt.Println("TICK", len(workers))
				go c.unchoke()
				go c.choke()
				for i, w := range workers {
					if w.Idle() {
						fmt.Println("RESTARTING", i, len(w.subpieces))
						go w.Restart()
					}
				}
			}

		}
	}()

	return nil
}

func (c *Client) unchoke() {
	// Optimistic unchoke
	c.msgOut <- swarm.MulticastMessage{
		Limit: 2,
		Filter: func(p *peer.Peer) bool {
			return p.Choked
		},
		Handler: func(p []*peer.Peer) {
			if len(p) < 5 {
				return
			}
			last := p[len(p)-1]
			last.Send(peer.UnchokeMessage{})
		},
	}
}

func (c *Client) choke() {
	// Choke the "worst" peer
	c.msgOut <- swarm.MulticastMessage{
		OrderBy: func(p1, p2 *peer.Peer) int {
			// Rank peers by upload rate
			return int(p2.UploadRate - p1.UploadRate)
		},
		Limit: 5,
		Filter: func(p *peer.Peer) bool {
			return !p.Choked
		},
		Handler: func(p []*peer.Peer) {
			if len(p) < 5 {
				return
			}
			last := p[len(p)-1]
			last.Send(peer.ChokeMessage{})
		},
	}

}

// TODO: Client should collect stats
// Session: Sends out the stat if it can
// Otherwise store the last snapshot of the stats
func (c *Client) listen() {
	for {
		select {
		case <-c.doneCh:
			return
		case msg := <-c.swarmCh:
			switch v := msg.(type) {
			case swarm.JoinEvent:
				p := v.Peer
				p.Send(peer.BitFieldMessage{BitField: c.pieces})
				c.Subscribe(p)
			}
		case msg := <-c.MsgIn:
			c.handleMessage(msg)
		}
	}
}

func (c *Client) handleMessage(msg messageReceived) {
	switch v := msg.msg.(type) {
	case peer.HaveMessage:
		if !c.pieces.Get(int(v.Index)) {
			msg.sender.Send(peer.InterestedMessage{})
		}
	case peer.BitFieldMessage:
		c.handleBitfieldMessage(v, msg.sender)
	case peer.PieceMessage:
		go func() {
			c.dataIn <- v
		}()
	case peer.RequestMessage:
		fmt.Println("RECEIVED REQUEST")
		c.handleRequestMessage(v, msg.sender)
	}
}

func (c *Client) Subscribe(p *peer.Peer) {
	go func() {
		for {
			select {
			case msg := <-p.Msg:
				c.MsgIn <- messageReceived{
					sender: p,
					msg:    msg,
				}
			case <-c.doneCh:
				fmt.Println("DONECH")
				break
			}
		}
	}()
}

type ClientEvent interface{}
type StateChange struct {
	To ClientState
}

type ClientCmd interface{}
type StopCmd struct{}

// Move downloader to client
// Pipe messages from peer to client
func (c *Client) emit(e ClientEvent) {
	switch v := e.(type) {
	case StateChange:
		c.state = v.To
	}
	c.stateCh <- e
}

// TODO: Let the session handle swarm directly
// Emit PeerLeave/PeerJoin event from swarm
type ClientConfig struct {
	InitState ClientState
	MaxPeers  int
	BaseDir   string
	OutDir    string
}

func NewClient(t btorrent.Torrent, cfg ClientConfig) *Client {
	return &Client{
		baseDir: cfg.BaseDir,
		outDir:  cfg.OutDir,
		state:   cfg.InitState,
		stateCh: make(chan ClientEvent),
		doneCh:  make(chan struct{}),
		MsgIn:   make(chan messageReceived, 2*cfg.MaxPeers),
		dataIn:  make(chan peer.PieceMessage, 32),
		msgOut:  make(chan swarm.Event, 32),
		swarmCh: make(chan swarm.Event, 32),
		torrent: t,
		pieces:  bits.NewBitField(len(t.Pieces())),
	}
}

// DownloadPieece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
// out = broadcast channel
// done = downloaded piece
func (c *Client) DownloadPiece(index uint32) *Worker {
	t := c.torrent

	var (
		size        = t.Length()
		pieceLength = t.PieceLength()
	)

	if int(index) == len(t.Pieces())-1 {
		pieceLength = size % pieceLength
	}

	lock := sync.Mutex{}
	return &Worker{
		Started:      time.Now(),
		LastModified: time.Now(),

		timeout: 5 * time.Minute,
		stop:    make(chan struct{}),
		in:      make(chan peer.PieceMessage, 32),

		index:       index,
		hash:        t.Pieces()[index],
		pieceLength: uint64(pieceLength),
		subpieces:   map[uint32][]byte{},
		torrent:     c.torrent,
		baseDir:     c.baseDir,
		lock:        &lock,

		out: c.msgOut,
	}
}

func (c *Client) handleBitfieldMessage(e peer.BitFieldMessage, p *peer.Peer) (bool, error) {
	s := c.torrent

	var (
		maxIndex = bits.GetMaxIndex(e.BitField)
		pieces   = s.Pieces()
	)

	if maxIndex >= len(pieces) {
		err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		return false, err
	}

	for i := range pieces {
		if !c.pieces.Get(i) && bits.BitField(e.BitField).Get(i) {
			go p.Send(peer.InterestedMessage{})

			return false, nil
		}
	}

	return false, nil
}

func (c *Client) handleRequestMessage(req peer.RequestMessage, p *peer.Peer) (bool, error) {
	var (
		filePath = path.Join(c.baseDir, c.torrent.HexHash(), fmt.Sprintf("%d.part", req.Index))
	)

	fmt.Printf("Request: Index: %d, offset: %d, length: %d (path: %s)\n", req.Index, req.Offset, req.Length, filePath)

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	data = data[req.Offset : req.Offset+req.Length]

	msg := peer.PieceMessage{
		Index:  req.Index,
		Offset: req.Offset,
		Piece:  data,
	}

	go p.Send(msg)

	return true, nil
}

func (c *Client) AssembleTorrent(dstDir string) error {
	t := c.torrent
	err := os.MkdirAll(dstDir, 0777)
	if err != nil {
		return err
	}

	files, err := t.Files()
	if err != nil {
		return err
	}

	offset := 0
	for _, file := range files {
		filePath := path.Join(dstDir, file.Name)
		outFile, err := os.Create(filePath)

		startIndex := offset / int(t.PieceLength())
		localOffset := offset % int(t.PieceLength())

		n, err := c.assembleFile(int(startIndex), uint64(localOffset), uint64(file.Length), outFile)
		if err != nil {
			return err
		}

		if n != int(file.Length) {
			return fmt.Errorf("expected file length to be %d but wrote %d\n", file.Length, n)
		}

		offset += n
	}

	return nil
}

func (c *Client) readPiece(index int) (*os.File, error) {
	t := c.torrent
	path := path.Join(c.baseDir, t.HexHash(), fmt.Sprintf("%d.part", index))

	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (c *Client) assembleFile(startIndex int, localOffset, fileSize uint64, w io.Writer) (int, error) {
	t := c.torrent
	var totalWritten int

	index := startIndex
	file, err := c.readPiece(index)

	if err != nil {
		return 0, err
	}

	var buf []byte
	offset := localOffset
	for uint64(totalWritten) < fileSize {
		if left := fileSize - uint64(totalWritten); left < uint64(t.PieceLength()) {
			buf = make([]byte, left)
		} else {
			buf = make([]byte, t.PieceLength())
		}

		n, err := file.ReadAt(buf, int64(offset))
		if err != nil && err != io.EOF {
			return totalWritten, err
		}

		n, err = w.Write(buf[:n])
		totalWritten += n
		if err != nil {
			return totalWritten, err
		}

		// load next piece
		index++
		if index < len(t.Pieces()) {
			file, err = c.readPiece(index)
			if err != nil {
				return totalWritten, err
			}
			offset = 0
		}
	}

	return totalWritten, nil
}
func (c *Client) verifyPieces() error {
	var (
		op         errors.Op = "(*Swarm).init"
		hexHash              = c.torrent.HexHash()
		torrentDir           = path.Join(c.baseDir, hexHash)
		infoLogger           = log.Info().Str("torrent", hexHash).Str("op", op.String())
	)

	err := os.MkdirAll(torrentDir, 0777)
	if err != nil {
		return err
	}

	files, err := os.ReadDir(torrentDir)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	infoLogger.Msg("Initializing torrent")
	infoLogger.Msgf("Verifying %d pieces", len(files))

	var verified int
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
				verified++
			}
		}
	}

	infoLogger.Msgf("Verified %d pieces", verified)

	return nil
}

func (c *Client) loadAndVerify(index int, piecePath string) bool {
	piece, err := os.ReadFile(piecePath)
	if err != nil {
		log.Err(err).Msgf("failed to load piece piece at %s", piecePath)
		return false
	}

	if !c.torrent.VerifyPiece(index, piece) {
		return false
	}

	if err := c.pieces.Set(index); err != nil {
		log.Err(err).Msg("failed to set bitfield")
		return false
	}

	return true
}
