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
	"path"
	"time"

	"github.com/namvu9/bitsy/internal/session"
	"github.com/namvu9/bitsy/pkg/btorrent"
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
	err = os.MkdirAll(baseDir, 0666)
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
	err = os.MkdirAll(downloadDir, 0666)
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
		t, err := btorrent.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("--------\n%s\n-------\n", t.Name())
		fmt.Printf("%s\n", t.HexHash())
		fmt.Printf("Forwarding port... ")

		// TODO: use port from config
		port, err := session.ForwardPorts(6881, 6889)
		if err != nil {
			fmt.Println("Failed to find open port", err)
			return
		}
		fmt.Printf("%d\n", port)

		fmt.Printf("Fetching metadata... ")
		if _, ok := t.Info(); !ok {
			ctx, cancel := withTimeout(context.Background(), 10*time.Second)
			t2, err := getMeta(ctx, t, port)
			if err != nil {
				cancel()
				return
			}
			t = t2
			cancel()
		}
		fmt.Printf("done\n")
		fmt.Printf("Initiating session\n")

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
			Port:           port,
			Files:          files,
		}, *t)

		s.Init()

		select {}

	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().
		IntSliceVarP(&files, "files", "f", []int{}, "Download specific files")
}
