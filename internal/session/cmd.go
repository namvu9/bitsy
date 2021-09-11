package session

import (
	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/pkg/btorrent"
)

type PauseCmd struct {
	Hash [20]byte
}

type StartCmd struct {
	Hash [20]byte
}

type RegisterCmd struct {
	t    btorrent.Torrent
	opts []data.Option
}

type StatCmd struct {
	res chan map[string]interface{}
}
