package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/namvu9/bitsy/src/client"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	debug := flag.Bool("debug", false, "sets log level to debug")
	flag.Parse()

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if *debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	zerolog.SetGlobalLevel(zerolog.Disabled)

	s := client.New(client.Config{
		BaseDir:        "./testdata",
		DownloadDir:    "./downloads",
		NatPMP:         true,
		MaxConnections: 30,
		IP:             "192.168.0.4",
	})

	_, err := s.Init()
	if err != nil {
		log.Error().Msg(err.Error())
		os.Exit(1)
	}

	ticker := time.NewTicker(10 * time.Second)
	for {
		<-ticker.C
		//cmd := exec.Command("clear") //Linux example, its tested
		//cmd.Stdout = os.Stdout
		//cmd.Run()
		stat := s.Stat()
		fmt.Println(stat)
	}
}
