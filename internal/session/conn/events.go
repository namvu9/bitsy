package conn

import "github.com/namvu9/bitsy/pkg/btorrent/peer"

type NewConnEvent struct {
	*peer.Peer
	Hash [20]byte
}

type ConnCloseEvent struct {
	*peer.Peer
	Hash [20]byte
}

