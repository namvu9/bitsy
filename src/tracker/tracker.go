package tracker

import (
	"fmt"
	"net"
	"net/url"
)

type Tracker interface {
	Host() string
	Announce(AnnounceRequest) (*AnnounceResp, error)
	ShouldAnnounce() bool
	//Scrape()
}

type HTTPTracker struct{}

type PeerInfo struct {
	PeerID   *[20]byte
	InfoHash [20]byte
	// The IP address of a peer in the swarm
	IP net.IP

	// The peer's listen Port
	Port uint16
}

func (p PeerInfo) String() string {
	return fmt.Sprintf("%s:%d", p.IP, p.Port)
}

const (
	CONNECT  uint32 = 0
	ANNOUNCE uint32 = 1
	SCRAPE   uint32 = 2
	ERROR    uint32 = 3
)

func New(url *url.URL) (Tracker, error) {
	if url.Scheme == "udp" {
		return &UDPTracker{
			URL: url,
		}, nil
	}

	return nil, fmt.Errorf("not implemented")
}
