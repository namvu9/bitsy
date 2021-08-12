package session

import (
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
	"github.com/rs/zerolog/log"
)

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

	out  chan swarm.Event
	stop chan struct{}
	in   chan peer.PieceMessage

	index uint32

	// Note: piece length may be shorter than what is
	// specified in the torrent if it is the last piece
	pieceLength uint64
	subpieces   map[uint32][]byte
	torrent     btorrent.Torrent
	baseDir     string
	lock        sync.Mutex
}

func (w *Worker) Idle() bool {
	return time.Now().Sub(w.LastModified) > 5*time.Second
}

func (w *Worker) Restart() {
	w.LastModified = time.Now()
	w.RequestPiece(uint32(w.index))
}

func (w *Worker) Progress() float32 {
	w.lock.Lock()
	defer w.lock.Unlock()
	return float32(len(w.subpieces)*LenSubpiece) / float32(w.pieceLength)
}

func (w *Worker) Run() {
	go func() {
		w.RequestPiece(w.index)
		for {
			select {
			case msg := <-w.in:
				w.LastModified = time.Now()
				w.lock.Lock()
				w.subpieces[msg.Offset] = msg.Piece
				w.lock.Unlock()

				if w.IsComplete() {
					err := w.savePiece(int(w.index), w.Bytes())
					if err != nil {
						fmt.Println("FAILED TO SAVE", err)
					}
					
					return
				}
			}

		}
	}()
}

func (w *Worker) IsComplete() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	var sum int
	for _, subpiece := range w.subpieces {
		sum += len(subpiece)
	}

	return uint64(sum) == w.pieceLength
}

func (w *Worker) Bytes() []byte {
	w.lock.Lock()
	defer w.lock.Unlock()
	buf := make([]byte, w.pieceLength)

	for offset, piece := range w.subpieces {
		copy(buf[offset:int(offset)+len(piece)], piece)
	}

	return buf
}

func (w *Worker) RequestPiece(index uint32) error {
	w.lock.Lock()
	defer w.lock.Unlock()

	var subPieceLength = 16 * 1024

	for offset := uint32(0); offset < uint32(w.pieceLength); offset += uint32(subPieceLength) {
		// Only request subpieces the worker doesn't already have
		if _, ok := w.subpieces[offset]; ok {
			continue
		}

		err := w.requestSubPiece(index, offset)
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *Worker) requestSubPiece(index uint32, offset uint32) error {
	var (
		remaining      = uint32(w.pieceLength) - offset
		subPieceLength = uint32(16 * 1024)
	)

	msg := peer.RequestMessage{
		Index:  index,
		Offset: offset,
	}

	if remaining < subPieceLength {
		msg.Length = uint32(remaining)
	} else {
		msg.Length = uint32(subPieceLength)
	}

	w.out <- swarm.MulticastMessage{
		OrderBy: func(p1, p2 *peer.Peer) int {
			// TODO: use uploadRate instead of total uploaded
			return int(p1.Uploaded) - int(p2.Uploaded)
		},
		Filter: func(p *peer.Peer) bool {
			return !p.Blocking && p.HasPiece(int(index)) && !p.IsServing(index, offset)
			//return p.HasPiece(int(index))
		},

		Limit: 4,
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				go p.Send(msg)
			}
		},
	}

	// Ask random peer
	w.out <- swarm.MulticastMessage{
		Filter: func(p *peer.Peer) bool {
			return p.HasPiece(int(index))
		},
		Limit: 1,
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				go p.Send(msg)
			}
		},
	}


	return nil
}

func (c *Worker) savePiece(index int, data []byte) error {
	s := c.torrent
	ok := s.VerifyPiece(index, data)
	if !ok {
		return fmt.Errorf("failed to save piece %d, verification failed", index)
	}

	// Create directory if it doesn't exist
	err := os.MkdirAll(c.baseDir, 0777)
	if err != nil {
		return err
	}

	filePath := path.Join(c.baseDir, s.HexHash(), fmt.Sprintf("%d.part", index))
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}

	n, err := file.Write(data)
	if err != nil {
		return err
	}

	log.Printf("Wrote %d bytes to %s\n", n, filePath)

	return nil
}
