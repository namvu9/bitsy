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
	"time"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/spf13/cobra"
)

// statCmd represents the stat command
var statCmd = &cobra.Command{
	Use:   "stat",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := withTimeout(context.Background(), 30*time.Second)
		defer cancel()

		t, err := btorrent.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
			os.Exit(1)
		}

		var count int
		var seeders int
		var leechers int

		log("Fetching metadata... ")
		start := time.Now()
		var gotMeta bool

		if _, ok := t.Info(); ok {
			gotMeta = true
		}

		for stat := range announce(ctx, t, 1337) {
			seeders += stat.Seeders
			leechers += stat.Leechers
			count++

			if !gotMeta {
				if len(stat.Peers) == 0 {
					continue
				}
				info, err := requestMeta(ctx, stat.Peers, t)
				if err != nil {
					log(err.Error())
					return
				}

				t.Dict().SetStringKey("info", info)

				if !t.VerifyInfoDict() {
					return
				}

				gotMeta = true
			}
		}

		cancel()
		log("Done (took %.2fs)\n\n", time.Since(start).Seconds())

		fmt.Printf("-------\n%s\n-------\n", t.Name())
		fmt.Printf("Piece length: %s\n", t.PieceLength())
		fmt.Printf("Seeders: %d\n", seeders/count)
		fmt.Printf("Leechers: %d\n", leechers/count)
		fmt.Printf("Info Hash: %s\n", t.HexHash())
		fmt.Printf("Total size: %s\n", t.Length())
		fmt.Println("Files:")
		files, _ := t.Files()
		for _, file := range files {
			fmt.Printf("  %s %s\n", file.Path, file.Length)
		}
		fmt.Printf("Trackers:\n")
		for _, tracker := range t.AnnounceList()[0] {
			fmt.Printf("  %s\n", tracker)
		}
	},
}

func init() {
	rootCmd.AddCommand(statCmd)
}
