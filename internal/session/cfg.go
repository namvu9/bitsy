package session

// Config represents the configuration of stuff.
// Not really important
type Config struct {
	BaseDir        string
	DownloadDir    string // Where to write the completed files to
	MaxConnections int
	IP             string
	Ports          []uint16
}
