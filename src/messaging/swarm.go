package messaging

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/namvu9/bitsy/src/bits"
	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
	"github.com/rs/zerolog/log"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	torrent.Torrent

	d DialListener

	// Path to the base directory where data is downloaded
	baseDir string

	peers    map[string]*Peer
	peerInfo []tracker.PeerInfo

	peerCh     chan *Peer
	closeCh    chan *Peer
	announceCh chan tracker.UDPAnnounceResponse
	outCh      chan map[string]interface{}

	leechers int
	seeders  int

	msgInCh  chan Message
	msgOutCh chan Message

	// the complete and verified pieces that the client has
	have bits.BitField

	pendingPieces map[uint32]map[uint32][]byte

	peerID [20]byte

	Downloaded uint64
	Uploaded   uint64

	LastPieceReceived time.Time
}

func (s *Swarm) Stat() string {
	var (
		choked      int
		choking     int
		interested  int
		interesting int
	)
	for _, peer := range s.peers {
		if peer.Choked {
			choked++
		}

		if peer.Blocking {
			choking++
		}

		if peer.Interested {
			interested++
		}

		if peer.Interesting {
			interesting++
		}
	}

	totalNPieces := len(s.Pieces())
	haveNPieces := s.have.GetSum()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("\n------\nTorrent: %s\n-----\n", s.Name()))
	sb.WriteString(fmt.Sprintf("Size: %s\n", torrent.FileSize(s.Length())))
	sb.WriteString(fmt.Sprintf("Have %d pieces out of %d (%.2f %%)\n", haveNPieces, totalNPieces, float32(haveNPieces)*100/float32(totalNPieces)))
	sb.WriteString(fmt.Sprintf("Downloaded: %d B\n", s.Downloaded))
	sb.WriteString(fmt.Sprintf("Uploaded: %d B\n", s.Uploaded))

	sb.WriteString(fmt.Sprintf("Seeders: %d\n", s.seeders))
	sb.WriteString(fmt.Sprintf("Leechers: %d\n", s.leechers))
	sb.WriteString(fmt.Sprintf("PeerInfos: %d\n", len(s.peerInfo)))

	sb.WriteString(fmt.Sprintf("%d peers (%d choked, %d interested, %d interesting) in swarm for %s\n", len(s.peers), choked, interested, interesting, s.Name()))
	sb.WriteString(fmt.Sprintf("Choked by %d peers out of %d\n", choking, len(s.peers)))
	sb.WriteString(fmt.Sprintf("Pending pieces: %d\n", len(s.pendingPieces)))
	sb.WriteString("-------------------------\n")

	return sb.String()
}

// TODO: Connect to multiple new peers
func (s *Swarm) connectNewPeer() error {
	var op errors.Op = "(*Swarm).connectNewPeer"

	if len(s.peerInfo) > 0 {
		peerInfo := s.peerInfo[0]
		s.peerInfo = s.peerInfo[1:]

		addr := fmt.Sprintf("%s:%d", peerInfo.IP, peerInfo.Port)

		fmt.Println("iniatiginat connection")
		log.Info().Str("op", op.String()).Msgf("Initiating connection to %s\n", addr)

		conn, err := s.d.Dial("tcp", addr)
		if err != nil {
			return errors.Wrap(err, op, errors.Network)
		}

		peerConn, err := Handshake(conn, *s.InfoHash(), s.peerID)
		if err != nil {
			fmt.Println("CONNECT NEW HANDSHAKE FAILED")
			return errors.Wrap(err, op, errors.Network)
		}

		s.peerCh <- peerConn

		log.Info().Str("op", op.String()).Msgf("Connected to %s\n", peerConn.RemoteAddr())
	}

	return nil
}

// A peer is defined to be 'idle' if no message has
// been received in two minutes or more
func (s *Swarm) closeIdleConnections() {
	for _, peer := range s.peers {
		if time.Now().Sub(peer.LastMessageReceived) > 2*time.Minute {
			s.closeCh <- peer
		}
	}
}

func (s *Swarm) init() error {
	var (
		op         errors.Op = "(*Swarm).init"
		hexHash              = s.HexHash()
		torrentDir           = path.Join(s.baseDir, hexHash)
		infoLogger           = log.Info().Str("torrent", hexHash).Str("op", op.String())
	)

	files, err := os.ReadDir(torrentDir)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	infoLogger.Msg("Initializing torrent")
	infoLogger.Msgf("Verifying %d pieces", len(files))

	var verified int
	for _, file := range files {
		matchString := fmt.Sprintf(`(\d+).part`)
		re := regexp.MustCompile(matchString)
		name := strings.ToLower(file.Name())

		if re.MatchString(name) {
			matches := re.FindStringSubmatch(name)
			index := matches[1]
			n, err := strconv.Atoi(index)
			if err != nil {
				return err
			}

			// LOAD AND VERIFY PIECE
			piece, err := os.ReadFile(path.Join(torrentDir, file.Name()))
			if err != nil {
				return errors.Wrap(err, op, errors.IO)
			}

			if !s.VerifyPiece(n, piece) {
				continue
			}

			// Add size to `downloaded`
			// downloaded / s.Torrent.Length()
			err = s.have.SetIndex(n)
			if err != nil {
				continue
			}

			s.Downloaded += uint64(len(piece))

			verified++
		}
	}

	s.outCh <- map[string]interface{}{
		"downloaded": s.Downloaded,
	}
	infoLogger.Msgf("Verified %d pieces", verified)

	return nil
}

func (s *Swarm) getUnchokedPeers() []*Peer {
	var out []*Peer
	for _, peer := range s.peers {
		if !peer.Choked {
			out = append(out, peer)
		}
	}
	return out
}

// Optimistically unchokes a randomly chosen choked peer
func (s *Swarm) unchokeRandomPeer() error {
	unchoked := s.getUnchokedPeers()
	if len(unchoked) > 10 {
		return nil
	}

	peer, ok := s.getRandomPeer(func(p *Peer) bool {
		return p.Choked && p.Interesting
	})

	if ok {
		s.unchokePeer(peer)
	}

	return nil
}

func (s *Swarm) unchokePeer(p *Peer) error {
	var op errors.Op = "(*Swarm).unchokePeer"

	_, err := p.Write(UnchokeMessage{}.Bytes())
	if err != nil {
		return errors.Wrap(err, op, errors.Network)
	}

	p.Unchoke()

	return nil
}

func (s *Swarm) getNRandomPeers(filter func(*Peer) bool, n int) []*Peer {
	var out []*Peer

	for len(out) < n {
		peer, ok := s.getRandomPeer(filter)
		if ok {
			out = append(out, peer)
		}
	}

	return out
}
func (s *Swarm) requestSubPiece(index int32, offset int32, p *Peer) error {
	var op errors.Op = "(*Swarm).requestSubPiece"

	remaining := int32(s.PieceLength()) - offset
	subPieceLength := int32(16 * 1024)

	msg := RequestMessage{
		Index:  index,
		Offset: int32(offset),
	}

	if remaining < subPieceLength {
		msg.Length = int32(remaining)
	} else {
		msg.Length = int32(subPieceLength)
	}

	// Skip if we've already asked the peer for this sub-piece
	for _, request := range p.requests {
		if request == msg {
			return nil
		}
	}

	_, err := p.Write(msg.Bytes())
	if err != nil {
		return errors.Wrap(err, op)
	}

	p.requests = append(p.requests, msg)
	return nil
}

func (s *Swarm) requestPieces() {
	var op errors.Op = "(*Swarm).requestPieces"

	if len(s.pendingPieces) < 10 {
		for i := 0; i < 5; i++ {
			pieces := s.Pieces()
			// Randomly pick a piece to download
			// Later, download rarest piece first
			var index int
			for {
				index = rand.Intn(len(pieces))
				if !s.have.IsIndexSet(index) {
					break
				}
			}

			peers := s.getNRandomPeers(func(p *Peer) bool {
				if p.BitField != nil && p.BitField.IsIndexSet(index) {
					return true
				}

				return false
			}, 2)

			log.Info().Str("torrent", s.HexHash()).Msgf("Requesting piece %d from %d peers\n", index, len(peers))
			for _, p := range peers {
				err := s.RequestPiece(int32(index), p)
				if err != nil {
					err = errors.Wrap(err, op)
					log.Error().Msg(err.Error())
				}

			}

		}
	}

	// Compute rarest pieces

}

func (s *Swarm) Listen() error {
	var op errors.Op = "(*Swarm).Listen"

	err := s.init()
	if err != nil {
		return errors.Wrap(err, op)
	}

	ticker := time.NewTicker(10 * time.Second)
	longTicker := time.NewTicker(30 * time.Second)

	go func(s *Swarm) {
		var op errors.Op = "(*Swarm).Listen.GoRoutine"
		for {
			select {

			case peer := <-s.closeCh:
				go func() {
					peer.Close()
					// TODO: Mutex
					delete(s.peers, peer.RemoteAddr().String())
				}()

			// Handshake completed
			case peer := <-s.peerCh:
				fmt.Println("GETTING NEW PEER")
				s.peers[peer.RemoteAddr().String()] = peer
				_, err := peer.Write(BitFieldMessage{
					BitField: s.have.Bytes(),
				}.Bytes())
				if err != nil {
					err := errors.Wrap(err, op)
					log.Err(err).Msgf("could not send bitfield to %s", peer.RemoteAddr())
				}

				// TODO: handle incoming messages via a channel at the
				// swarm level
				go peer.Listen(s.handleMessage)

			case announce := <-s.announceCh:
				s.leechers = int(announce.NLeechers)
				s.seeders = int(announce.NSeeders)

				var peers []tracker.PeerInfo
				for _, peer := range announce.Peers {
					if peer.IP.IsUnspecified() {
						continue
					}

					peers = append(peers, peer)
				}

				fmt.Println(len(announce.Peers), len(peers))

				s.peerInfo = append(s.peerInfo, peers...)

			case <-longTicker.C:

			case <-ticker.C:
				// TODO: CLEAN THIS UP
				if time.Now().Sub(s.LastPieceReceived) > 40*time.Second {
					fmt.Println("<<<<<<<<< Haven't received a piece in 40 secs, Deleting pending")
				} else if time.Now().Sub(s.LastPieceReceived) > 20*time.Second {
					fmt.Println("<<<<<<<<< Haven't received a piece in 20 secs, broadcasting request")
					subPieceLength := int32(16 * 1024)

					for index, piece := range s.pendingPieces {
						var totalSize int
						for _, subpiece := range piece {
							totalSize += len(subpiece)
						}
						left := s.PieceLength() - uint64(totalSize)
						var offsets []int32

						for offset := int32(0); offset < int32(s.PieceLength()); offset += subPieceLength {
							if _, ok := piece[uint32(offset)]; !ok {
								msg := RequestMessage{
									Index:  int32(index),
									Offset: offset,
									Length: int32(subPieceLength),
								}

								// If left < multiple of subpiece length it
								// cannot be in the middle , however, if
								// left > multiple of subpiece, and we ask
								// for the last subpiece that is smaller
								// than multiple, then boom, but only a
								// problem with the very last piece
								if left < s.PieceLength() {
									msg.Length = int32(left)
								}

								s.Broadcast(msg, func(p *Peer) bool {
									return p.Interesting && !p.Blocking
								})

								offsets = append(offsets, offset/subPieceLength)
							}
						}
						fmt.Printf("PENDING: %d subpieces (%d B); left %d; %d - %v\n", len(piece), totalSize, left, left/16*1024, offsets)
					}
				}

				// Send Keep-alives
				go s.Broadcast(KeepAliveMessage{}, func(p *Peer) bool {
					return time.Now().Sub(p.LastMessageSent) > 2*time.Minute
				})

				s.requestPieces()
				go s.connectNewPeer()
				//go s.closeIdleConnections()
				go s.unchokeRandomPeer()

				// Go through peerInfo list
			}
		}
	}(s)

	return nil
}

// TODO: Implement
// Returns a peer and a boolean indicating whether an
// interesting and non-blocking peer could be found. peer is
// nil otherwise.
func (s *Swarm) getRandomPeer(filter func(*Peer) bool) (*Peer, bool) {
	for _, peer := range s.peers {
		if rand.Intn(100) < 50 {
			continue
		}

		if filter(peer) {
			return peer, true
		}
	}

	return nil, false
}

func (s *Swarm) ShowInterest(p *Peer) error {
	var op errors.Op = "(*Swarm).ShowInterest"

	if p.Interesting {
		return nil
	}

	p.Interesting = true
	_, err := p.Write(InterestedMessage{}.Bytes())
	if err != nil {
		return errors.Wrap(err, op)
	}

	return nil
}

func (s *Swarm) RequestPiece(index int32, p *Peer) error {
	var op errors.Op = "(*Swarm).RequestPiece"

	pieceLength := s.PieceLength()
	subPieceLength := 16 * 1024

	for offset := int32(0); offset < int32(pieceLength); offset += int32(subPieceLength) {
		err := s.requestSubPiece(index, offset, p)
		if err != nil {
			return errors.Wrap(err, op)
		}
	}

	return nil
}

// Broadcast a message to all peers that satisfy the filter
func (s *Swarm) Broadcast(msg Message, filter func(*Peer) bool) {
	fmt.Printf("broadcasting: %+v\n", msg)
	for _, peer := range s.peers {
		if filter != nil && !filter(peer) {
			continue
		}

		peer.Write(msg.Bytes())
	}
}

func (s *Swarm) handleMessage(p *Peer, message []byte, err error) {
	if err != nil {
		if nerr, ok := err.(net.Error); ok && !nerr.Timeout() || err == io.EOF {
			log.Err(err).Msg("failed to handle message")
			s.closeCh <- p
		}
		return
	}

	messageType := message[0]

	switch messageType {
	case BitField:
		err = s.handleBitfieldMessage(p, message[1:])
	case Choke:
		err = s.handleChokeMessage(p)
	case Unchoke:
		err = s.handleUnchokeMessage(p)
	case Interested:
		err = s.handleInterestedMessage(p)
	case NotInterested:
		err = s.handleNotInterestedMessage(p)
	case Have:
		err = s.handleHaveMessage(p, message[1:])
	case Request:
		err = s.handleRequestMessage(p, message[1:])
	case Piece:
		s.LastPieceReceived = time.Now()
		err = s.handlePieceMessage(p, message[1:])
	case Cancel:
		err = s.handleCancelMessage(p, message[1:])
	case Extended:
		err = s.handleExtendedMessage(p, message[1:])
	default:
		fmt.Printf("(%s) Unknown message type: %d (%d bytes)\n", p.RemoteAddr(), messageType, len(message))
	}

	if err != nil {
		log.Err(err).Msg("failed to handle message")
	}
}
func (s *Swarm) handleBitfieldMessage(peer *Peer, payload []byte) error {
	var op errors.Op = "(*Swarm).handleBitfieldMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(BitField)).
		Msgf("Received BitField message from %s", peer.RemoteAddr())

	var (
		maxIndex = bits.GetMaxIndex(payload)
		pieces   = s.Pieces()
	)

	if maxIndex >= len(pieces) {
		err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		return errors.Wrap(err, op, errors.BadArgument)
	}

	peer.BitField = payload

	for i := range pieces {
		if !s.have.IsIndexSet(i) && peer.BitField.IsIndexSet(i) {
			s.ShowInterest(peer)
			return nil
		}
	}

	return nil
}

func (s *Swarm) handleChokeMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Choke)).
		Msgf("Received Choke message from %s", p.RemoteAddr())

	p.Blocking = true
	p.requests = []RequestMessage{}
	return nil
}

func (s *Swarm) handleUnchokeMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Unchoke)).
		Msgf("Received Unchoke message from %s", p.RemoteAddr())

	p.Blocking = false
	return nil
}

func (s *Swarm) handleInterestedMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(NotInterested)).
		Msgf("Received NotInterested message from %s", p.RemoteAddr())

	p.Interested = true
	return nil
}

func (s *Swarm) handleNotInterestedMessage(p *Peer) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(NotInterested)).
		Msgf("Received NotInterested message from %s", p.RemoteAddr())

	p.Interested = false
	return nil
}

// TODO: Parse message before sending to handler
func (s *Swarm) handleHaveMessage(p *Peer, payload []byte) error {
	var op errors.Op = "(*Swarm).handleHaveMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Have)).
		Msgf("Received Have message from %s", p.RemoteAddr())

	if len(payload) != 4 {
		err := errors.Newf("have message payload longer than 4: %d", len(payload))
		return errors.Wrap(err, op, errors.BadArgument)
	}

	index := binary.BigEndian.Uint32(payload)
	if !s.have.IsIndexSet(int(index)) && !p.Blocking {
		err := s.ShowInterest(p)
		if err != nil {
			return errors.Wrap(err, op)
		}
	}

	return nil
}

func (s *Swarm) handleRequestMessage(p *Peer, payload []byte) error {
	var op errors.Op = "(*Swarm).handleRequestMessage"

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Request)).
		Msgf("Received request message from %s", p.RemoteAddr())

	index := binary.BigEndian.Uint32(payload[:4])
	offset := binary.BigEndian.Uint32(payload[4:8])
	length := binary.BigEndian.Uint32(payload[8:12])

	filePath := path.Join(s.baseDir, s.HexHash(), fmt.Sprintf("%d.part", index))
	fmt.Printf("Request: Index: %d, offset: %d, length: %d (path: %s)\n", index, offset, length, filePath)

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	data = data[offset : offset+length]

	msg := PieceMessage{
		Index:  int32(index),
		Offset: int32(offset),
		Piece:  data,
	}

	n, err := p.Write(msg.Bytes())
	if err != nil {
		return errors.Wrap(err, op)
	}

	s.Uploaded += uint64(n)

	return nil
}

func (s *Swarm) handlePieceMessage(p *Peer, payload []byte) error {
	var op errors.Op = "(*Swarm).handlePieceMessage"

	pieceLength := s.PieceLength()
	subPieceLength := 16 * 1024
	expectedNPieces := (int(pieceLength) / subPieceLength)
	if pieceLength%uint64(subPieceLength) != 0 {
		expectedNPieces++
	}

	var (
		index  = binary.BigEndian.Uint32(payload[:4])
		offset = binary.BigEndian.Uint32(payload[4:8])
		piece  = payload[8:]
	)

	pendingPieces, ok := s.pendingPieces[index]
	if !ok {
		// If it exists and we don't have it, it's all good
		if !s.have.IsIndexSet(int(index)) {
			pendingPieces = make(map[uint32][]byte)
			s.pendingPieces[index] = pendingPieces
		}
		return nil
	}
	pendingPieces[offset] = piece

	// TODO:
	// Not safe
	s.Downloaded += uint64(len(piece))
	p.Uploaded += int64(len(piece))

	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Piece)).
		Msgf("Received Piece message (index %d, offset %d, length %d / %d) from %s\n", index, offset, len(piece), pieceLength, p.RemoteAddr())

	if len(pendingPieces) == expectedNPieces {
		file, err := os.Create(fmt.Sprintf("%s/%s/%d.part", s.baseDir, s.HexHash(), index))
		if err != nil {
			return errors.Wrap(err, op, errors.IO)
		}

		defer file.Close()

		for offset, piece := range pendingPieces {
			_, err := file.WriteAt(piece, int64(offset))
			if err != nil {
				return errors.Wrap(err, op, errors.IO)
			}
		}

		delete(s.pendingPieces, index)
		piece, err := io.ReadAll(file)
		if err != nil {
			return errors.Wrap(err, op, errors.IO)
		}

		if !s.VerifyPiece(int(index), piece) {
			err := errors.Newf("completed piece was corrupted %d", index)
			return errors.Wrap(err, op)
		}

		err = s.have.SetIndex(int(index))
		if err != nil {
			return errors.Wrap(err, op)
		}

		s.Broadcast(HaveMessage{Index: int32(index)}, nil)

		log.Info().
			Str("torrent", s.HexHash()).
			Int("messageType", int(Piece)).
			Msgf("Wrote piece %d to disk (%d B)", index, len(piece))
	}

	return nil
}

func (s *Swarm) handleCancelMessage(p *Peer, payload []byte) error {
	fmt.Println("HANDLE CANCEL")
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Cancel)).
		Msgf("Received cancel message from %s", p.RemoteAddr())

	return nil
}

func (s *Swarm) handleExtendedMessage(p *Peer, payload []byte) error {
	log.Info().
		Str("torrent", s.HexHash()).
		Int("messageType", int(Extended)).
		Msgf("Received extended message from %s", p.RemoteAddr())

	return nil
}

func NewSwarm(t torrent.Torrent, stat chan tracker.UDPAnnounceResponse, d DialListener, peerID [20]byte, baseDir string, out chan map[string]interface{}) Swarm {
	pieces := t.Pieces()
	bitFieldLength := len(pieces) / 8
	if bitFieldLength%8 != 0 {
		bitFieldLength++
	}

	swarm := Swarm{
		Torrent:           t,
		peerCh:            make(chan *Peer),
		peers:             make(map[string]*Peer),
		closeCh:           make(chan *Peer, 30),
		pendingPieces:     make(map[uint32]map[uint32][]byte),
		have:              make(bits.BitField, bitFieldLength),
		announceCh:        stat,
		outCh:             out,
		d:                 d,
		peerID:            peerID,
		baseDir:           baseDir,
		LastPieceReceived: time.Now(),
	}

	return swarm
}
