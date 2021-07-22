package tracker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/errors"
)

// UDP Tracker Actions
const (
	CONNECT  uint32 = 0
	ANNOUNCE uint32 = 1
	SCRAPE   uint32 = 2
	ERROR    uint32 = 3
)

func UDPConnect(url *url.URL, d Dialer) (connID uint64, err error) {
	var op errors.Op = "tracker.UDPConnect"
	conn, err := d.Dial("udp", url.Host)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && !nerr.Temporary() {
			return 0, errors.Wrap(err, op, errors.Network)
		}

		return 0, err
	}

	defer conn.Close()

	req := newConnReq()
	data, err := req.Bytes()
	if err != nil {
		return 0, errors.Wrap(err, op)
	}

	_, err = conn.Write(data)
	if err != nil {
		return 0, errors.Wrap(err, op, errors.Network)
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var res ConnectReq
	if err := UnmarshalConnectMsg(conn, &res); err != nil {
		return 0, errors.Wrap(err, op)
	}

	if req.TxID != res.TxID {
		err := fmt.Errorf("Transaction IDs do not match: want %d got %d", req.TxID, res.TxID)
		return 0, errors.Wrap(err, op, errors.Internal)
	}

	if res.Action != req.Action {
		err := fmt.Errorf("Actions do not match: want %d got %d", req.Action, res.Action)
		return 0, errors.Wrap(err, op, errors.Internal)
	}

	return res.ProtocolID, nil
}

type PeerInfo struct {
	IP   net.IP
	Port uint16
}

func UDPAnnounce(url *url.URL, req UDPAnnounceReq, d Dialer) (*UDPAnnounceResponse, error) {
	var op errors.Op = "tracker.UDPAnnounce"

	conn, err := d.Dial("udp", url.Host)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.Network)
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	defer conn.Close()

	data, err := req.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	_, err = conn.Write(data)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.Network)
	}

	var res UDPAnnounceResponse
	if err := UnmarshalUDPAnnounceResp(conn, &res); err != nil {
		return nil, errors.Wrap(err, op)
	}

	return &res, nil
}

type TrackerGroup struct {
	btorrent.Torrent

	Downloaded uint64
	Keft       uint64

	outCh chan UDPAnnounceResponse
	inCh  chan map[string]interface{}

	stats  map[string]*TrackerStat
	peerID [20]byte

	Port uint16
}

type Dialer interface {
	Dial(string, string) (net.Conn, error)
}

func (t *TrackerGroup) Listen(d Dialer) (chan UDPAnnounceResponse, chan map[string]interface{}) {
	if t.outCh != nil {
		return t.outCh, t.inCh
	}

	t.outCh = make(chan UDPAnnounceResponse, 30)
	t.inCh = make(chan map[string]interface{}, 30)

	go func(outCh chan UDPAnnounceResponse) {
		var op errors.Op = "(*TrackerGroup).Listen.GoFunc"

		for {
			n := 0
			for _, tier := range t.AnnounceList() {
				for _, tracker := range tier {
					if n >= 30 {
						time.Sleep(30 * time.Second)
						n = 0
					}
					stat := t.stats[tracker]

					if stat.Failures > 8 {
						continue
					}

					nextAnnounce := stat.LastAnnounce.Add(time.Duration(stat.Interval) * time.Second)
					if time.Now().Before(nextAnnounce) {
						continue
					}

					url, err := url.Parse(tracker)
					if err != nil {
						log.Err(err).Strs("trace", []string{string(op)}).Msg("Invalid tracker url")
						stat.Failures = 9999
						continue
					}

					n++
					go func(tracker string) {
						connID, err := UDPConnect(url, d)
						if err != nil {
							err = errors.Wrap(err, op)
							log.Err(err).Strs("trace", errors.Ops(err)).Msg("failed to connect to UDP tracker")
							return
						}

						log.Print("Announcing", tracker)
						req := NewUDPAnnounceReq(t.InfoHash(), connID, t.Port, t.peerID)
						select {
						case data := <-t.inCh:
							req.Downloaded = data["downloaded"].(uint64)
							t.Downloaded = req.Downloaded
						default:
							req.Downloaded = t.Downloaded
						}

						resp, err := UDPAnnounce(url, req, d)
						if err != nil {
							err = errors.Wrap(err, op)
							log.Err(err).Strs("trace", errors.Ops(err)).Msg("faileed to announce over UDP tracker")
							return
						}
						log.Printf("Discovered %d peers from %s\n", len(resp.Peers), tracker)


						stat := t.stats[tracker]
						stat.Interval = int(resp.Interval)
						t.outCh <- *resp
					}(tracker)
				}
			}
		}
	}(t.outCh)

	return t.outCh, t.inCh
}

type TrackerStat struct {
	LastAnnounce time.Time // Time of last successful announce
	Interval     int
	Failures     int // Number of unsuccessful connection/announce attempts
}

func NewTracker(t btorrent.Torrent, port uint16, peerID [20]byte) *TrackerGroup {
	stats := make(map[string]*TrackerStat)

	for _, tier := range t.AnnounceList() {
		for _, tracker := range tier {
			stats[tracker] = &TrackerStat{}
		}
	}

	return &TrackerGroup{
		Torrent: t,
		stats:   stats,
		Port:    port,
		peerID:  peerID,
	}
}
// TODO: TEST
func NewUDPAnnounceReq(hash [20]byte, connID uint64, port uint16, peerID [20]byte) UDPAnnounceReq {
	return UDPAnnounceReq{
		TxID:   rand.Uint32(),
		Want:   -1,
		PeerID: peerID,
		Hash:   hash,
		ConnID: connID,
		Port:   port,
	}
}

type UDPAnnounceResponse struct {
	Action    uint32
	TxID      uint32
	Interval  uint32
	NLeechers uint32
	NSeeders  uint32
	Peers     []PeerInfo
}

func UnmarshalUDPAnnounceResp(r io.Reader, v *UDPAnnounceResponse) error {
	var op errors.Op = "tracker.UnmarshalUDPAnnounceResp"
	data := make([]byte, 1024)

	n, err := io.ReadAtLeast(r, data, 20)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	var (
		action = binary.BigEndian.Uint32(data[:4])
		txid   = binary.BigEndian.Uint32(data[4:8])
	)

	if action == ERROR {
		message := string(data[8:])
		return errors.Wrap(errors.New(message), op, errors.IO)
	}

	if action != ANNOUNCE {
		err := errors.Newf("Expected response action to be %d but got %d", ANNOUNCE, action)
		return errors.Wrap(err, op)
	}

	var (
		interval  = binary.BigEndian.Uint32(data[8:12])
		nleechers = binary.BigEndian.Uint32(data[12:16])
		nseeders  = binary.BigEndian.Uint32(data[16:20])
	)

	var peers []PeerInfo

	if n > 20 {
		data = data[:n]
		offset := 20
		for len(data[offset:]) >= 48 {
			var ip = make(net.IP, 4)
			_ip := binary.BigEndian.Uint32(data[offset : offset+32])
			binary.BigEndian.PutUint32(ip, _ip)

			port := data[offset+32 : offset+48]
			_port := binary.BigEndian.Uint16(port)

			peers = append(peers, PeerInfo{IP: ip, Port: _port})
			offset += 48
		}

	}

	v.Action = action
	v.TxID = txid
	v.Interval = interval
	v.NLeechers = nleechers
	v.NSeeders = nseeders
	v.Peers = peers

	return nil
}
// ConnectReq represents the structure that constitutes the
// connect portion of the UDP tracker prtocol
type ConnectReq struct {
	Action     uint32
	ProtocolID uint64
	TxID       uint32
}

func (msg ConnectReq) Bytes() ([]byte, error) {
	var op errors.Op = "(ConnectReq).Bytes"
	var buf bytes.Buffer

	err := binary.Write(&buf, binary.BigEndian, msg.ProtocolID)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}

	err = binary.Write(&buf, binary.BigEndian, msg.Action)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}

	err = binary.Write(&buf, binary.BigEndian, msg.TxID)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}

	return buf.Bytes(), nil
}

func newConnReq() ConnectReq {
	return ConnectReq{
		Action:     CONNECT,
		ProtocolID: 0x41727101980, // Magic constant
		TxID:       rand.Uint32(),
	}
}

type ConnectResp struct {
	Action uint32
	TxID   uint32
	ConnID uint64
}

func (msg ConnectResp) Bytes() ([]byte, error) {
	var op errors.Op = "(ConnectReq).Bytes"
	var buf bytes.Buffer

	err := binary.Write(&buf, binary.BigEndian, msg.Action)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}
	err = binary.Write(&buf, binary.BigEndian, msg.TxID)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}
	err = binary.Write(&buf, binary.BigEndian, msg.ConnID)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}

	return buf.Bytes(), nil
}

func UnmarshalConnectMsg(r io.Reader, m *ConnectReq) error {
	var op errors.Op = "tracker.UnmarshalConnectMsg"
	var data = make([]byte, 32)

	_, err := io.ReadAtLeast(r, data, 16)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
			return errors.Wrap(nerr, op, errors.IO)
		} else if !ok {
			return errors.Wrap(err, op, errors.IO)
		}
	}

	action := binary.BigEndian.Uint32(data[:4])
	m.Action = action

	if action == ERROR {
		err := fmt.Errorf("%s", data[4:])
		return errors.Wrap(err, op)
	}

	txID := binary.BigEndian.Uint32(data[4:8])
	m.TxID = txID

	connID := binary.BigEndian.Uint64(data[8:])
	m.ProtocolID = connID

	return nil
}

type UDPAnnounceReq struct {
	ConnID uint64
	TxID   uint32

	Hash   [20]byte
	PeerID [20]byte

	Downloaded uint64
	Left       uint64
	Uploaded   uint64
	Event      uint32 // 0: None
	IP         uint32 // Default: 0
	Key        uint32
	Want       int32 // Default: -1
	Port       uint16
}

func (req UDPAnnounceReq) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	binary.Write(&buf, binary.BigEndian, req.ConnID)
	binary.Write(&buf, binary.BigEndian, ANNOUNCE)
	binary.Write(&buf, binary.BigEndian, req.TxID)

	binary.Write(&buf, binary.BigEndian, req.Hash[:])
	binary.Write(&buf, binary.BigEndian, req.PeerID) // peer_id

	binary.Write(&buf, binary.BigEndian, req.Downloaded)
	binary.Write(&buf, binary.BigEndian, req.Left)
	binary.Write(&buf, binary.BigEndian, req.Uploaded)
	binary.Write(&buf, binary.BigEndian, req.Event)
	binary.Write(&buf, binary.BigEndian, req.IP)
	binary.Write(&buf, binary.BigEndian, req.Key)
	binary.Write(&buf, binary.BigEndian, req.Want)
	binary.Write(&buf, binary.BigEndian, req.Port)

	return buf.Bytes(), nil
}

