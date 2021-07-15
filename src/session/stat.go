package session

import (
	"github.com/namvu9/bitsy/src/torrent"
	"github.com/namvu9/bitsy/src/tracker"
)

type TorrentState struct {
	*torrent.Torrent
	Location   string
	Status     TorrentStatus
	Left       uint64
	Downloaded uint64
	Uploaded   uint64

	// Hash list of pieces that have been downloaded
	Have     []string
	Peers    []tracker.PeerInfo
	Trackers []tracker.Tracker
}

type TorrentStatus int

const (
	NOT_STARTED TorrentStatus = iota
	STARTED
	PAUSED
	RESUMING
	ERROR
)
