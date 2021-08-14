package client

import (
	"fmt"
	"strings"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

type FileStat struct {
	Index   int
	Ignored bool
	//Status     ClientStat
	Name       string
	Size       btorrent.Size
	Downloaded btorrent.Size
}

func (fs FileStat) String() string {
	var sb strings.Builder

	percent := float64(fs.Downloaded) / float64(fs.Size) * 100

	fmt.Fprintf(&sb, "File: %s\n", fs.Name)
	fmt.Fprintf(&sb, "Ignored: %v\n", fs.Ignored)
	fmt.Fprintf(&sb, "Index: %d\n", fs.Index)
	fmt.Fprintf(&sb, "Size: %s\n", fs.Size)
	fmt.Fprintf(&sb, "Downloaded: %s (%.3f %%)\n", fs.Downloaded, percent)

	return sb.String()
}

type ClientStat struct {
	State        ClientState
	Pieces       int
	TotalPieces  int
	DownloadRate btorrent.Size // per second
	Downloaded   btorrent.Size
	Left         btorrent.Size
	Uploaded     btorrent.Size
	Files        []FileStat
	Pending      int

	BaseDir string
	OutDir  string
}

func (c ClientStat) String() string {
	var sb strings.Builder

	total := c.Downloaded + c.Left
	percentage := float64(c.Downloaded) / float64(total) * 100

	fmt.Fprintf(&sb, "State: %s\n", c.State)
	fmt.Fprintf(&sb, "Uploaded: %s\n", c.Uploaded)
	fmt.Fprintf(&sb, "Downloaded: %s (%.3f %%)\n", c.Downloaded, percentage)
	fmt.Fprintf(&sb, "Download rate: %s / s \n", c.DownloadRate)
	fmt.Fprintf(&sb, "Pieces pending: %d\n", c.Pending)

	fmt.Fprint(&sb, "\n")
	for _, file := range c.Files {
		fmt.Fprint(&sb, file)
		fmt.Fprint(&sb, "\n")
	}

	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) Stat() ClientStat {
	var fs []FileStat

	for i, file := range c.torrent.Files() {
		var ignored bool
		for _, pc := range file.Pieces {
			pcIdx := c.torrent.GetPieceIndex(pc)
			if pcIdx >= 0 && c.ignoredPieces.Get(pcIdx) {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}
		var downloaded int
		for _, piece := range file.Pieces {
			if c.pieces.Get(c.torrent.GetPieceIndex(piece)) {
				downloaded += int(c.torrent.PieceLength())
			}
		}

		fs = append(fs, FileStat{
			Index:      i,
			Ignored:    ignored,
			Name:       file.Name,
			Size:       file.Length,
			Downloaded: btorrent.Size(min(int(file.Length), downloaded)),
		})
	}

	return ClientStat{
		State:        c.state,
		Uploaded:     btorrent.Size(c.Uploaded),
		Downloaded:   btorrent.Size(c.pieces.GetSum() * int(c.torrent.PieceLength())),
		DownloadRate: c.DownloadRate,
		Left:         btorrent.Size((len(c.torrent.Pieces()) - c.pieces.GetSum()) * int(c.torrent.PieceLength())),

		Pieces:      c.pieces.GetSum(),
		Pending:     c.Pending,
		TotalPieces: len(c.torrent.Pieces()),
		Files:       fs,

		BaseDir: c.baseDir,
		OutDir:  c.outDir,
	}
}
