package session

import (
	"time"

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

// Client represents the client in, and encapsulates
// interactions with, a swarm for a particular torrent
// the client is responsible for peer and piece selection
// strategy
type Client struct {
	state   ClientState
	baseDir string
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

	MsgIn chan messageReceived
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

	// Load and Verify pieces
	// If not ok, set state to ERROR
	go c.emit(StateChange{To: STARTED})

	go c.listen()

	return nil
}

func (c *Client) unchoke() {
	// Optimistic unchoke
	c.msgOut <- swarm.MulticastMessage{
		Limit: 1,
		Filter: func(p *peer.Peer) bool {
			return p.Choked && p.Interested
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
		case v := <-c.swarmCh:
			switch v := v.(type) {
			case swarm.JoinEvent:
				peer := v.Peer
				c.Subscribe(peer)
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
	}
}

func (c *Client) Subscribe(p *peer.Peer) {
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
	baseDir   string
}

func newClient(t btorrent.Torrent, cfg ClientConfig) *Client {
	return &Client{
		baseDir: cfg.baseDir,
		state:   cfg.InitState,
		stateCh: make(chan ClientEvent),
		doneCh:  make(chan struct{}),
		MsgIn:   make(chan messageReceived, 2*cfg.MaxPeers),
		msgOut:  make(chan swarm.Event),
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
		pieceLength = uint64(size) % pieceLength
	}

	return &Worker{
		Started:      time.Now(),
		LastModified: time.Now(),

		timeout: 5 * time.Minute,
		stop:    make(chan struct{}),
		in:      make(chan peer.PieceMessage, 32),

		index:       index,
		hash:        t.Pieces()[index],
		pieceLength: pieceLength,
		subpieces:   map[uint32][]byte{},
	}
}

// TODO: Figure out rarest piece
//func (s *Stats) handleMessageReceived(e MessageReceived) (bool, error) {
	//switch v := e.Message.(type) {
	//case peer.KeepAliveMessage:
		//s.messageSentCount["keepAlive"]++
	//case peer.ChokeMessage:
		//s.messageReceivedCount["choke"]++
	//case peer.UnchokeMessage:
		//s.messageReceivedCount["unchoke"]++
	//case peer.InterestedMessage:
		//s.messageReceivedCount["interested"]++
	//case peer.NotInterestedMessage:
		//s.messageReceivedCount["notInterested"]++
	//case peer.HaveMessage:
		//s.messageReceivedCount["have"]++
		//s.pieceFrequency[int(v.Index)]++
	//case peer.BitFieldMessage:
		//bitfield := bits.BitField(v.BitField)
		//for i := range s.Pieces() {
			//if bitfield.Get(i) {
				//s.pieceFrequency[i]++
			//}
		//}
		//s.messageReceivedCount["bitfield"]++
	//case peer.RequestMessage:
		//s.messageReceivedCount["request"]++
	//case peer.PieceMessage:
		//s.messageReceivedCount["piece"]++
		//s.downloaded += uint64(len(v.Piece))

		//if diff := time.Now().Sub(s.lastDownloadChunk); diff > 5*time.Second {
			//s.downloadRate = (s.downloaded - s.downloadedChunk) / uint64(diff.Seconds())
			//s.downloadedChunk = s.downloaded
			//s.lastDownloadChunk = time.Now()
		//}

	//case peer.CancelMessage:
		//s.messageReceivedCount["cancel"]++
	//case peer.ExtendedMessage:
		//s.messageReceivedCount["extended"]++
	//default:
		//s.messageReceivedCount["unknown"]++
	//}

	//return true, nil
//}

//// TODO: Figure out rarest piece
//func (s *Stats) handleMessageSent(e MessageSent) (bool, error) {
	//switch v := e.Message.(type) {
	//case peer.KeepAliveMessage:
		//s.messageSentCount["keepAlive"]++
	//case peer.ChokeMessage:
		//s.messageSentCount["choke"]++
	//case peer.UnchokeMessage:
		//s.messageSentCount["unchoke"]++
	//case peer.InterestedMessage:
		//s.messageSentCount["interested"]++
	//case peer.NotInterestedMessage:
		//s.messageSentCount["notInterested"]++
	//case peer.HaveMessage:
		//s.messageSentCount["have"]++
		//s.pieceFrequency[int(v.Index)]++
	//case peer.BitFieldMessage:
		//bitfield := bits.BitField(v.BitField)
		//for i := range s.Pieces() {
			//if bitfield.Get(i) {
				//s.pieceFrequency[i]++
			//}
		//}
		//s.messageSentCount["bitfield"]++
	//case peer.RequestMessage:
		//s.messageSentCount["request"]++
	//case peer.PieceMessage:
		//s.messageSentCount["piece"]++
		//s.uploaded += uint64(len(v.Piece))
	//case peer.CancelMessage:
		//s.messageSentCount["cancel"]++
	//case peer.ExtendedMessage:
		//s.messageSentCount["extended"]++
	//default:
		//s.messageSentCount["unknown"]++
	//}

	//return true, nil
//}

//func (s *Stats) handleDataSentEvent(e DataSentEvent) (bool, error) {
	//s.uploaded += uint64(len(e.Piece))
	//return true, nil
//}

//func (s *Stats) handleDataReceivedEvent(e DownloadCompleteEvent) (bool, error) {
	//s.downloaded += uint64(len(e.Data))
	//return true, nil
//}

//func NewStats(torrent btorrent.Torrent) *Stats {
	//pieceFreq := make(map[int]int)

	//for i := range torrent.Pieces() {
		//pieceFreq[i] = 0
	//}

	//return &Stats{
		//Torrent:              torrent,
		//eventCh:              make(chan Event, 32),
		//outCh:                make(chan Event, 32),
		//messageReceivedCount: make(map[string]int),
		//messageSentCount:     make(map[string]int),
		//pieceFrequency:       pieceFreq,
		//lastDownloadChunk:    time.Now(),
	//}
//}
