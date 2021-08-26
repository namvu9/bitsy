package tracker

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/namvu9/bitsy/internal/errors"
)

type UDPTracker struct {
	*url.URL
	lastAnnounce time.Time
	interval     time.Duration
	seeders      int
	leechers     int
	peers        []net.Addr
	err          error
	failures     int
}

func (tr *UDPTracker) Stat() TrackerStat {
	return TrackerStat{
		Url:          tr.URL,
		Seeders:      tr.seeders,
		Leechers:     tr.leechers,
		Peers:        tr.peers,
		Err:          tr.err,
		NextAnnounce: tr.lastAnnounce.Add(tr.interval),
	}
}

func (tr *UDPTracker) Announce(req Request) (*Response, error) {
	var op errors.Op = "tracker.UDPAnnounce"

	connID, err := tr.Connect()
	if err != nil {
		tr.scheduleRetry(err)
		return nil, errors.Wrap(err, op)
	}

	ureq := UDPRequest{
		ConnID: connID,
		Action: ANNOUNCE,
		TxID:   rand.Uint32(),

		Request: req,
	}

	conn, err := net.DialTimeout("udp", tr.URL.Host, 500*time.Millisecond)
	if err != nil {
		tr.scheduleRetry(err)
		return nil, errors.Wrap(err, op, errors.Network)
	}
	conn.SetWriteDeadline(time.Now().Add(500*time.Millisecond))
	conn.SetReadDeadline(time.Now().Add(500*time.Millisecond))

	defer conn.Close()

	err = binary.Write(conn, binary.BigEndian, ureq)
	if err != nil {
		tr.scheduleRetry(err)
		return nil, errors.Wrap(err, op, errors.Network)
	}

	var res Response
	rcvBuf := make([]byte, 1024)
	n, err := conn.Read(rcvBuf)

	err = unmarshalResponse(rcvBuf[:n], &res)
	if err != nil {
		tr.scheduleRetry(err)
		return nil, err
	}

	tr.lastAnnounce = time.Now()
	tr.interval = time.Second * time.Duration(res.Interval)
	tr.leechers = int(res.NLeechers)
	tr.seeders = int(res.NSeeders)
	tr.err = nil
	tr.failures = 0

	for _, peer := range res.Peers {
		tr.peers = append(tr.peers, &net.TCPAddr{IP: peer.IP, Port: int(peer.Port)})
	}

	return &res, nil
}

func (tr *UDPTracker) Err() error {
	return tr.err
}

func (tr *UDPTracker) ShouldAnnounce() bool {
	nextAnnounce := tr.lastAnnounce.Add(tr.interval)
	if time.Now().Before(nextAnnounce) {
		return false
	}

	return true
}

func (tr *UDPTracker) Connect() (uint64, error) {
	var op errors.Op = "tracker.UDPConnect"
	conn, err := net.DialTimeout("udp", tr.URL.Host, 500*time.Millisecond)
	if err != nil {
		return 0, err
	}

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	if err != nil {
		tr.scheduleRetry(err)

		if nerr, ok := err.(net.Error); ok && !nerr.Temporary() {
			return 0, errors.Wrap(err, op, errors.Network)
		}

		return 0, err
	}

	defer conn.Close()

	req := newConnReq()
	err = binary.Write(conn, binary.BigEndian, req)
	if err != nil {
		tr.scheduleRetry(err)
		return 0, errors.Wrap(err, op, errors.Network)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var res ConnMessage
	err = binary.Read(conn, binary.BigEndian, &res)
	if err != nil {
		tr.scheduleRetry(err)
		return 0, errors.Wrap(err, op)
	}

	err = ValidateConnection(req, res)
	if err != nil {
		tr.scheduleRetry(err)
		return 0, err
	}

	return res.ConnID, nil
}

func (tr *UDPTracker) scheduleRetry(e error) {
	tr.err = e
	tr.interval = time.Duration(15 * math.Pow(2.0, float64(tr.failures)))
	tr.failures++
}

func ValidateConnection(req ConnectReq, res ConnMessage) error {
	if req.TxID != res.TxID {
		err := fmt.Errorf("Transaction IDs do not match: want %d got %d", req.TxID, res.TxID)
		return errors.Wrap(err, errors.Internal)
	}

	if res.Action != req.Action {
		err := fmt.Errorf("Actions do not match: want %d got %d", req.Action, res.Action)
		return errors.Wrap(err, errors.Internal)
	}

	return nil
}

// ConnectReq represents the structure that constitutes the
// connect portion of the UDP tracker prtocol
type ConnectReq struct {
	ProtocolID uint64
	Action     uint32
	TxID       uint32
}

const UDP_PROTOCOL_ID = 0x41727101980

func newConnReq() ConnectReq {
	return ConnectReq{
		Action:     CONNECT,
		ProtocolID: UDP_PROTOCOL_ID, // Magic constant
		TxID:       rand.Uint32(),
	}
}

type ConnMessage struct {
	Action uint32
	TxID   uint32
	ConnID uint64
}

func unmarshalResponse(data []byte, v *Response) error {
	if len(data) < 20 {
		return fmt.Errorf("Invalid tracker response %d", len(data))
	}

	v.Action = binary.BigEndian.Uint32(data[:4])

	if v.Action == ERROR {
		return errors.New(string(data[4:]))
	}

	if v.Action != ANNOUNCE {
		return fmt.Errorf("Expected action %d but got %d", ANNOUNCE, v.Action)
	}

	v.TxID = binary.BigEndian.Uint32(data[4:8])
	v.Interval = binary.BigEndian.Uint32(data[8:12])
	v.NLeechers = binary.BigEndian.Uint32(data[12:16])
	v.NSeeders = binary.BigEndian.Uint32(data[16:20])

	offset := 20
	for len(data[offset:]) >= 6 {
		v.Peers = append(v.Peers, PeerInfo{
			IP:   data[offset : offset+4],
			Port: binary.BigEndian.Uint16(data[offset+4 : offset+6]),
		})

		offset += 6
	}

	return nil
}

func NewUDPTracker(url *url.URL) *UDPTracker {
	return &UDPTracker{
		URL:          url,
		lastAnnounce: time.Now(),
	}
}
