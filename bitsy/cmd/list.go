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
	"fmt"
	"os"

	"github.com/namvu9/bitsy/pkg/btorrent"
	"github.com/spf13/cobra"
)

// downloadCmd represents the download command
var listCmd= &cobra.Command{
	Use: "list",
	Short: "List active torrents",
	Run: func(cmd *cobra.Command, args []string) {
		baseDir, err := BaseDir()
		if err != nil {
			fmt.Println(err)
			return
		}

		torrents, err := btorrent.LoadDir(baseDir)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		for _, t := range torrents {
			fmt.Println(t.HexHash(), t.Name())
		}
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
