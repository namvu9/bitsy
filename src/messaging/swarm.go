package messaging

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/namvu9/bitsy/src/bencode"
	"github.com/namvu9/bitsy/src/bits"
	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
)

// A Swarm represents a group of peers (including the
// client) interested in a specific torrent
type Swarm struct {
	torrent.Torrent
	d       DialListener
	baseDir string
	peerID  [20]byte

	peerCh     chan *Peer
	announceCh chan tracker.UDPAnnounceResponse // TODO: REMOVE??

	have bits.BitField // the complete and verified pieces that the client has

	downloaded        uint64
	lastPieceReceived time.Time
	leechers          int
	peerInfo          []tracker.PeerInfo
	peers             map[string]*Peer
	seeders           int
	uploaded          uint64

	downloader *Downloader
}

func (s *Swarm) Stat() string {
	var (
		choked      int
		choking     int
		interested  int
		interesting int
	)

	for _, peer := range s.Peers(nil) {
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
	sb.WriteString(fmt.Sprintf("Downloaded: %s\n", torrent.FileSize(s.downloaded)))
	sb.WriteString(fmt.Sprintf("Uploaded: %s\n", torrent.FileSize(s.uploaded)))

	sb.WriteString(fmt.Sprintf("Seeders: %d\n", s.seeders))
	sb.WriteString(fmt.Sprintf("Leechers: %d\n", s.leechers))
	sb.WriteString(fmt.Sprintf("PeerInfos: %d\n", len(s.peerInfo)))

	sb.WriteString(fmt.Sprintf("%d peers (%d choked, %d interested, %d interesting) in swarm for %s\n", len(s.peers), choked, interested, interesting, s.Name()))
	sb.WriteString(fmt.Sprintf("Choked by %d peers out of %d\n", choking, len(s.peers)))
	//sb.WriteString(fmt.Sprintf("Pending pieces: %d\n", len(s.pendingPieces)))
	sb.WriteString("-------------------------\n")

	s.downloader.Stat()

	return sb.String()
}

func (s *Swarm) Listen() error {
	var op errors.Op = "(*Swarm).Listen"

	err := s.verifyPieces()
	if err != nil {
		return errors.Wrap(err, op)
	}

	var (
		ticker     = time.NewTicker(10 * time.Second)
		downloader = newDownloader(s.Torrent, s.getNRandomPeers, s.randomPieceIndex, s.baseDir)
	)

	s.downloader = &downloader

	go downloader.Init()
	go s.listenForMessages()

	go func(s *Swarm) {
		//var op errors.Op = "(*Swarm).Listen.GoRoutine"
		for {
			select {
			case peer := <-s.peerCh:
				if len(s.peers) > 30 {
					peer.Close()
				} else {
					cpy := s.Peers(nil)
					cpy[peer.RemoteAddr().String()] = peer
					s.peers = cpy

					go peer.SendBitField(BitFieldMessage{s.have.Bytes()})
					go peer.Listen()
				}

			case <-s.announceCh:
			case haveMsg := <-downloader.haveCh:
				s.Broadcast(haveMsg, nil)
				err = s.have.Set(int(haveMsg.Index))
				if err != nil {
					fmt.Println(errors.Wrap(err, op))
				}

			case <-ticker.C:
				go s.Broadcast(KeepAliveMessage{}, func(p Peer) bool {
					return time.Now().Sub(p.LastMessageSent) > 2*time.Minute
				})
				//go s.closeIdleConnections()
				go s.unchokeRandomPeer()
			}
		}
	}(s)

	return nil
}

// Broadcast a message to all peers that satisfy the filter
func (s *Swarm) Broadcast(msg Message, filter func(Peer) bool) {
	for _, peer := range s.Peers(filter) {
		go peer.Send(msg)
	}
}

// TODO: Simplify (it has 6 arguments wtf)
func NewSwarm(t torrent.Torrent, stat chan tracker.UDPAnnounceResponse, peerID [20]byte, baseDir string) Swarm {
	nPieces := len(t.Pieces())

	swarm := Swarm{
		Torrent:           t,
		peerCh:            make(chan *Peer),
		peers:             make(map[string]*Peer),
		have:              bits.NewBitField(nPieces),
		announceCh:        stat,
		peerID:            peerID,
		baseDir:           baseDir,
		lastPieceReceived: time.Now(),
	}

	return swarm
}

// Peers returns a copy of the current peers in
// the swarm.
func (s *Swarm) Peers(filter func(Peer) bool) map[string]*Peer {
	out := make(map[string]*Peer)
	for key, peer := range s.peers {
		if filter != nil && !filter(*peer) {
			continue
		}
		out[key] = peer
	}

	return out
}

func (s *Swarm) listenForMessages() {
	for {
		for _, peer := range s.Peers(nil) {
			select {
			case msg := <-peer.out:
				go s.handleMessage(msg, peer)
			default:
			}
		}
	}
}

// A peer is defined to be 'idle' if no message has
// been received in two minutes or more
func (s *Swarm) closeIdleConnections() {
	//peers := s.getNRandomPeers(10, func(p Peer) bool {
	//return p.Blocking && !p.Interested
	//})

	//for _, peer := range peers {
	//go func() {
	//close(peer.in)
	//}()
	//delete(s.peers, peer.RemoteAddr().String())
	//}
}

// This just actually loads and verifies torrents
func (s *Swarm) verifyPieces() error {
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

	fmt.Println("VERIFNG")

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
			err = s.have.Set(n)
			if err != nil {
				continue
			}

			s.downloaded += uint64(len(piece))

			verified++
		}

		if s.have.GetSum() == len(s.Pieces()) {
			// Reconstitute file
			s.reconstituteFile(torrentDir)
		}
	}

	infoLogger.Msgf("Verified %d pieces", verified)

	return nil
}

func (s *Swarm) reconstituteFile(dir string) error {
	torrent := s.Torrent
	info, _ := torrent.Info()

	var finfo []*bencode.Dictionary

	finfos, _ := info.GetList("files")
	for _, file := range finfos {
		dict, _ := file.ToDict()
		finfo = append(finfo, dict)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
		//return errors.Wrap(err, op, errors.IO)
	}

	data := make([][]byte, len(torrent.Pieces()))

	count := 0
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
			piece, err := os.ReadFile(path.Join(dir, file.Name()))
			if err != nil {
				return err
			}
			data[n] = piece
			//fmt.Printf("writing piece %d to data\n", n)
			count++
		}
	}

	var buf bytes.Buffer
	for _, piece := range data {
		buf.Write(piece)
	}

	sumLength := 0
	offset := 0
	concatData := buf.Bytes()
	for _, finfo := range finfo {
		name, _ := finfo.GetList("path")
		length, _ := finfo.GetInteger("length")

		sumLength += int(length)

		var sb strings.Builder
		for _, segment := range name {
			b, _ := segment.ToBytes()
			sb.Write(b)
		}

		err := os.MkdirAll(path.Join(s.baseDir, torrent.Name()), 0777)
		if err != nil {
			return err
		}

		filePath := path.Join(s.baseDir, torrent.Name(), sb.String())
		err = os.WriteFile(filePath, concatData[offset:offset+int(length)], 0777)
		if err != nil {
			fmt.Println(err)
			return err
		}

		fmt.Printf("Wrote %d B to %s (%d:%d)\n", length, filePath, offset, offset+int(length))
		offset += int(length)
	}

	fmt.Println(sumLength)

	return nil
}

func (s *Swarm) getUnchokedPeers() []*Peer {
	var out []*Peer
	for _, peer := range s.Peers(nil) {
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

	peers := s.getNRandomPeers(3, func(p Peer) bool {
		// if I have all pieces, shouldn't need to be
		// interesting
		return p.Choked
	})

	for _, peer := range peers {
		s.unchokePeer(peer)
	}

	for _, peer := range s.getNRandomPeers(1, func(p Peer) bool {
		return !p.Choked
	}) {
		s.chokePeer(peer)
	}

	return nil
}

func (s *Swarm) chokePeer(p *Peer) {
	_, err := p.Write(ChokeMessage{}.Bytes())
	if err != nil {
		//return errors.Wrap(err, op, errors.Network)
		return
	}

	p.Choke()
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

func (s *Swarm) getNRandomPeers(n int, filter func(Peer) bool) []*Peer {
	var out []*Peer

	peers := s.Peers(filter)

	if len(peers) < n {
		for _, peer := range peers {
			out = append(out, peer)
		}
	} else {
		for len(out) < n {
			for _, peer := range peers {
				ok := rand.Intn(100) < 50
				if ok {
					out = append(out, peer)
				}
			}
		}
	}

	return out
}

// Returns the index of a piece that the client doesn't have
func (s *Swarm) randomPieceIndex() int {
	pieces := s.Pieces()
	var index int
	for {
		index = rand.Intn(len(pieces))
		if !s.have.Get(index) {
			break
		}
	}

	return index
}
