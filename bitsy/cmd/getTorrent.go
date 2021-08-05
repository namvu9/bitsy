/*
Copyright © 2021 NAME HERE <vuna@protonmail.com>

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
	"math/rand"
	"net"
	"os"
	"sync"
	"time"

	"github.com/namvu9/bitsy/internal/session"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
	"github.com/spf13/cobra"
)

func connect(ctx context.Context, peers []tracker.PeerInfo, infoHash, peerID [20]byte) chan *peer.Peer {
	out := make(chan *peer.Peer, len(peers))

	go func() {
		var wg sync.WaitGroup
		for _, p := range peers {
			wg.Add(1)
			go func(p tracker.PeerInfo) {
				defer func() {
					wg.Done()
				}()
				addr := fmt.Sprintf("%s:%d", p.IP, p.Port)
				conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
				if err != nil {
					return
				}

				conn.SetDeadline(time.Now().Add(time.Second))
				err = session.Handshake(conn, infoHash, peerID)
				if err != nil {
					conn.Close()
					return
				}

				pPeer := peer.New(conn)
				var msg peer.HandshakeMessage
				err = peer.UnmarshalHandshake(conn, &msg)
				if err != nil {
					return
				}
				pPeer.Extensions = btorrent.NewExtensionsField(msg.Reserved)
				pPeer.ID = msg.PeerID[:]

				out <- pPeer
			}(p)
		}
		wg.Wait()
		close(out)
	}()

	return out
}

var peerID = [20]byte{'-', 'B', 'T', '0', '0', '0', '0', '-'}

// getTorrentCmd represents the getTorrent command
var getTorrentCmd = &cobra.Command{
	Use:   "getTorrent <magnet url>",
	Short: "Convert a magnet url to a torrent file",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		rand.Read(peerID[8:])
		t, err := btorrent.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
		}

		var peerID [20]byte

		tg := tracker.NewGroup(t.AnnounceList()[0])
		tc := tg.AnnounceC(tracker.NewRequest(t.InfoHash(), 1337, peerID))

		var out []*peer.Peer
		for stat := range tc {
			for _, chunk := range stat.Peers.Chunk(30) {
				for p := range connect(context.Background(), chunk, t.InfoHash(), peerID) {
					out = append(out, p)
					if len(out) >= 10 {
						//fmt.Printf("Took %s to find %d peers\n", time.Since(start), len(out))
						return
					}
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(getTorrentCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// getTorrentCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// getTorrentCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
