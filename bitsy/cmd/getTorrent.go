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
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/namvu9/bencode"
	"github.com/namvu9/bitsy/internal/session"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/tracker"
	"github.com/spf13/cobra"
)

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	go func() {
		<-ctx.Done()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			os.Stderr.WriteString(fmt.Sprintf("%s\n", ctx.Err()))
			os.Exit(1)
		}
	}()

	return ctx, cancel
}

// getTorrentCmd represents the getTorrent command
var getTorrentCmd = &cobra.Command{
	Use:   "getTorrent <magnet url>",
	Short: "Convert a magnet url to a torrent file",
	Long: `Example:

bitsy getTorrent <magnetURL> > out.torrent
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := withTimeout(context.Background(), 60*time.Second)
		defer cancel()

		t, err := btorrent.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
			os.Exit(1)
		}

		if _, ok := t.Info(); ok {
			os.Stdout.Write(t.Bytes())
			return
		}

		log("Fetching metadata... ")
		var (
			start = time.Now()
		)

		t, err = getMeta(ctx, t, 6881)
		if err != nil {
			panic(err)
		}

		out := cmd.Flag("out")
		if out != nil && out.Value.String() != "" {
			if err := os.WriteFile(out.Value.String(), t.Bytes(), 0777); err != nil {
				log("%s\n", err)
				return
			}
		} else if _, err := os.Stdout.Write(t.Bytes()); err != nil {
			log("%s\n", err)
			return
		}
		log("Done (took %.2fs)\n", time.Since(start).Seconds())
	},
}

func foo(ctx context.Context, t btorrent.Torrent, peers []net.Addr) (*btorrent.Torrent, error) {
	info, err := requestMeta(ctx, peers, &t)
	if err != nil {
		return nil, err
	}

	t.Dict().SetStringKey("info", info)
	if !t.VerifyInfoDict() {
		return nil, fmt.Errorf("ASDF")
	}

	return &t, nil
}

func getMeta(ctx context.Context, t *btorrent.Torrent, port uint16) (*btorrent.Torrent, error) {
	res := make(chan btorrent.Torrent)
	go func() {
	start:
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		for stat := range announce(ctx, t, port, 0) {
			if len(stat.Peers) == 0 {
				continue
			}

			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()

			t, err := foo(ctx, *t, stat.Peers)
			if err == nil {
				res <- *t
				return
			}
		}

		goto start
	}()

	select {
	case t := <-res:
		return &t, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func requestMeta(ctx context.Context, peers []net.Addr, t *btorrent.Torrent) (*bencode.Dictionary, error) {
	var (
		dialCfg = peer.DialConfig{
			InfoHash:   t.InfoHash(),
			Timeout:    500 * time.Millisecond,
			PeerID:     session.PeerID,
			Extensions: session.Reserved,
			PStr:       session.PStr,
		}
	)

	info, err := peer.GetInfoDict(ctx, peers, dialCfg)
	if err != nil {
		return nil, err
	}

	return info, nil
}

func log(fmtString string, args ...interface{}) {
	os.Stderr.WriteString(fmt.Sprintf(fmtString, args...))
}

func announce(ctx context.Context, t *btorrent.Torrent, port uint16, downloaded uint64) chan tracker.TrackerStat {
	var (
		tiers = t.AnnounceList()
		out   = make(chan tracker.TrackerStat, 10)
		req   = tracker.NewRequest(t.InfoHash(), port, session.PeerID, downloaded)
	)

	go func() {
		var wg sync.WaitGroup
		for _, urls := range tiers {
			wg.Add(1)
			go func(urls []string, wg *sync.WaitGroup) {
				defer wg.Done()

				select {
				case <-ctx.Done():
					return
				default:
					res := tracker.AnnounceS(ctx, urls, req)
					for {
						select {
						case r, ok := <-res:
							if !ok {
								return
							}
							out <- r
						case <-ctx.Done():
							return
						}
					}
				}
			}(urls, &wg)
		}
		wg.Wait()
		close(out)
	}()

	return out
}

func init() {
	getTorrentCmd.Flags().StringP("out", "o", "", "file to write torrent file to")
	rootCmd.AddCommand(getTorrentCmd)
}
