/*
Copyright © 2021 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/internal/peers"
	"github.com/namvu9/bitsy/internal/session"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/size"
	"github.com/spf13/cobra"
)

var files []int

// TODO: Use config
func BaseDir() (string, error) {
	d, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	baseDir := path.Join(d, ".bitsy")
	err = os.MkdirAll(baseDir, 0777)
	if err != nil {
		return "", err
	}

	return baseDir, nil
}

func DownloadDir() (string, error) {
	d, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	downloadDir := path.Join(d, "Downloads")
	err = os.MkdirAll(downloadDir, 0777)
	if err != nil {
		return "", err
	}

	return downloadDir, nil
}

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download <torrent>",
	Short: "Start/Resume a torrent download",
	Long: `This command starts downloading all of the files described by a torrent, unless a modifier flag is provided. If a download has previously been initiated for a torrent with an identical info hash, the download is resumed.

To download specific files of a torrent, see the --files/-f modifier.

The downloaded file(s) are written to the 'outDir' directory specified by the bitsy config file (by default located at ~/.bitsy.json).

Examples:

bitsy download <Magnet URL>
bitsy download --files 0,1,2 <Magnet URL>
bitsy download -f 0 /path/to/torrent
bitsy download /path/to/torrent
`,
	Run: func(cmd *cobra.Command, args []string) {
		baseDir, err := BaseDir()
		if err != nil {
			fmt.Println(err)
			return
		}

		downloadDir, err := DownloadDir()
		if err != nil {
			fmt.Println(err)
			return
		}

		s := session.New(session.Config{
			BaseDir:        baseDir,
			DownloadDir:    downloadDir,
			MaxConnections: 50,
			IP:             "192.168.0.4",
			Ports:          []uint16{6881, 6889, 6882, 7881, 9999},
		})

		opts := []data.Option{}
		if len(files) > 0 {
			opts = append(opts, data.WithFiles(files...))
		}

		fmt.Printf("Initiating session... ")
		s.Init()
		fmt.Printf("done\n")

		if len(args) > 0 {
			t, err := btorrent.Load(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
				os.Exit(1)
			}
			fmt.Printf("Fetching metadata... ")
			if _, ok := t.Info(); !ok {
				ctx, cancel := withTimeout(context.Background(), 10*time.Second)
				t2, err := getMeta(ctx, t, 6881)
				if err != nil {
					cancel()
					return
				}
				t = t2
				cancel()
			}
			fmt.Printf("done\n")
			s.Register(*t, opts...)
		}

		for {
			s := printStat(s.Stat())
			clear()
			fmt.Println(s)
			time.Sleep(time.Second)
		}
	},
}

func clear() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func printStat(stat map[string]interface{}) string {
	var sb strings.Builder

	clients := stat["clients"].(map[string]data.ClientStat)
	swarms := stat["swarms"].(map[string]peers.SwarmStat)

	fmt.Fprintf(&sb, "-----\nBitsy\n-----\n")
	fmt.Fprintf(&sb, "Connections: %d\n", stat["connections"])

	for hash, c := range clients {
		swarm := swarms[hash]
		fmt.Fprintf(&sb, "<<<%s>>>\n", c.Name)
		if c.TimeLeft > 0 {
			fmt.Fprintf(&sb, "Time left: %s\n", c.TimeLeft)
		} else {
			fmt.Fprintf(&sb, "Time left: N/A\n")
		}
		fmt.Fprintf(&sb, "Piece Length: %s\n", c.PieceLength)
		fmt.Fprintf(&sb, "Uploaded: %s\n", c.Uploaded)
		fmt.Fprintf(&sb, "Downloaded: %s\n", c.Downloaded)
		fmt.Fprintf(&sb, "Download Rate: %s / s\n", c.DownloadRate)
		fmt.Fprintf(&sb, "Pending pieces: %d\n", c.Pending)

		fmt.Fprintf(&sb, "Peers: %d (%d Interesting, %d Choked by)\n\n", len(swarm.Peers), swarm.Interesting, swarm.Blocking)
		fmt.Fprintf(&sb, "Top uploaders:\n")

		sort.SliceStable(swarm.Peers, func(i, j int) bool {
			return swarm.Peers[i].UploadRate > swarm.Peers[j].UploadRate
		})

		count := 0
		for idx, p := range swarm.Peers {
			if count == 5 {
				break
			}
			fmt.Fprintf(&sb, "%d: %s %s / s (Total: %s)\n", idx, p.IP, size.Size(p.UploadRate), size.Size(p.Uploaded))
			count++
		}

		fmt.Fprintln(&sb, "")

		for _, file := range c.Files {
			if file.Ignored {
				continue
			}

			fmt.Fprintf(&sb, "%d: %s\n", file.Index, file.Name)
			fmt.Fprintf(&sb, "Downloaded: %s (%s)\n", file.Downloaded, file.Size)

		}

		fmt.Fprintf(&sb, "\n\n")
	}

	return sb.String()
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().
		IntSliceVarP(&files, "files", "f", []int{}, "Download specific files")
}
