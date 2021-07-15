package tracker

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/url"
	"time"
)

// UDPTracker implements the UDP Tracker 'Connect' and
// 'Announce' protocols as defined in BEP-15
// (https://www.bittorrent.org/beps/bep_0015.html)
type UDPTracker struct {
	*url.URL
	conn *udpConn

	// UDP is an 'unreliable' protocol. This means it doesn't
	// retransmit lost packets itself. The application is
	// responsible for this. If a response is not received
	// after 15 * 2 ^ n seconds, the client should retransmit
	// the request, where n starts at 0 and is increased up to
	// 8 (3840 seconds) after every retransmission. Note that
	// it is necessary to rerequest a connection ID when it
	// has expired.
	nConnAttempts   int
	nextConnAttempt time.Time
	nextAnnounce    time.Time
}

// ConnMessage represents the structure that constitutes the
// connect portion of the UDP tracker prtocol
type ConnMessage struct {
	Action uint32
	ConnID uint64
	TxID   uint32
}

func newConnReq() ConnMessage {
	return ConnMessage{
		Action: CONNECT,
		ConnID: 0x41727101980, // Magic constant
		TxID:   rand.Uint32(),
	}
}

func marshalConnMessage(w io.Writer, m ConnMessage) (int, error) {
	var buf bytes.Buffer

	err := binary.Write(&buf, binary.BigEndian, m.ConnID)
	if err != nil {
		return 0, err
	}
	err = binary.Write(&buf, binary.BigEndian, m.Action)
	if err != nil {
		return 0, err
	}
	err = binary.Write(&buf, binary.BigEndian, m.TxID)
	if err != nil {
		return 0, err
	}

	return w.Write(buf.Bytes())
}

func unmarshalConnMessage(r io.Reader, m *ConnMessage) error {
	data, err := io.ReadAll(r)
	if err != nil {
		if nerr, ok := err.(net.Error); ok && !nerr.Timeout() {
			return nerr
		} else if !ok {
			return err
		}
	}

	if len(data) < 16 {
		return fmt.Errorf("Malformed response: %d bytes", len(data))
	}

	action := binary.BigEndian.Uint32(data[:4])
	m.Action = action

	if action == ERROR {
		return fmt.Errorf("%s", data[4:])
	}

	txID := binary.BigEndian.Uint32(data[4:8])
	m.TxID = txID

	connID := binary.BigEndian.Uint64(data[8:])
	m.ConnID = connID

	return nil
}

func (tr *UDPTracker) connect() error {
	conn, err := net.Dial("udp", tr.Host())
	if err != nil {
		if nerr, ok := err.(net.Error); ok && !nerr.Temporary() {
			return nerr
		}

		tr.scheduleRetry()
		return err
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	req := newConnReq()
	_, err = marshalConnMessage(conn, req)
	if err != nil {
		return err
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	// Handle response
	var res ConnMessage
	if err := unmarshalConnMessage(conn, &res); err != nil {
		tr.scheduleRetry()
		return err
	}

	if req.TxID != res.TxID {
		return fmt.Errorf("Transaction IDs do not match: want %d got %d", req.TxID, res.TxID)
	}

	if res.Action != req.Action {
		return fmt.Errorf("Actions do not match: want %d got %d", req.Action, res.Action)
	}

	tr.conn = &udpConn{
		ID:       res.ConnID,
		Deadline: time.Now().Add(time.Minute),
	}

	tr.nConnAttempts = 0

	return nil
}

type udpConn struct {
	ID       uint64
	Deadline time.Time
}

func (conn *udpConn) Expired() bool {
	if conn == nil {
		return true
	}

	return conn.Deadline.Before(time.Now())
}

func (tr *UDPTracker) Announce(req AnnounceRequest) (*AnnounceResp, error) {
	if tr.conn.Expired() {
		err := tr.connect()
		if err != nil {
			return nil, err
		}
	}

	conn, err := net.Dial("udp", tr.Host())
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	_, err = conn.Write(packUDPAnnounceRequest(UDPAnnounceRequest{
		connID:          tr.conn.ID,
		txID:            rand.Uint32(),
		AnnounceRequest: req,
	}))
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var buf bytes.Buffer
	_, err = io.Copy(&buf, conn)
	if err != nil {
		if err, ok := err.(net.Error); !ok || ok && !err.Timeout() {
			return nil, err
		}

	}

	res, err := parseAnnounceResp(&buf, req.Hash)
	if err != nil {
		return nil, err
	}

	tr.nextAnnounce = time.Now().Add(time.Duration(res.Interval) * time.Second)

	return res, nil

}

// TODO: test
func (tr *UDPTracker) ShouldAnnounce() bool {
	if tr.nConnAttempts == -1 || time.Now().Before(tr.nextConnAttempt) {
		return false
	}

	return time.Now().After(tr.nextAnnounce)
}

func (tr *UDPTracker) Host() string {
	return tr.URL.Host
}

func (tr *UDPTracker) scheduleRetry() {
	if tr.nConnAttempts >= 8 {
		return
	}

	tr.nConnAttempts++
	t := int64(15 * math.Pow(2, float64(tr.nConnAttempts)))
	tr.nextConnAttempt = time.Now().Add(time.Duration(t) * time.Second)
}
