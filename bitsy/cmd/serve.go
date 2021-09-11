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
	"encoding/json"
	"fmt"
	glog "log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/gorilla/mux"
	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/internal/session"
	"github.com/spf13/cobra"
)

// spaHandler implements the http.Handler interface, so we can use it
// to respond to HTTP requests. The path to the static directory and
// path to the index file within that static directory are used to
// serve the SPA in the given static directory.
type spaHandler struct {
	staticPath string
	indexPath  string
}

// ServeHTTP inspects the URL path to locate a file within the static dir
// on the SPA handler. If a file is found, it will be served. If not, the
// file located at the index path on the SPA handler will be served. This
// is suitable behavior for serving an SPA (single page application).
func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the absolute path to prevent directory traversal
	path, err := filepath.Abs(r.URL.Path)
	if err != nil {
		// if we failed to get the absolute path respond with a 400 bad request
		// and stop
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// prepend the path with the path to the static directory
	path = filepath.Join(h.staticPath, path)

	// check whether a file exists at the given path
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// file does not exist, serve index.html
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if err != nil {
		// if we got an error (that wasn't that the file doesn't exist) stating the
		// file, return a 500 internal server error and stop
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// otherwise, use http.FileServer to serve the static dir
	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start/Resume a torrent download",
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
			Ports:          []uint16{6881},
		})

		opts := []data.Option{}
		if len(files) > 0 {
			opts = append(opts, data.WithFiles(files...))
		}

		fmt.Printf("Initiating session... ")
		err = s.Init()
		if err != nil {
			panic(err)
		}

		fmt.Printf("done\n")

		r := mux.NewRouter()

		r.HandleFunc("/api/torrents", func(rw http.ResponseWriter, r *http.Request) {
			jsonData, err := json.MarshalIndent(s.Stat(), "", " ")
			if err != nil {
				rw.Write([]byte(err.Error()))
				return
			}

			rw.Write(jsonData)
		})

		spa := spaHandler{staticPath: "./client/build", indexPath: "index.html"}
		r.PathPrefix("/").Handler(spa)

		srv := &http.Server{
			Handler: r,
			Addr:    "127.0.0.1:8000",
			// Good practice: enforce timeouts for servers you create!
			WriteTimeout: 15 * time.Second,
			ReadTimeout:  15 * time.Second,
		}

		//go openbrowser("http://localhost:8000")
		glog.Fatal(srv.ListenAndServe())
	},
}

func openbrowser(url string) {
	var err error

	switch runtime.GOOS {
	case "linux":
		err = exec.Command("firefox", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		err = exec.Command("open", url).Start()
	default:
		err = fmt.Errorf("unsupported platform")
	}
	if err != nil {
		glog.Fatal(err)
	}

}

func init() {
	rootCmd.AddCommand(serveCmd)
}
