package session

type Config struct {
	BaseDir        string
	DownloadDir    string // Where to write the completed files to
	MaxConnections int
	IP             string
	Ports          []uint16
}
