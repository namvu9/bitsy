package swarm

import (
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"
)

const MIN_PEERS = 10

func shuffle(peers []net.Addr) {
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})
}

type SwarmStat struct {
	Peers       int
	Choked      int
	Blocking    int
	Interested  int
	Interesting int
}

func (s SwarmStat) String() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Peers: %d\n", s.Peers)
	fmt.Fprintf(&sb, "Choked: %d\n", s.Choked)
	fmt.Fprintf(&sb, "Choking: %d\n", s.Blocking)
	fmt.Fprintf(&sb, "Interested: %d\n", s.Interested)
	fmt.Fprintf(&sb, "Interesting: %d\n", s.Interesting)

	return sb.String()
}

//func (s *Swarm) Stat() SwarmStat {
	//cpy := make([]*peer.Peer, len(s.peers))
	//copy(cpy, s.peers)

	//var (
		//choked      int
		//blocking    int
		//interested  int
		//interesting int
	//)

	//for _, p := range cpy {
		//if p.Blocking {
			//blocking++
		//}

		//if p.Choked {
			//choked++
		//}

		//if p.Interested {
			//interested++
		//}

		//if p.Interesting {
			//interesting++
		//}

	//}

	//return SwarmStat{
		//Peers:       len(cpy),
		//Choked:      choked,
		//Blocking:    blocking,
		//Interested:  interested,
		//Interesting: interesting,
	//}
//}
