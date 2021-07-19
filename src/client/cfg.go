package client

type Config struct {
	BaseDir        string
	DownloadDir    string
	MaxConnections int
	NatPMP         bool
	IP             string

	// The specified ports will be tried in order. The first
	// one that is successfully forwarded will be used for
	// incoming connections
	Port []uint16
}

// DEFAULTPORTS
var DEFAULTPORTS = []uint16{
	6881,
	6881,
	6882,
	6883,
	6884,
	6885,
	6886,
	6887,
	6888,
	6889,
}
