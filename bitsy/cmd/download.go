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

	"github.com/namvu9/bitsy/internal/session"
	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/spf13/cobra"
)

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		t, err := btorrent.Load(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not load torrent: %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("--------\n%s\n-------\n", t.Name())
		fmt.Printf("%s\n", t.HexHash())
		fmt.Printf("Forwarding port... ")
		port, err := session.ForwardPorts([]uint16{6881})
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
				fmt.Println("Failed to fetch metadata", err)
				cancel()
				return
			}
			t = t2
			cancel()
		}
		fmt.Printf("done\n")
		fmt.Printf("Initiating session\n")

		s := session.New(session.Config{
			DownloadDir:    "./downloads",
			MaxConnections: 50,
			IP:             "192.168.0.4",
			Port:           port,
		}, *t)

		s.Init()

		select {}

	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// downloadCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// downloadCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}