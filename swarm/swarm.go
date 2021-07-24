package swarm

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
	"github.com/namvu9/bitsy/tracker"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	btorrent.Torrent

	baseDir string
	peerID  [20]byte

	have bits.BitField // the complete and verified pieces that the client has

	workers     []chan Event // TODO: prioritize workers over subscribers
	subscribers []chan Event
	eventCh     chan Event
	outCh       chan Event
	AnnounceCh  chan []tracker.PeerInfo
	PeerCh      chan net.Conn
	downloadCh  chan btorrent.PieceMessage

	// Only the swarm gets to access peers
	peers    []*Peer
	peerInfo []tracker.PeerInfo

	stats *Stats

	downloadDir string
}

func (s *Swarm) Done() bool {
	return s.have.GetSum() == len(s.Pieces())
}

func (s *Swarm) Stat() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["torrent"] = s.HexHash()
	stats["npeers"] = len(s.peers)
	stats["have"] = s.have.GetSum()
	stats["npieces"] = len(s.Pieces())

	var (
		interested  = 0
		interesting = 0
		choking     = 0
		blocking    = 0
		idle        = 0
	)

	for _, peer := range s.peers {
		if peer.Interested {
			interested++
		}

		if peer.Interesting {
			interesting++
		}

		if peer.Choked {
			choking++
		}

		if peer.Blocking {
			blocking++
		}

		if time.Now().Sub(peer.LastMessageReceived) > 2*time.Minute {
			idle++
		}
	}

	stats["interested"] = interested
	stats["interesting"] = interesting
	stats["choking"] = choking
	stats["blocking"] = blocking
	stats["idle"] = idle

	select {
	case swarmStats := <-s.stats.outCh:
		stats["stats"] = swarmStats.(StatEvent).payload
	default:
	}

	return stats
}

func (s *Swarm) Init() error {
	var op errors.Op = "(*Swarm).Init"
	err := s.verifyPieces()
	if err != nil {
		return err
	}

	if err != nil {
		return errors.Wrap(err, op)
	}

	s.Subscribe(s.stats)

	go s.download()
	go s.listen()

	return nil
}

func (sw *Swarm) Subscribe(sub Subscriber) {
	out := sub.Subscribe(sw.eventCh)
	sw.subscribers = append(sw.subscribers, out)
}

func (s *Swarm) listen() {
	ticker := time.NewTicker(10 * time.Second)

	for {
		select {
		case <-ticker.C:
			go s.unchokePeers() // randomly unchoke peers
			s.clearIdlePeers()
		case conn := <-s.PeerCh:
			event := JoinEvent{NewPeer(conn)}
			go func() {
				s.eventCh <- event
				s.publish(event)
			}()
		case event := <-s.eventCh:
			propagate, err := s.handleEvent(event)
			if err != nil {
				log.Err(err).Str("swarm", s.HexHash()).Msg("Handle event failed")
				continue
			}

			if propagate {
				s.publish(event)
			}
		}
	}
}

type PeerRequest struct {
	Hash [20]byte
	Want int
}

func (s *Swarm) requestPeers(n int) {
	s.outCh <- PeerRequest{
		Hash: s.InfoHash(),
		Want: n,
	}
}

func (s *Swarm) download() {
	var (
		maxPending = 50
		pieces     = s.Pieces()
		completeCh = make(chan DownloadCompleteEvent, maxPending)
	)

	pending := make(map[uint32]*Worker)

	for len(s.peers) < 5 {
		time.Sleep(time.Second)
	}

	// Request every piece?
	// Initialize download by simply picking the first 10 that
	// the client doesn't have
	var missing []int
	for i := range pieces {
		if !s.have.Get(i) {
			missing = append(missing, i)
		}
	}
	for _, index := range missing {
		if len(pending) == 10 {
			break
		}
		worker := s.DownloadPiece(uint32(index), completeCh, s.eventCh)
		go worker.Run()
		pending[uint32(index)] = worker
	}

	ticker := time.NewTicker(5 * time.Second)
	for {
		done := s.have.GetSum() == len(pieces)
		if done {
			return
		}

		select {
		case msg := <-s.downloadCh:
			worker, ok := pending[msg.Index]
			if ok {
				go func() {
					worker.in <- msg
				}()
			}

		case event := <-completeCh:
			err := s.savePiece(int(event.Index), event.Data)
			if err != nil {
				continue
			}

			// save piece
			s.have.Set(int(event.Index))
			delete(pending, event.Index)

			go s.publish(event)
			if s.Done() {
				err := s.AssembleTorrent(path.Join(s.downloadDir, s.Name()))
				if err != nil {
					return
				}

				return
			}

			// TODO: Broadcast Cancel messages for any pending requests

			// Download new pieces by picking the rarest first
			count := 0
			for _, piece := range s.stats.PieceIdxByFreq() {
				if count == 10 {
					break
				}
				if _, isPending := pending[uint32(piece.index)]; !isPending && !s.have.Get(piece.index) {
					worker := s.DownloadPiece(uint32(piece.index), completeCh, s.eventCh)
					go worker.Run()
					pending[uint32(piece.index)] = worker
					count++
				}
			}
		case <-ticker.C:
			// Check if any of the workers need to be restarted
			for _, worker := range pending {
				if worker.Idle() {
					go worker.Restart()
				}
			}

			count := 0
			for _, piece := range s.stats.PieceIdxByFreq() {
				if count == 10 {
					break
				}
				if _, isPending := pending[uint32(piece.index)]; !isPending && !s.have.Get(piece.index) {
					worker := s.DownloadPiece(uint32(piece.index), completeCh, s.eventCh)
					go worker.Run()
					pending[uint32(piece.index)] = worker
					count++
				}
			}

		}

	}
}

func (s *Swarm) savePiece(index int, data []byte) error {
	ok := s.VerifyPiece(index, data)
	if !ok {
		return fmt.Errorf("failed to save piece %d, verification failed", index)
	}

	// Create directory if it doesn't exist
	err := os.MkdirAll(s.baseDir, 0777)
	if err != nil {
		return err
	}

	filePath := path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", index))
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

func (s *Swarm) Choked() (choked []*Peer, unchoked []*Peer) {
	for _, peer := range s.peers {
		if peer.Choked {
			choked = append(choked, peer)
		} else {
			unchoked = append(unchoked, peer)
		}
	}

	return
}

func (s *Swarm) unchokePeers() {
	// Done
	if s.have.GetSum() == len(s.Pieces()) {
		choked, _ := s.Choked()
		for _, peer := range choked {
			go peer.Send(btorrent.UnchokeMessage{})
		}

		return
	}

	choked, unchoked := s.Choked()

	nUnchoked := len(unchoked)
	nChoked := len(choked)

	if nUnchoked >= 5 {
		if len(choked) > 0 {
			index := rand.Int31n(int32(len(choked)))
			peer := choked[index]

			go peer.Send(btorrent.UnchokeMessage{})
		}

		index := rand.Int31n(int32(len(unchoked)))
		peer := unchoked[index]

		// TODO: Only choke the "worst" peers
		go peer.Send(btorrent.ChokeMessage{})
	} else {
		for nChoked > 0 && nUnchoked < 5 {
			index := rand.Int31n(int32(len(choked)))
			peer := choked[index]

			go peer.Send(btorrent.UnchokeMessage{})
			nChoked--
			nUnchoked++
		}
	}

	if len(s.peers) > 0 {
		index := rand.Int31n(int32(len(s.peers)))
		peer := s.peers[index]

		go peer.Send(btorrent.UnchokeMessage{})

		//
	}
}

func (s *Swarm) clearIdlePeers() {
	for _, peer := range s.peers {
		if peer.Idle() {
			event := LeaveEvent{peer}
			s.eventCh <- event
			s.publish(event)
		}
	}
}

func (s *Swarm) addPeer(peer *Peer) (bool, error) {
	s.Subscribe(peer)
	s.peers = append(s.peers, peer)

	go func() {
		peer.Send(btorrent.BitFieldMessage{BitField: s.have})

		for index := range s.Pieces() {
			if s.have.Get(index) {
				go peer.Send(btorrent.HaveMessage{Index: uint32(index)})
			}
		}
	}()

	return true, nil
}

func (s *Swarm) removePeer(peer *Peer) (bool, error) {
	for i, p := range s.peers {
		if p == peer {
			s.peers[i] = s.peers[len(s.peers)-1]
			s.peers = s.peers[:len(s.peers)-1]
		}
	}
	go peer.Close()

	return true, nil
}

func (s *Swarm) publish(e Event) {
	var out []chan Event
	for _, subscriber := range s.subscribers {
		if subscriber != nil {
			out = append(out, subscriber)
			go func(s chan Event) {
				defer func() {
					if r := recover(); r != nil {
						fmt.Println("SWARM PUBLISH RECOVERED", r)
					}
				}()
				s <- e
			}(subscriber)
		}
	}

	s.subscribers = out
}

func New(t btorrent.Torrent, out chan Event, peerID [20]byte, baseDir string) Swarm {
	nPieces := len(t.Pieces())

	swarm := Swarm{
		Torrent:    t,
		PeerCh:     make(chan net.Conn, 32),
		eventCh:    make(chan Event, 32),
		stats:      NewStats(t),
		have:       bits.NewBitField(nPieces),
		peerID:     peerID,
		baseDir:    baseDir,
		AnnounceCh: make(chan []tracker.PeerInfo, 32),
		outCh:      out,
		downloadCh: make(chan btorrent.PieceMessage, 32),
	}

	return swarm
}

func (s *Swarm) verifyPieces() error {
	var (
		op         errors.Op = "(*Swarm).init"
		hexHash              = s.HexHash()
		torrentDir           = path.Join(s.baseDir, hexHash)
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
			if ok := s.loadAndVerify(n, piecePath); ok {
				verified++
			}
		}
	}

	infoLogger.Msgf("Verified %d pieces", verified)

	return nil
}

func (s *Swarm) loadAndVerify(index int, piecePath string) bool {
	piece, err := os.ReadFile(piecePath)
	if err != nil {
		log.Err(err).Msgf("failed to load piece piece at %s", piecePath)
		return false
	}

	if !s.VerifyPiece(index, piece) {
		return false
	}

	if err := s.have.Set(index); err != nil {
		log.Err(err).Msg("failed to set bitfield")
		return false
	}

	return true
}
