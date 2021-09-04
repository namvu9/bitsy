package data

import (
	"fmt"
	"sync"
	"time"

	"github.com/namvu9/bitsy/internal/pieces"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

// lenSubpiece is the maximum length to use when requesting
// a subpiece
const lenSubpiece = 16 * 1024

// worker represents an active request for a piece
type worker struct {
	Started      time.Time
	LastModified time.Time

	// The SHA-1 hash of the complete piece
	hash []byte

	timeout time.Duration

	out  chan interface{}
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

	fast bool

	pieceMgr pieces.Service
}

func (w *worker) idle() bool {
	return time.Now().Sub(w.LastModified) > 5*time.Second
}

func (w *worker) dead() bool {
	return time.Now().Sub(w.LastModified) > 15*time.Second
}

func (w *worker) restart() {
	w.requestPiece(uint32(w.index), w.fast)
}

func (w *worker) run() {
	go func() {
		w.requestPiece(w.index, w.fast)
		for {
			select {
			case <-w.stop:
				return
			case msg, ok := <-w.in:
				if !ok {
					return
				}
				// ignore subpieces we already have
				if _, ok := w.subpieces[msg.Offset]; ok {
					continue
				}
				w.LastModified = time.Now()
				w.lock.Lock()
				w.subpieces[msg.Offset] = msg.Piece
				w.lock.Unlock()

				if w.isComplete() {
					err := w.savePiece(int(w.index), w.bytes())
					if err != nil {
						fmt.Println("FAILED TO SAVE", err)
					}

					return
				}
			}

		}
	}()
}

func (w *worker) isComplete() bool {
	return uint64(16*1024*len(w.subpieces)) >= w.pieceLength
}

func (w *worker) bytes() []byte {
	w.lock.Lock()
	defer w.lock.Unlock()
	buf := make([]byte, w.pieceLength)

	for offset, piece := range w.subpieces {
		copy(buf[offset:int(offset)+len(piece)], piece)
	}

	return buf
}

func (w *worker) requestPiece(index uint32, fast bool) error {
	var subPieceLength = 16 * 1024

	for offset := uint32(0); offset < uint32(w.pieceLength); offset += uint32(subPieceLength) {
		// Only request subpieces the worker doesn't already have
		if _, ok := w.subpieces[offset]; ok {
			continue
		}

		w.requestSubPiece(index, offset, fast)
	}

	return nil
}

func (w *worker) requestSubPiece(index uint32, offset uint32, fast bool) error {
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

	w.out <- RequestMessage{
		Hash:           w.torrent.InfoHash(),
		RequestMessage: msg,
	}

	return nil
}

func (c *worker) savePiece(index int, data []byte) error {
	err := c.pieceMgr.Verify(c.torrent.InfoHash(), index, data)
	if err != nil {
		return err
	}

	return c.pieceMgr.Save(c.torrent.InfoHash(), index, data)
}
