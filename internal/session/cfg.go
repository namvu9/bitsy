package session

type Config struct {
	BaseDir        string
	DownloadDir    string // Where to write the completed files to
	MaxConnections int
	IP             string

	// The specified ports will be tried in order. The first
	// one that is successfully forwarded will be used for
	// incoming connections
	Port uint16
}
