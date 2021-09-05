package session

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

func fmtDuration(d time.Duration) string {
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
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

		var (
			totalSize    = torrent.Length()
			downloaded   = clientStat.Downloaded
			left         = totalSize - downloaded
			downloadRate = clientStat.DownloadRate
			timeLeft     = time.Duration(left/(downloadRate+1)) * time.Second
		)

		if downloadRate == 0 {
			fmt.Fprintln(&sb, "Time left: N/A")
		} else {
			fmt.Fprintf(&sb, "Time left: %s\n", fmtDuration(timeLeft))
		}

		fmt.Fprintf(&sb, "Connections: %d\n", s.conn.Stat())
		fmt.Fprintf(&sb, "Uploaded: %s\n", clientStat.Uploaded)
		fmt.Fprintf(&sb, "Downloaded: %s / %s\n", min(downloaded, totalSize), totalSize)
		fmt.Fprintf(&sb, "Download rate: %s / s\n", clientStat.DownloadRate)
		fmt.Fprintf(&sb, "Pending pieces: %d\n\n", clientStat.Pending)

		for idx, file := range clientStat.Files {
			if file.Ignored {
				continue
			}

			fmt.Fprintf(&sb, "%d: %s (%s/%s)\n", idx, file.Name, file.Downloaded, file.Size)
		}
		fmt.Fprintln(&sb, "")
		fmt.Fprintln(&sb, s.peers.Swarms()[hash])
		clear()
		fmt.Println(sb.String())
	}
}
