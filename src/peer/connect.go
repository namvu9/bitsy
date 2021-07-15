package peer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"
)

// Conn represents a connection to another peer in the
// swarm
type Conn struct {
	net.Conn

	InfoHash [20]byte
	IP       string // The Peer's IP Address
	Protocol string // must be 'TCP' or 'uTP'

	// State

	// A 'choked' peer is one that the client refuses
	// to send file pieces to. This may occur for several
	// reasons: - The peer is a 'seed', in which case it
	// 'uninterested' - The peer has been blacklisted - The
	// client has reached its upload limit (max_uploads)
	//
	// Peers are initially choked
	Choked bool

	// A downloader is 'interested' if it wants one or more
	// pieces that the client has
	//
	// Peers are initially uninterested
	Interested bool

	// Number of bytes uploaded in the last n-second window
	UploadRate   int64
	DownloadRate int64
}

func (c *Conn) Choke() error {
	c.Choked = true
	return nil
}

// Handshake attempts to perform a handshake with the given
// client at addr with infoHash.
func Handshake(conn net.Conn, addr string, infoHash [20]byte, peerID [20]byte) (*Conn, error) {
	msg := HandShakeMessage{
		infoHash: infoHash,
		peerID:   peerID,
	}

	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := conn.Write(msg.Payload())
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	_, err = ioutil.ReadAll(conn)
	if err != nil {
		return nil, err
	}

	return &Conn{
		Conn:     conn,
		InfoHash: infoHash,
		IP:       addr,
		Protocol: "tcp", // The only supported protocol
		Choked:   true,
	}, nil
}

func RespondHandshake(ctx context.Context, conn net.Conn, hasHashFn func([20]byte) bool, peerID [20]byte) (*Conn, error) {
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var buf bytes.Buffer

	n, err := io.CopyN(&buf, conn, 68)
	if err != nil {
		return nil, err
	}

	if n != 68 {
		conn.Close()
		return nil, fmt.Errorf("expeted handshake message of length %d but got %d", 68, n)
	}

	data := buf.Bytes()

	//pstrlen := data[0]
	pstr := data[1:20]
	if !bytes.Equal(pstr, []byte("BitTorrent protocol")) {
		fmt.Printf("Handshake failed. Wanted pstr = BitTorrent protocol; got %s\n", pstr)
	}

	//reserved := data[20:28]

	var infoHash [20]byte
	copy(infoHash[:], data[28:48])

	// TODO: PeerIDs are not returned by UDP trackers, which
	// is currently not supported
	//peerID := data[48:]

	if !hasHashFn(infoHash) {
		conn.Close()
		return nil, fmt.Errorf("Unknown info hash: %v\n", infoHash)
	}

	// RespondHandshake
	peerConn, err := Handshake(conn, conn.RemoteAddr().String(), infoHash, peerID)
	if err != nil {
		return nil, fmt.Errorf("faile to respond to handshake (%s): %w", conn.RemoteAddr(), err)
	}

	return peerConn, nil
}
