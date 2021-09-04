package session

import (
	"fmt"
	"sort"
	"strings"

	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/pkg/btorrent"
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

func min(a, b btorrent.Size) btorrent.Size {
	if a < b {
		return a
	}
	return b
}

func (s *Session) stat() {
	for hash, torrent := range s.torrents {
		var sb strings.Builder
		fmt.Fprintln(&sb, torrent.Name())
		fmt.Fprintln(&sb, torrent.HexHash())

		clientStat := s.data.Stat(hash)
		if clientStat.State == data.ERROR {
			fmt.Fprintf(&sb, "State: %s (%s)\n", clientStat.State, clientStat.Error)
		} else {
			fmt.Fprintf(&sb, "State: %s\n", clientStat.State)
		}
		fmt.Fprintf(&sb, "Connections: %d\n", s.conn.Stat())
		fmt.Fprintf(&sb, "Uploaded: %s\n", clientStat.Uploaded)
		fmt.Fprintf(&sb, "Downloaded: %s / %s\n", min(clientStat.Downloaded, torrent.Length()), torrent.Length())
		fmt.Fprintf(&sb, "Download rate: %s / s\n", clientStat.DownloadRate)
		fmt.Fprintf(&sb, "Pending pieces: %d\n\n", clientStat.Pending)

		for _, file := range clientStat.Files {
			if file.Ignored {
				continue
			}

			fmt.Fprintf(&sb, "%s (%s/%s)\n", file.Name, file.Downloaded, file.Size)
		}

		fmt.Fprintln(&sb, s.peers.Swarms()[hash])
		clear()
		fmt.Println(sb.String())
	}
}
