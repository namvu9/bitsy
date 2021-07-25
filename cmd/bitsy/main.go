package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/namvu9/bitsy"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/rs/zerolog"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Usage: bitsy <path to torrent> <outputDir>")
		os.Exit(127)
	}

	zerolog.SetGlobalLevel(zerolog.Disabled)
	pathToTorrent := os.Args[1]

	torrent, err := btorrent.Load(pathToTorrent)
	if err != nil {
	fmt.Println(err)
	os.Exit(1)
	}

	downloadDir := os.Args[2]
	baseDir := path.Join(path.Dir(downloadDir), ".tmp")
	err = os.MkdirAll(baseDir, 0777)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	s := bitsy.New(bitsy.Config{
		BaseDir:        baseDir,
		DownloadDir:    downloadDir,
		MaxConnections: 50,
		IP:             "192.168.0.4",
	})

	fmt.Printf("Downloading %s\n", pathToTorrent)
	done, err := s.Download(torrent)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	start := time.Now()
	for {
		select {
		case <-done:
			fmt.Printf("Download complete (%s)\n", time.Now().Sub(start))
			return
		default:
			fmt.Printf("Downloading %s\n", torrent.Name())
			stat := s.Stat()
			swarm := stat["swarms"].([]map[string]interface{})[0]
			have := swarm["have"]
			nPieces := swarm["npieces"]

			fmt.Printf("%v / %v pieces\n", have, nPieces)

			time.Sleep(5 * time.Second)
			cmd := exec.Command("clear")
			cmd.Stdout = os.Stdout
			cmd.Run()

		}
	}
}
