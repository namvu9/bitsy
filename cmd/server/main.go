package main

import (
	"flag"

	"github.com/rs/zerolog"
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

}
