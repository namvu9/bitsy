package tracker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

func packUDPAnnounceRequest(req UDPAnnounceRequest) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, req.connID)
	binary.Write(&buf, binary.BigEndian, ANNOUNCE)
	binary.Write(&buf, binary.BigEndian, req.txID)

	buf.Write(packAnnounceRequest(req.AnnounceRequest))

	return buf.Bytes()
}

func packAnnounceRequest(req AnnounceRequest) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.BigEndian, req.Hash[:])
	binary.Write(&buf, binary.BigEndian, req.Hash[:]) // peer_id

	binary.Write(&buf, binary.BigEndian, req.Downloaded)
	binary.Write(&buf, binary.BigEndian, req.Left)
	binary.Write(&buf, binary.BigEndian, req.Uploaded)
	binary.Write(&buf, binary.BigEndian, req.Event)
	binary.Write(&buf, binary.BigEndian, req.IP)
	binary.Write(&buf, binary.BigEndian, req.Key)
	binary.Write(&buf, binary.BigEndian, req.Want)
	binary.Write(&buf, binary.BigEndian, req.Port)

	return buf.Bytes()
}

type AnnounceRequest struct {
	Hash       [20]byte
	PeerID     [20]byte
	Downloaded uint64
	Left       uint64
	Uploaded   uint64
	Event      uint32
	IP         uint32
	Key        uint32
	Want       int32
	Port       uint16
}

type UDPAnnounceRequest struct {
	connID uint64
	txID   uint32
	AnnounceRequest
}

type AnnounceResp struct {
	Action    uint32
	TxID      uint32
	Interval  uint32
	NLeechers uint32
	NSeeders  uint32
	Peers     []PeerInfo
	InfoHash  [20]byte
}

func (ar AnnounceResp) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Seeders: %d\n", ar.NSeeders))
	sb.WriteString(fmt.Sprintf("Leechers: %d\n", ar.NLeechers))
	sb.WriteString(fmt.Sprintf("Peers: %d\n", len(ar.Peers)))
	sb.WriteString(fmt.Sprintf("Interval: %d seconds\n", ar.Interval))

	return sb.String()
}

func parseAnnounceResp(r io.Reader, infoHash [20]byte) (*AnnounceResp, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	n := len(data)

	action := binary.BigEndian.Uint32(data[:4])
	txid := binary.BigEndian.Uint32(data[4:8])

	if action == uint32(ERROR) {
		message := string(data[8:])
		return nil, errors.New(message)
	}

	interval := binary.BigEndian.Uint32(data[8:12])
	nleechers := binary.BigEndian.Uint32(data[12:16])
	nseeders := binary.BigEndian.Uint32(data[16:20])

	var peers []PeerInfo

	if n > 20 {
		offset := 20
		for len(data[offset:]) >= 48 {
			var ip = make(net.IP, 4)
			_ip := binary.BigEndian.Uint32(data[offset : offset+32])
			binary.BigEndian.PutUint32(ip, _ip)

			port := data[offset+32 : offset+48]
			_port := binary.BigEndian.Uint16(port)

			peers = append(peers, PeerInfo{IP: ip, Port: _port, InfoHash: infoHash})
			offset += 48
		}

	}

	return &AnnounceResp{
		InfoHash:  infoHash,
		Action:    action,
		TxID:      txid,
		Interval:  interval,
		NLeechers: nleechers,
		NSeeders:  nseeders,
		Peers:     peers,
	}, nil
}
