package session

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
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
	done chan swarm.DownloadCompleteEvent
	in   chan peer.PieceMessage

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
					w.done <- swarm.DownloadCompleteEvent{
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

	msg := peer.RequestMessage{
		Index:  index,
		Offset: offset,
	}

	if remaining < subPieceLength {
		msg.Length = uint32(remaining)
	} else {
		msg.Length = uint32(subPieceLength)
	}

	//event := BroadcastRequest{
	//Message: msg,
	//Limit:   5,
	//}

	//go func() {
	//w.out <- event
	//}()

	return nil
}
func (c *Client) download() {
	t := c.torrent
	var (
		//maxPending = 50
		pieces = t.Pieces()
	)

	pending := make(map[uint32]*Worker)

	//for len(c.peers) < 5 {
	//time.Sleep(time.Second)
	//}

	// Request every piece?
	// Initialize download by simply picking the first 10 that
	// the client doesn't have
	var missing []int
	for i := range pieces {
		if !c.pieces.Get(i) {
			missing = append(missing, i)
		}
	}
	for _, index := range missing {
		if len(pending) == 10 {
			break
		}
		worker := c.DownloadPiece(uint32(index))
		go worker.Run()
		pending[uint32(index)] = worker
	}

	//ticker := time.NewTicker(5 * time.Second)
	for {
		done := c.pieces.GetSum() == len(pieces)
		if done {
			return
		}

		select {
		//case msg := <-t.downloadCh:
		//worker, ok := pending[msg.Index]
		//if ok {
		//go func() {
		//worker.in <- msg
		//}()
		//}

		//case event := <-completeCh:
		//err := t.savePiece(int(event.Index), event.Data)
		//if err != nil {
		//continue
		//}

		//// save piece
		//t.have.Set(int(event.Index))
		//delete(pending, event.Index)

		//go t.publish(event)
		//if t.Done() {
		//err := t.AssembleTorrent(path.Join(t.downloadDir, t.Name()))
		//if err != nil {
		//return
		//}

		//return
		//}

		//// TODO: Broadcast Cancel messages for any pending requests

		//// Download new pieces by picking the rarest first
		//count := 0
		//for _, piece := range t.stats.PieceIdxByFreq() {
		//if count == 10 {
		//break
		//}
		//if _, isPending := pending[uint32(piece.index)]; !isPending && !t.have.Get(piece.index) {
		//worker := t.DownloadPiece(uint32(piece.index), completeCh, t.eventCh)
		//go worker.Run()
		//pending[uint32(piece.index)] = worker
		//count++
		//}
		//}
		//case <-ticker.C:
		//// Check if any of the workers need to be restarted
		//for _, worker := range pending {
		//if worker.Idle() {
		//go worker.Restart()
		//}
		//}

		//count := 0
		//for _, piece := range t.stats.PieceIdxByFreq() {
		//if count == 10 {
		//break
		//}
		//if _, isPending := pending[uint32(piece.index)]; !isPending && !t.have.Get(piece.index) {
		//worker := t.DownloadPiece(uint32(piece.index), completeCh, t.eventCh)
		//go worker.Run()
		//pending[uint32(piece.index)] = worker
		//count++
		//}
		//}

		}

	}
}

func (c *Client) savePiece(index int, data []byte) error {
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

//func (s *Swarm) handleDownloadCompleteEvent(e DownloadCompleteEvent) (bool, error) {
//s.have.Set(int(e.Index))
//go s.publish(BroadcastRequest{
//Message: peer.HaveMessage{Index: e.Index},
//})

//return true, nil
//}
//func (s *Swarm) unchokePeers() {
	//// Done
	//if s.have.GetSum() == len(s.Pieces()) {
		//choked, _ := s.Choked()
		//for _, peer := range choked {
			//go peer.Send(peer.UnchokeMessage{})
		//}

		//return
	//}

	//choked, unchoked := s.Choked()

	//nUnchoked := len(unchoked)
	//nChoked := len(choked)

	//if nUnchoked >= 5 {
		//if len(choked) > 0 {
			//index := rand.Int31n(int32(len(choked)))
			//peer := choked[index]

			//go peer.Send(peer.UnchokeMessage{})
		//}

		//index := rand.Int31n(int32(len(unchoked)))
		//peer := unchoked[index]

		//// TODO: Only choke the "worst" peers
		//go peer.Send(peer.ChokeMessage{})
	//} else {
		//for nChoked > 0 && nUnchoked < 5 {
			//index := rand.Int31n(int32(len(choked)))
			//peer := choked[index]

			//go peer.Send(peer.UnchokeMessage{})
			//nChoked--
			//nUnchoked++
		//}
	//}

	//if len(s.peers) > 0 {
		//index := rand.Int31n(int32(len(s.peers)))
		//peer := s.peers[index]

		//go peer.Send(peer.UnchokeMessage{})

		////
	//}
//}

//func (s *Swarm) clearIdlePeers() {
	//for _, peer := range s.peers {
		//if peer.Idle() {
			//event := LeaveEvent{peer}
			//s.eventCh <- event
			//s.publish(event)
		//}
	//}
//}


//func (s *Swarm) handleUnchokeEvent(e UnchokeEvent) (bool, error) {
	//peer, ok := s.getPeer(e.RemoteAddr())
	//if !ok {
		//return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	//}

	//if !peer.Choked {
		//return true, nil
	//}

	//peer.Choked = false
	//go peer.Send(peer.UnchokeMessage{})

	//return true, nil

//}

//func (s *Swarm) handleChokeEvent(e ChokeEvent) (bool, error) {
	//peer, ok := s.getPeer(e.Peer.RemoteAddr())
	//if !ok {
		//return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	//}

	//peer.Choked = true
	//return true, nil
//}

//func (s *Swarm) handleInterestingEvent(e InterestingEvent) (bool, error) {
	//peer, ok := s.getPeer(e.Peer.RemoteAddr())
	//if !ok {
		//return false, fmt.Errorf("User with address %s is not a participant in the swarm", e.RemoteAddr())
	//}

	//peer.Interesting = true
	//go peer.Send(InterestedEvent{})

	//return false, nil
//}


//func (s *Swarm) handleMessageReceived(msg MessageReceived) (bool, error) {
	//switch v := msg.Message.(type) {
	//case peer.RequestMessage:
		//return s.handleDataRequestEvent(v, msg.Peer)
	//case peer.BitFieldMessage:
		//return s.handleBitfieldMessage(v, msg.Peer)
	//case peer.HaveMessage:
		//return s.handleHaveMessage(v, msg.Peer)
	//case peer.PieceMessage:
		//go func() {
			//s.downloadCh <- v
		//}()

	//}
	//return true, nil
//}

//func (s *Swarm) handleDataRequestEvent(req peer.RequestMessage, peer *Peer) (bool, error) {
	//var (
		//filePath = path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", req.Index))
	//)

	//log.Printf("Request: Index: %d, offset: %d, length: %d (path: %s)\n", req.Index, req.Offset, req.Length, filePath)

	//data, err := ioutil.ReadFile(filePath)
	//if err != nil {
		//return false, err
	//}

	//data = data[req.Offset : req.Offset+req.Length]

	//msg := peer.PieceMessage{
		//Index:  req.Index,
		//Offset: req.Offset,
		//Piece:  data,
	//}

	//go peer.Send(msg)

	//return true, nil
//}

//func (s *Swarm) handleBitfieldMessage(e peer.BitFieldMessage, peer *Peer) (bool, error) {
	//var op errors.Op = "(*Swarm).handleBitfieldMessage"

	//log.Info().
		//Str("torrent", s.HexHash()).
		//Int("messageType", int(peer.BitField)).
		//Msgf("Received BitField message from %s", peer.RemoteAddr())

	//var (
		//maxIndex = bits.GetMaxIndex(e.BitField)
		//pieces   = s.Pieces()
	//)

	//if maxIndex >= len(pieces) {
		//err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		//return false, errors.Wrap(err, op, errors.BadArgument)
	//}

	//for i := range pieces {
		//if !s.have.Get(i) && bits.BitField(e.BitField).Get(i) {
			//go peer.Send(peer.InterestedMessage{})

			//return false, nil
		//}
	//}

	//return false, nil
//}

//func (s *Swarm) handleHaveMessage(msg peer.HaveMessage, peer *Peer) (bool, error) {
	//if !s.have.Get(int(msg.Index)) {
		//go peer.Send(peer.InterestedMessage{})

		//return false, nil
	//}

	//return true, nil
//}

//func (s *Swarm) AssembleTorrent(dstDir string) error {
	//err := os.MkdirAll(dstDir, 0777)
	//if err != nil {
		//return err
	//}

	//files, err := s.Files()
	//if err != nil {
		//return err
	//}

	//offset := 0
	//for _, file := range files {
		//filePath := path.Join(dstDir, file.Name)
		//outFile, err := os.Create(filePath)

		//startIndex := offset / int(s.PieceLength())
		//localOffset := offset % int(s.PieceLength())

		//n, err := s.assembleFile(int(startIndex), uint64(localOffset), file.Length, outFile)
		//if err != nil {
			//return err
		//}

		//if n != int(file.Length) {
			//return fmt.Errorf("expected file length to be %d but wrote %d\n", file.Length, n)
		//}

		//offset += n
	//}

	//return nil
//}

//func (s *Swarm) readPiece(index int) (*os.File, error) {
	//path := path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", index))

	//file, err := os.Open(path)
	//if err != nil {
		//return nil, err
	//}

	//return file, nil
//}

//func (s *Swarm) assembleFile(startIndex int, localOffset, fileSize uint64, w io.Writer) (int, error) {
	//var totalWritten int

	//index := startIndex
	//file, err := s.readPiece(index)

	//if err != nil {
		//return 0, err
	//}

	//var buf []byte
	//offset := localOffset
	//for uint64(totalWritten) < fileSize {
		//if left := fileSize - uint64(totalWritten); left < s.PieceLength() {
			//buf = make([]byte, left)
		//} else {
			//buf = make([]byte, s.PieceLength())
		//}

		//n, err := file.ReadAt(buf, int64(offset))
		//if err != nil && err != io.EOF {
			//return totalWritten, err
		//}

		//n, err = w.Write(buf[:n])
		//totalWritten += n
		//if err != nil {
			//return totalWritten, err
		//}

		//// load next piece
		//index++
		//if index < len(s.Pieces()) {
			//file, err = s.readPiece(index)
			//if err != nil {
				//return totalWritten, err
			//}
			//offset = 0
		//}
	//}

	//return totalWritten, nil
//}
