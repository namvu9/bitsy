package peers

import (
	"fmt"
	"strings"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type SwarmStat struct {
	Peers       int
	Choked      int
	Blocking    int
	Interested  int
	Interesting int
	TopPeers    []*peer.Peer
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
		fmt.Fprintf(&sb, "%d: %s\n", idx, btorrent.Size(p.Uploaded))
	}

	return sb.String()
}
