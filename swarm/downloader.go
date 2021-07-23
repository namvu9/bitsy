package swarm

import (
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
)

// DownloadPieece spawns a worker that manages requesting
// subpieces from peers and will retransmit requests if the
// download begins to stall
// out = broadcast channel
// done = downloaded piece
func (s *Swarm) DownloadPiece(index uint32, done chan DownloadCompleteEvent, out chan Event) *Worker {

	var (
		size        = s.Length()
		pieceLength = s.PieceLength()
	)

	if int(index) == len(s.Pieces())-1 {
		pieceLength = uint64(size) % pieceLength
	}

	return &Worker{
		Started:      time.Now(),
		LastModified: time.Now(),

		timeout: 5 * time.Minute,
		stop:    make(chan struct{}),
		out:     out,
		done:    done,
		in:      make(chan btorrent.PieceMessage, 32),

		index:       index,
		hash:        s.Pieces()[index],
		pieceLength: pieceLength,
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

	out  chan Event
	stop chan struct{}
	done chan DownloadCompleteEvent
	in   chan btorrent.PieceMessage

	index uint32

	// Note: piece length may be shorter than what is
	// specified in the torrent if it is the last piece
	pieceLength uint64
	subpieces   map[uint32][]byte
}

func (w *Worker) Idle() bool {
	return time.Now().Sub(w.LastModified) > 5*time.Second
}

func (w *Worker) Dead() bool {
	return time.Now().Sub(w.LastModified) > time.Minute
}

func (w *Worker) Restart() {
	w.LastModified = time.Now()
	w.RequestPiece(uint32(w.index))
}

func (w *Worker) Progress() float32 {
	return float32(len(w.subpieces)*LenSubpiece) / float32(w.pieceLength)
}

func (w *Worker) Run() {
	now := time.Now()

	go w.RequestPiece(w.index)

	go func() {
		for {
			select {
			case msg := <-w.in:
				w.LastModified = time.Now()
				w.subpieces[msg.Offset] = msg.Piece

				if w.IsComplete() {
					data := w.Bytes()
					w.done <- DownloadCompleteEvent{
						Data:     data,
						Index:    w.index,
						Duration: time.Now().Sub(now),
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

	return buf
}

func (w *Worker) RequestPiece(index uint32) error {
	var op errors.Op = "(*Swarm).RequestPiece"
	var subPieceLength = 16 * 1024

	for offset := uint32(0); offset < uint32(w.pieceLength); offset += uint32(subPieceLength) {
		// Only request subpieces the worker doesn't already have
		if _, ok := w.subpieces[offset]; ok {
			continue
		}

		err := w.requestSubPiece(index, offset)
		if err != nil {
			return errors.Wrap(err, op)
		}
	}

	return nil
}

func (w *Worker) requestSubPiece(index uint32, offset uint32) error {
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

	event := BroadcastRequest{
		Message: msg,
		Limit:   5,
	}

	go func() {
		w.out <- event
	}()

	return nil
}
