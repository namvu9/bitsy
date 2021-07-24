package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"

	"github.com/namvu9/bitsy"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	silent := flag.Bool("silent", false, "")
	debug := flag.Bool("debug", false, "sets log level to debug")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	if *silent {
		zerolog.SetGlobalLevel(zerolog.Disabled)
	}

	s := bitsy.New(bitsy.Config{
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

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		data := s.Stat()
		json, _ := json.Marshal(data)
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Write(json)
	})

	http.ListenAndServe(":8080", nil)
}
