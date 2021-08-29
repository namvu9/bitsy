package peers

import (
	"fmt"
	"strings"
)

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
