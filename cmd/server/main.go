package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/namvu9/bitsy/internal/session"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	var (
		silent = flag.Bool("silent", false, "")
		debug  = flag.Bool("debug", false, "sets log level to debug")
	)
	flag.Parse()

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	if *silent {
		zerolog.SetGlobalLevel(zerolog.Disabled)
	}

	s := session.New(session.Config{
		BaseDir:        "./testdata",
		DownloadDir:    "./downloads",
		MaxConnections: 50,
		IP:             "192.168.0.4",
	})

	_, err := s.Init()
	if err != nil {
		log.Error().Msg(err.Error())
		os.Exit(1)
	}

	r := mux.NewRouter()
	r.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		data := s.Stat()
		json, _ := json.Marshal(data)
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Write(json)
	})
	r.HandleFunc("/torrents/{hash}", func(rw http.ResponseWriter, r *http.Request) {})


	http.ListenAndServe(":8080", r)
}
