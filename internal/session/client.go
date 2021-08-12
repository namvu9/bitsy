package session

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
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

func (cs ClientState) String() string {
	switch cs {
	case STARTED:
		return "started"
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
	StateCh chan swarm.Event

	// SwarmCh
	pieces  bits.BitField
	torrent btorrent.Torrent

	// Out
	doneCh  chan struct{}
	swarmCh chan swarm.Event
	msgOut  chan swarm.Event

	MsgIn  chan messageReceived
	dataIn chan peer.PieceMessage

	Pending      int
	DownloadRate btorrent.FileSize
	Uploaded     int
}

type messageReceived struct {
	sender *peer.Peer
	msg    peer.Message
}

type FileStat struct {
	Index      int
	Name       string
	Size       btorrent.FileSize
	Downloaded btorrent.FileSize
}

func (fs FileStat) String() string {
	var sb strings.Builder

	percent := float64(fs.Downloaded) / float64(fs.Size) * 100

	fmt.Fprintf(&sb, "File: %s\n", fs.Name)
	fmt.Fprintf(&sb, "Index: %d\n", fs.Index)
	fmt.Fprintf(&sb, "Size: %s\n", fs.Size)
	fmt.Fprintf(&sb, "Downloaded: %s (%.3f %%)\n", fs.Downloaded, percent)

	return sb.String()
}

type ClientStat struct {
	State        ClientState
	Pieces       int
	TotalPieces  int
	DownloadRate btorrent.FileSize // per second
	Downloaded   btorrent.FileSize
	Left         btorrent.FileSize
	Uploaded     btorrent.FileSize
	Files        []FileStat
	Pending      int

	BaseDir string
	OutDir  string
}

func (c ClientStat) String() string {
	var sb strings.Builder

	total := c.Downloaded + c.Left
	percentage := float64(c.Downloaded) / float64(total) * 100

	fmt.Fprintf(&sb, "State: %s\n", c.State)
	fmt.Fprintf(&sb, "Uploaded: %s\n", c.Uploaded)
	fmt.Fprintf(&sb, "Downloaded: %s (%.3f %%)\n", c.Downloaded, percentage)
	fmt.Fprintf(&sb, "Download rate: %s / s \n", c.DownloadRate)
	fmt.Fprintf(&sb, "Pieces pending: %d\n", c.Pending)

	fmt.Fprint(&sb, "\n")
	for _, file := range c.Files {
		fmt.Fprint(&sb, file)
		fmt.Fprint(&sb, "\n")
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) Stat() ClientStat {
	var fs []FileStat

	files, err := c.torrent.Files()
	if err == nil {
		for i, file := range files {
			var downloaded int
			for _, piece := range file.Pieces {
				if c.pieces.Get(c.torrent.GetPieceIndex(piece)) {
					downloaded += int(c.torrent.PieceLength())
				}
			}

			fs = append(fs, FileStat{
				Index:      i,
				Name:       file.Name,
				Size:       file.Length,
				Downloaded: btorrent.FileSize(min(int(file.Length), downloaded)),
			})
		}
	}

	return ClientStat{
		State:        c.state,
		Uploaded:     btorrent.FileSize(c.Uploaded),
		Downloaded:   btorrent.FileSize(c.pieces.GetSum() * int(c.torrent.PieceLength())),
		DownloadRate: c.DownloadRate,
		Left:         btorrent.FileSize((len(c.torrent.Pieces()) - c.pieces.GetSum()) * int(c.torrent.PieceLength())),

		Pieces:      c.pieces.GetSum(),
		Pending:     c.Pending,
		TotalPieces: len(c.torrent.Pieces()),
		Files:       fs,

		BaseDir: c.baseDir,
		OutDir:  c.outDir,
	}
}

func (c *Client) Stop() {
	c.doneCh <- struct{}{}
	c.emit(StateChange{To: STOPPED})
}

func (c *Client) nextNPieces(n int, exclude map[int]*Worker) []int {
	nPieces := len(c.torrent.Pieces())
	count := min(n, nPieces-c.pieces.GetSum())
	var out []int

	for len(out) < count {
		i := int(rand.Int31n(int32(nPieces)))
		if !c.pieces.Get(i) && exclude[i] == nil {
			out = append(out, i)
		}
	}

	return out
}

func (c *Client) Start() error {
	c.emit(StateChange{To: STARTING})

	if err := c.verifyPieces(); err != nil {
		c.emit(StateChange{To: ERROR, Msg: err.Error()})
		return err
	}

	c.emit(StateChange{To: STARTED})

	go c.listen()

	if !c.done() {
		go c.download()
	} else {
		// TODO: Check if data has been written
		c.AssembleTorrent(path.Join(c.outDir, c.torrent.Name()))

		c.emit(StateChange{To: SEEDING})
	}

	return nil
}

func (c *Client) download() {
	var (
		workers = make(map[int]*Worker)
		ticker  = time.NewTicker(5 * time.Second)
	)

	go c.DownloadN(5, workers)

	last := time.Now()

	batchSize := 5 * c.torrent.PieceLength()
	downloadRate := 0.0
	count := 0

	for {
		select {
		case msg := <-c.dataIn:
			done, err := c.handlePieceMessage(msg, workers)
			if err != nil || !done {
				continue
			}

			if done {
				count++
				if count == 5 {
					diffT := time.Now().Sub(last)

					rate := float64(batchSize) / diffT.Seconds()
					downloadRate = rate

					last = time.Now()
					count = 0
				}

				fmt.Println("Downloaded", msg.Index, len(workers))
				c.emit(DownloadCompleteEvent{
					Index:        int(msg.Index),
					DownloadRate: btorrent.FileSize(downloadRate),
					Pending:      len(workers),
				})
			}

			if c.done() {
				err := c.AssembleTorrent(path.Join(c.outDir, c.torrent.Name()))
				if err != nil {
					c.emit(StateChange{To: ERROR, Msg: err.Error()})
					return
				}

				c.emit(StateChange{To: SEEDING})
				return
			}
			c.DownloadN(1, workers)
		// Finished downloading piece
		case <-ticker.C:
			go c.unchoke()
			go c.choke()

			count := 0
			for _, w := range workers {
				if w.Idle() {
					count++
					go w.Restart()
				}
			}

			if count > 5 {
				c.DownloadN(5, workers)
			}
		}
	}
}

func (c *Client) handlePieceMessage(msg peer.PieceMessage, workers map[int]*Worker) (bool, error) {
	w, ok := workers[int(msg.Index)]
	if !ok {
		return false, fmt.Errorf("%d: unknown piece", msg.Index)
	}

	w.in <- msg

	if w.IsComplete() {
		delete(workers, int(w.index))
		c.pieces.Set(int(w.index))

		c.msgOut <- HaveMessage(int(w.index))
		//c.msgOut <- CancelMessage(int(w.index))
		return true, nil
	}

	return false, nil
}

func HaveMessage(idx int) swarm.MulticastMessage {
	return swarm.MulticastMessage{
		Filter: func(p *peer.Peer) bool {
			return !p.HasPiece(idx)
		},
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				p.Send(peer.HaveMessage{Index: uint32(idx)})
			}
		},
	}
}

func CancelMessage(idx int) swarm.MulticastMessage {
	return swarm.MulticastMessage{
		Filter: func(p *peer.Peer) bool {
			return p.HasPiece(idx)
		},
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				p.Send(peer.CancelMessage{Index: uint32(idx)})
			}
		},
	}
}

func (c *Client) done() bool {
	return c.pieces.GetSum() == len(c.torrent.Pieces())
}

func (c *Client) DownloadN(n int, workers map[int]*Worker) {
	for _, pieceIdx := range c.nextNPieces(n, workers) {
		workers[pieceIdx] = c.DownloadPiece(uint32(pieceIdx))
		c.emit(DownloadEvent{
			Pending: len(workers),
		})
	}
}

func (c *Client) unchoke() {
	// Optimistic unchoke
	c.msgOut <- swarm.MulticastMessage{
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
		case msg := <-c.swarmCh:
			switch v := msg.(type) {
			case swarm.JoinEvent:
				p := v.Peer
				p.Send(peer.BitFieldMessage{BitField: c.pieces})
				c.subscribe(p)
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
		c.handleRequestMessage(v, msg.sender)
	}
}

func (c *Client) subscribe(p *peer.Peer) {
	go func() {
		for {
			select {
			case msg := <-p.Msg:
				c.MsgIn <- messageReceived{
					sender: p,
					msg:    msg,
				}
			case <-c.doneCh:
				break
			}
		}
	}()
}

type ClientEvent interface{}
type StateChange struct {
	To  ClientState
	Msg string
}

type DownloadEvent struct {
	Pending int
}

type DownloadCompleteEvent struct {
	Index        int
	DownloadRate btorrent.FileSize
	Pending      int
}

func (c *Client) emit(e ClientEvent) {
	switch v := e.(type) {
	case StateChange:
		c.state = v.To
	case DownloadCompleteEvent:
		c.DownloadRate = v.DownloadRate
		c.Pending = v.Pending
	case DownloadEvent:
		c.Pending = v.Pending
	}
	go func() {
		c.StateCh <- c.Stat()
	}()
}

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
		StateCh: make(chan swarm.Event),
		doneCh:  make(chan struct{}),
		MsgIn:   make(chan messageReceived, 2*cfg.MaxPeers),
		dataIn:  make(chan peer.PieceMessage, 32),
		msgOut:  make(chan swarm.Event, 32),
		swarmCh: make(chan swarm.Event, 32),
		torrent: t,
		pieces:  bits.NewBitField(len(t.Pieces())),
	}
}

// DownloadPiece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
func (c *Client) DownloadPiece(index uint32) *Worker {
	var (
		t           = c.torrent
		size        = t.Length()
		pieceLength = t.PieceLength()
	)

	if int(index) == len(t.Pieces())-1 {
		pieceLength = size % pieceLength
	}

	w := &Worker{
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

		out: c.msgOut,
	}

	w.Run()

	return w
}

func (c *Client) handleBitfieldMessage(e peer.BitFieldMessage, p *peer.Peer) (bool, error) {
	var (
		t        = c.torrent
		maxIndex = bits.GetMaxIndex(e.BitField)
		pieces   = t.Pieces()
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
	c.Uploaded += len(data)

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

	fmt.Printf("Verified %d pieces (%d)\n", c.pieces.GetSum(), len(c.torrent.Pieces()))

	return nil
}

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
