package peers

import (
	"fmt"
	"strings"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type SwarmStat struct {
	Peers          int
	Choked         int
	Blocking       int
	Interested     int
	Interesting    int
	TopPeers       []*peer.Peer
	TopDownloaders []*peer.Peer
}

func (s SwarmStat) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Peers: %d\n", s.Peers)
	fmt.Fprintf(&sb, "Choking: %d\n", s.Choked)
	fmt.Fprintf(&sb, "Choked by: %d\n", s.Blocking)
	fmt.Fprintf(&sb, "Interested: %d\n", s.Interested)
	fmt.Fprintf(&sb, "Interesting: %d\n", s.Interesting)

	fmt.Fprintln(&sb, "Top 5 uploaders:")
	for idx, p := range s.TopPeers {
		uniquePieces := make(map[int]int)

		for _, req := range p.Requests {
			uniquePieces[int(req.Index)]++
		}
		fmt.Fprintf(&sb, "%d: %s (%d pieces, %d requests, idle: %v) %s / s (Total: %s)\n", idx, p.RemoteAddr(), len(uniquePieces), len(p.Requests), p.Idle(), btorrent.Size(p.UploadRate), btorrent.Size(p.Uploaded))
	}

	fmt.Fprintln(&sb, "Top 5 Downloaders:")
	for idx, p := range s.TopDownloaders {
		uniquePieces := make(map[int]int)

		for _, req := range p.Requests {
			uniquePieces[int(req.Index)]++
		}
		fmt.Fprintf(&sb, "%d: %s %s\n", idx, p.RemoteAddr(), btorrent.Size(p.Downloaded))
	}

	return sb.String()
}
