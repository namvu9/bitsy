package peers

import "github.com/namvu9/bitsy/pkg/btorrent/peer"

type JoinEvent struct {
	*peer.Peer
	Hash [20]byte
}

type LeaveEvent struct {
	*peer.Peer
	Hash [20]byte
}

type MessageReceived struct {
	*peer.Peer
	Hash [20]byte
	Msg  interface{}
}
