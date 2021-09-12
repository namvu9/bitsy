package session

import (
	"fmt"
	"sort"
	"time"

	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/internal/peers"
	"github.com/namvu9/bitsy/pkg/btorrent/size"
)

type freq struct {
	index int
	freq  int
}

func sortByValue(x map[int]int) []freq {
	var out []freq

	for idx, v := range x {
		out = append(out, freq{index: idx, freq: v})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].freq < out[j].freq
	})

	return out
}

func min(a, b size.Size) size.Size {
	if a < b {
		return a
	}
	return b
}

func fmtDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (s *Session) Stat() map[string]interface{} {
	cmd := StatCmd{res: make(chan map[string]interface{})}
	s.eventsIn <- cmd

	return <- cmd.res
}

func (s *Session) stat() map[string]interface{} {
	out := make(map[string]interface{})
	out["connections"] = s.conn.Stat()
	clients := make(map[string]data.ClientStat)
	swarms := make(map[string]peers.SwarmStat)
	out["clients"] = clients
	out["swarms"] = swarms

	for hash, torrent := range s.torrents {
		clients[torrent.HexHash()] = s.data.Stat(hash)
		swarms[torrent.HexHash()] = s.peers.Swarms()[hash]
	}

	return out
}
