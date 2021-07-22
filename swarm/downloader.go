package swarm

import (
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
)

// DownloadPieece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
func (s *Swarm) DownloadPiece(index int, peers []Peer) *Worker {
	return &Worker{
		Started:      time.Now(),
		LastModified: time.Now(),

		timeout: 5 * time.Minute,
		stop:    make(chan struct{}),
		done:    make(chan Event),
		in:      make(chan Event, 32),

		hash:        s.Pieces()[index],
		pieceLength: s.PieceLength(),
		subpieces:   map[uint32][]byte{},
	}
}

// LenSubpiece is the maximum length to use when requesting
// a subpiece
const LenSubpiece = 16 * 1024

// Worker represents an active request for a piece
// Make a map of channels where the receiver is a
// goroutine that manages pending pieces
// The goroutine returns whenever the channel is closed or
// it completes
type Worker struct {
	Started      time.Time
	LastModified time.Time

	// The SHA-1 hash of the complete piece
	hash []byte

	timeout time.Duration

	stop chan struct{}
	done chan Event
	in   chan Event

	index uint32

	// Note: piece length may be shorter than what is
	// specified in the torrent if it is the last piece
	pieceLength uint64
	subpieces   map[uint32][]byte
}

func (w *Worker) Idle() bool {
	return time.Now().Sub(w.LastModified) > 20*time.Second
}

func (w *Worker) Dead() bool {
	return time.Now().Sub(w.LastModified) > time.Minute
}

func (w *Worker) Restart(peers []Peer) {
	for _, peer := range peers {
		go w.RequestPiece(uint32(w.index), peer)
	}
}

func (w *Worker) Progress() float32 {
	return float32(len(w.subpieces)*LenSubpiece) / float32(w.pieceLength)
}

func (w *Worker) Run(peers []Peer) {
	for _, peer := range peers {
		go w.RequestPiece(w.index, peer)
	}

	go func() {
		for {
			select {
			case <-time.After(w.timeout):
				return
			case event := <-w.in:
				switch v := event.(type) {
				case DataReceivedEvent:
					if _, ok := w.subpieces[v.Offset]; !ok {
						w.subpieces[v.Offset] = v.Piece
						w.LastModified = time.Now()
					}

					if w.IsComplete() {
						data := w.Bytes()
						w.done <- DownloadCompleteEvent{
							Data:  data,
							Index: w.index,
							Duration: time.Now().Sub(w.Started),
						}
					}

					return
				}
			}

		}

	}()

}

func (w *Worker) IsComplete() bool {
	var sum int
	for _, subpiece := range w.subpieces {
		sum += len(subpiece)
	}

	return uint64(sum) == w.pieceLength
}

func (w *Worker) Bytes() []byte {
	buf := make([]byte, w.pieceLength)

	for offset, piece := range w.subpieces {
		copy(buf[offset:int(offset)+len(piece)], piece)
	}

	return nil
}

func (w *Worker) RequestPiece(index uint32, p Peer) error {
	var op errors.Op = "(*Swarm).RequestPiece"
	var subPieceLength = 16 * 1024

	for offset := uint32(0); offset < uint32(w.pieceLength); offset += uint32(subPieceLength) {
		err := w.requestSubPiece(index, offset, p)
		if err != nil {
			return errors.Wrap(err, op)
		}
	}

	return nil
}

func (w *Worker) requestSubPiece(index uint32, offset uint32, p Peer) error {
	var (
		remaining      = uint32(w.pieceLength) - offset
		subPieceLength = uint32(16 * 1024)
	)

	msg := btorrent.RequestMessage{
		Index:  index,
		Offset: offset,
	}

	if remaining < subPieceLength {
		msg.Length = uint32(remaining)
	} else {
		msg.Length = uint32(subPieceLength)
	}

	return p.Send(msg)
}
