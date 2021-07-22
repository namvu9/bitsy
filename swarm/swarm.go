package swarm

import (
	"fmt"
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

	//peers *Peers

	baseDir string
	peerID  [20]byte

	have bits.BitField // the complete and verified pieces that the client has

	PeerCh  chan net.Conn
	eventCh chan Event

	// Out
	workers     []chan Event // prioritize workers over subscribers
	subscribers []chan Event

	// Only the swarm gets to access peers
	peers []*Peer

	stats      *Stats
	announceCh chan tracker.UDPAnnounceResponse
}

func (s *Swarm) Stat() map[string]interface{} {
	stats := make(map[string]interface{})

	stats["torrent"] = s.HexHash()
	stats["npeers"] = len(s.peers)
	stats["have"] = s.have.GetSum()
	stats["npieces"] = len(s.Pieces()) 

	swarmStats := <-s.stats.outCh
	stats["stats"] = swarmStats.(StatEvent).payload

	return stats
}

func (s *Swarm) Init() error {
	var op errors.Op = "(*Swarm).Init"
	err := s.verifyPieces()
	if err != nil {
		return errors.Wrap(err, op)
	}

	s.Subscribe(s.stats)

	go s.listen()

	return nil
}

func (sw *Swarm) Subscribe(sub Subscriber) {
	out := sub.Subscribe(sw.eventCh)
	sw.subscribers = append(sw.subscribers, out)
}

func (s *Swarm) listen() {
	for {
		select {
		case announce := <-s.announceCh:
			var peers []map[string]string

			for _, peer := range announce.Peers {
				peers = append(peers, map[string]string{
					"ip":   peer.IP.String(),
					"port": fmt.Sprint(peer.Port),
				})
			}
			s.broadcastEvent(TrackerAnnounceEvent{
				timestamp: time.Now(),
				seeders:   int(announce.NSeeders),
				leechers:  int(announce.NLeechers),
			})

		case conn := <-s.PeerCh:
			event := JoinEvent{NewPeer(conn)}
			go func() {
				s.eventCh <- event
			}()
		case event := <-s.eventCh:
			propagate, err := s.handleEvent(event)
			if err != nil {
				fmt.Println(err)
				continue
			}

			if propagate {
				s.broadcastEvent(event)
			}
		}
	}
}

func (s *Swarm) addPeer(peer *Peer) (bool, error) {
	s.Subscribe(peer)
	s.peers = append(s.peers, peer)

	// Send bitfield
	event := BitFieldEvent{
		BitFieldMessage: btorrent.BitFieldMessage{BitField: s.have},
	}
	go peer.Send(event)

	return true, nil
}

func (s *Swarm) removePeer(peer *Peer) (bool, error) {
	var out []*Peer
	for _, p := range s.peers {
		if p != peer {
			out = append(out, p)
		}
	}

	return true, nil
}

func (s *Swarm) broadcastEvent(e Event) {
	for _, subscriber := range s.subscribers {
		go func(s chan Event) {
			s <- e
		}(subscriber)
	}
}

func New(t btorrent.Torrent, stat chan tracker.UDPAnnounceResponse, peerID [20]byte, baseDir string) Swarm {
	nPieces := len(t.Pieces())

	stats := &Stats{
		eventCh: make(chan Event, 32),
		outCh:   make(chan Event, 32),
	}

	swarm := Swarm{
		Torrent:    t,
		PeerCh:     make(chan net.Conn, 32),
		eventCh:    make(chan Event, 32),
		stats:      stats,
		have:       bits.NewBitField(nPieces),
		peerID:     peerID,
		baseDir:    baseDir,
		announceCh: stat,
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

			err = s.have.Set(n)
			if err != nil {
				continue
			}

			//go s.broadcastEvent(DownloadCompleteEvent{
			//HaveMessage: btorrent.HaveMessage{Index: uint32(n)},
			//FileSize:    btorrent.FileSize(len(piece)),
			//})

			verified++
		}
	}

	infoLogger.Msgf("Verified %d pieces", verified)

	return nil
}
