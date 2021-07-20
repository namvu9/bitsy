package messaging

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/rs/zerolog/log"
)

// PendingRequest represents an active request for a piece
// Make a map of channels where the receiver is a
// goroutine that manages pending pieces
// The goroutine returns whenever the channel is closed or
// it completes
type PendingRequest struct {
	Started      time.Time
	LastModified time.Time

	// Request missing pieces
	in   chan PieceMessage
	done chan CompletedRequest
}

type CompletedRequest struct {
	StartedAt time.Time
	Duration  time.Duration
	Err       error
}

type Downloader struct {
	t       torrent.Torrent
	didInit bool
	pending map[uint32]*PendingRequest

	haveCh     chan HaveMessage
	responseCh chan PieceMessage

	nextPeers getPeersFunc
	nextPiece getPieceFunc

	baseDir string
}

type getPeersFunc func(int, func(Peer) bool) []*Peer
type broadcastFunc func(...Message)
type getPieceFunc func() int

func (d *Downloader) requestSubPiece(index uint32, offset uint32, p *Peer) error {
	//var op errors.Op = "(*Swarm).requestSubPiece"

	var (
		remaining      = uint32(d.t.PieceLength()) - offset
		subPieceLength = uint32(16 * 1024)
	)

	msg := RequestMessage{
		Index:  index,
		Offset: offset,
	}

	if remaining < subPieceLength {
		msg.Length = uint32(remaining)
	} else {
		msg.Length = uint32(subPieceLength)
	}

	// Skip if we've already asked the peer for this sub-piece
	for _, request := range p.requests {
		if request == msg {
			return nil
		}
	}

	go p.RequestPiece(msg)

	p.requests = append(p.requests, msg)
	return nil
}

func newDownloader(t torrent.Torrent, nextPeers getPeersFunc, nextPiece getPieceFunc, baseDir string) Downloader {
	return Downloader{
		t:          t,
		pending:    make(map[uint32]*PendingRequest),
		responseCh: make(chan PieceMessage, 10),
		haveCh:     make(chan HaveMessage, 10),
		nextPeers:  nextPeers,
		nextPiece:  nextPiece,
		baseDir:    baseDir,
	}
}

func (d *Downloader) Stat() {
	fmt.Println("PENDING", len(d.pending))
}

func (d *Downloader) Init() {
	if d.didInit {
		return
	}

	d.didInit = true

	for {
		select {
		case pieceMsg, ok := <-d.responseCh:
			if !ok {
				return
			}
			req, ok := d.pending[pieceMsg.Index]
			if ok {
				req.in <- pieceMsg
			}
		default:
			if pending := len(d.pending); pending < 10 {
				left := 10 - pending
				for i := 0; i < left; i++ {
					index := d.nextPiece()
					// TODO: Figure out what to do for last piece
					d.pending[uint32(index)] = d.DownloadPiece(uint32(index), 16)
				}
			}
		}

		var completed []uint32
		for index, pending := range d.pending {
			select {
			case res := <-pending.done:
				completed = append(completed, index)
				if res.Err != nil {
					fmt.Println("ERROR", res.Err)
				} else {
					d.haveCh <- HaveMessage{index}
				}
			default:
			}
		}

		for _, index := range completed {
			delete(d.pending, index)
		}
	}
}

// This is managed by the swarm, so we know which torrent
// this pending piece belongs to
// bufSize corresponds to the expected number of subpieces
// Rename to RequestPiece
// TODO: queue up n of these (pipelining)
func (d *Downloader) DownloadPiece(index uint32, bufSize int) *PendingRequest {
	var peers []*Peer
	var op errors.Op = "(*Downloader).DownloadPiece"
	var startTime = time.Now()

	var (
		done        = make(chan CompletedRequest)
		in          = make(chan PieceMessage, bufSize)
		pieceLength = d.t.PieceLength()
		ticker      = time.NewTicker(10 * time.Second)
	)

	for {
		peers = d.nextPeers(2, func(p Peer) bool {
			return !p.Blocking && p.BitField != nil && p.BitField.Get(int(index))
		})

		if len(peers) > 1 {
			break
		}

		time.Sleep(1 * time.Second)
	}

	for _, peer := range peers {
		go d.RequestPiece(uint32(index), peer)
	}

	log.Info().Str("torrent", d.t.HexHash()).Msgf("Requesting piece %d from %d peers\n", index, len(peers))

	go func() {
		out := CompletedRequest{
			StartedAt: startTime,
		}

		var (
			lastModified = time.Now()
			subPieces    = make(map[int][]byte)
		)

		for {
			select {
			case <-ticker.C:
				if time.Now().Sub(lastModified) > 20*time.Second {
					go func() {
						peers := d.nextPeers(30, func(p Peer) bool {
							return !p.Blocking && p.BitField.Get(int(index))
						})

						for _, peer := range peers {
							for offset := 0; offset < int(pieceLength); offset += 16 * 1024 {
								if _, ok := subPieces[offset]; !ok {
									fmt.Println("BIT LATE REREQUESTING TO ", offset)
									d.requestSubPiece(uint32(index), uint32(offset), peer)
								}
							}
						}
					}()
				}

			case pieceMsg, ok := <-in:
				if !ok {
					return
				}

				if _, ok := subPieces[int(pieceMsg.Offset)]; !ok {
					subPieces[int(pieceMsg.Offset)] = pieceMsg.Piece
					lastModified = time.Now()
				}

				if len(subPieces) == bufSize {
					file, err := os.Create(fmt.Sprintf("%s/%s/%d.part", d.baseDir, d.t.HexHash(), index))
					if err != nil {
						out.Err = errors.Wrap(err, op, errors.IO)
						done <- out
						return
					}

					defer file.Close()

					for offset, piece := range subPieces {
						_, err := file.WriteAt(piece, int64(offset))
						if err != nil {
							out.Err = errors.Wrap(err, op, errors.IO)
							done <- out
							return
						}
					}

					piece, err := io.ReadAll(file)
					if err != nil {
						out.Err = errors.Wrap(err, op, errors.IO)
						done <- out
						return
					}

					if !d.t.VerifyPiece(int(index), piece) {
						out.Err = errors.Newf("completed piece was corrupted %d", index)
						done <- out
						return
					}

					done <- CompletedRequest{
						StartedAt: startTime,
						Duration:  time.Now().Sub(startTime),
					}
					return
				}
			default:
			}
		}
	}()

	return &PendingRequest{
		in:           in,
		done:         done,
		LastModified: time.Now(),
	}
}

func (d *Downloader) RequestPiece(index uint32, p *Peer) error {
	var op errors.Op = "(*Swarm).RequestPiece"

	var (
		pieceLength    = d.t.PieceLength()
		subPieceLength = 16 * 1024
	)

	for offset := uint32(0); offset < uint32(pieceLength); offset += uint32(subPieceLength) {
		err := d.requestSubPiece(index, offset, p)
		if err != nil {
			return errors.Wrap(err, op)
		}
	}

	return nil
}
