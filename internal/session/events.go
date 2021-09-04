package session

import (
	"context"
	"fmt"
	"net"

	"github.com/namvu9/bitsy/internal/conn"
	"github.com/namvu9/bitsy/internal/data"
	"github.com/namvu9/bitsy/internal/peers"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

const ALLOWED_FAST_SET_SIZE = 10

func (s *Session) handleCloseConn(ev conn.ConnCloseEvent) {
	s.peers.Remove(ev.Hash, ev.Peer)
}

func (s *Session) handleDownloadCompleteEvent(ev data.DownloadCompleteEvent) {
	res := s.peers.Get(ev.Hash, peers.GetRequest{})
	for _, p := range res.Peers {
		go p.Send(peer.HaveMessage{Index: uint32(ev.Index)})
	}
}

func (s *Session) handleRequestMessage(msg data.RequestMessage) {
	res := s.peers.Get(msg.Hash, peers.GetRequest{
		Limit: 3,
		OrderBy: func(p1, p2 *peer.Peer) bool {
			if p1.UploadRate > p2.UploadRate {
				return true
			}

			if p1.UploadRate == p2.UploadRate && p1.Uploaded > p2.Uploaded {
				return true
			}

			return false

		},
		Filter: func(p *peer.Peer) bool {
			if p.HasPiece(int(msg.Index)) && s.isAllowedFast(msg.Hash, int(msg.Index), p) {
				return true
			}

			return p.HasPiece(int(msg.Index)) &&
				!p.Blocking &&
				!p.IsServing(msg.Index, msg.Offset) &&
				!p.Idle()
		},
	})

	for _, p := range res.Peers {
		go p.Send(msg.RequestMessage)
	}
}

var pieceFreq = make(map[int]int)

func (s *Session) handlePeerMessage(ev peers.MessageReceived) {
	pieces, err := s.data.Pieces(ev.Hash)
	if err != nil {
		fmt.Println(err)
		return
	}

	switch msg := ev.Msg.(type) {
	case peer.AllowedFastMessage:
		s.data.RequestPiece(ev.Hash, int(msg.Index))

	case peer.BitFieldMessage:
		t := s.torrents[ev.Hash]
		for idx := range t.Pieces() {
			if ev.Peer.HasPiece(idx) {
				pieceFreq[idx]++
			}

			if !pieces.Get(idx) {
				ev.Peer.Send(peer.InterestedMessage{})
			}
		}
	case peer.HaveMessage:
		pieceFreq[int(msg.Index)]++
		if !pieces.Get(int(msg.Index)) {
			ev.Peer.Send(peer.InterestedMessage{})
		}
	case peer.RequestMessage:
		data, err := s.data.GetPiece(ev.Hash, msg)
		if err != nil {
			p := ev.Peer
			if p.Extensions.IsEnabled(peer.EXT_FAST) {
				p.Send(peer.RejectRequestMsg{
					Index:  msg.Index,
					Offset: msg.Offset,
					Length: msg.Length,
				})
			}
			return
		}

		ev.Peer.Send(peer.PieceMessage{
			Index:  msg.Index,
			Offset: msg.Offset,
			Piece:  data,
		})
	}
}

func (s *Session) handleNewConn(ev conn.NewConnEvent) {
	p := ev.Peer
	p.OnClose(func(p *peer.Peer) {
		s.peers.Remove(ev.Hash, p)
	})

	status, err := s.data.Status(ev.Hash)
	if err != nil {
		p.Close(err.Error())
		return
	}

	if status == data.PAUSED || status == data.ERROR {
		p.Close(err.Error())
		return
	}

	if status == data.STOPPED {
		s.data.Init(context.Background())
	}

	err = s.data.AddDataStream(ev.Hash, p)
	if err != nil {
		p.Close(err.Error())
		return
	}

	bitField, err := s.data.Pieces(ev.Hash)
	if err != nil {
		p.Close(err.Error())
		return
	}

	if p.Extensions.IsEnabled(peer.EXT_FAST) {
		go s.sendFastBitfield(ev.Hash, p)
		go s.sendAllowFastSet(ev.Hash, p)
	}

	go p.Send(peer.BitFieldMessage{BitField: bitField})

	s.peers.Add(ev.Hash, p)
}

func (s *Session) sendFastBitfield(hash [20]byte, p *peer.Peer) {
	var (
		t         = s.torrents[hash]
		pieces, _ = s.data.Pieces(hash)
		haveN     = pieces.GetSum()
	)

	if haveN == len(t.Pieces()) {
		p.Send(peer.HaveAllMessage{})
		return
	}

	if haveN == 0 {
		p.Send(peer.HaveNoneMessage{})
		return
	}

	p.Send(peer.BitFieldMessage{BitField: pieces})
}

func (s *Session) sendAllowFastSet(hash [20]byte, p *peer.Peer) {
	for _, pieceIdx := range s.computeAllowedFastSet(hash, p) {
		go p.Send(peer.AllowedFastMessage{Index: uint32(pieceIdx)})
	}
}

func (s *Session) computeAllowedFastSet(hash [20]byte, p *peer.Peer) []int {
	addr := p.RemoteAddr().(*net.TCPAddr)
	t := s.torrents[hash]

	return t.GenFastSet(addr.IP, ALLOWED_FAST_SET_SIZE)
}

func (s *Session) isAllowedFast(hash [20]byte, idx int, p *peer.Peer) bool {
	for _, pieceIdx := range s.computeAllowedFastSet(hash, p) {
		if pieceIdx == idx {
			return true
		}
	}

	return false
}
