package tracker

import (
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/namvu9/bitsy/src/errors"
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/rs/zerolog/log"
)

// UDP Tracker Actions
const (
	CONNECT  uint32 = 0
	ANNOUNCE uint32 = 1
	SCRAPE   uint32 = 2
	ERROR    uint32 = 3
)

type Dialer interface {
	Dial(string, string) (net.Conn, error)
	CanDial() bool
}

type DialFunc func(string, string) (net.Conn, error)

func (fn DialFunc) Dial(network, addr string) (net.Conn, error) {
	return fn(network, addr)
}

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
	torrent.Torrent

	Downloaded uint64
	Keft       uint64

	outCh chan UDPAnnounceResponse
	inCh  chan map[string]interface{}

	stats  map[string]*TrackerStat
	peerID [20]byte

	d    Dialer
	Port uint16
}

func (t *TrackerGroup) Listen() (chan UDPAnnounceResponse, chan map[string]interface{}) {
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

					//if !t.d.CanDial() {
					//continue
					//}

					n++
					go func(tracker string) {
						connID, err := UDPConnect(url, t.d)
						if err != nil {
							err = errors.Wrap(err, op)
							log.Err(err).Strs("trace", errors.Ops(err)).Msg("failed to connect to UDP tracker")
							return
						}

						req := NewUDPAnnounceReq(*t.InfoHash(), connID, t.Port, t.peerID)
						select {
						case data := <-t.inCh:
							req.Downloaded = data["downloaded"].(uint64)
							t.Downloaded = req.Downloaded
						default:
							req.Downloaded = t.Downloaded
						}

						resp, err := UDPAnnounce(url, req, t.d)
						if err != nil {
							err = errors.Wrap(err, op)
							log.Err(err).Strs("trace", errors.Ops(err)).Msg("faileed to announce over UDP tracker")
							return
						}

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

func New(t torrent.Torrent, d Dialer, port uint16, peerID [20]byte) *TrackerGroup {
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
		d:       d,
		peerID:  peerID,
	}
}
