package session

import (
	"context"
	"fmt"
	"net"

	"github.com/namvu9/bitsy/internal/session/conn"
	"github.com/namvu9/bitsy/internal/session/data"
	"github.com/namvu9/bitsy/internal/session/peers"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

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
		Limit: 2,
		Filter: func(p *peer.Peer) bool {
			// TODO: OR FAST
			return p.HasPiece(int(msg.Index)) &&
				!p.Blocking &&
				!p.IsServing(msg.Index, msg.Offset)
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
		// TODO: IMPLEMENT
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
			fmt.Println(err)
			p := ev.Peer
			if p.Extensions.IsEnabled(peer.EXT_FAST) {
				p.Send(peer.RejectRequestMessage{
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
	status, err := s.data.Status(ev.Hash)
	if err != nil {
		p.Close(err.Error())
		return
	}

	if status == data.PAUSED || status == data.ERROR {
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

	s.peers.Add(ev.Hash, p)

	if p.Extensions.IsEnabled(peer.EXT_FAST) {
		go s.sendFastBitfield(ev.Hash, p)
		go s.sendAllowFastSet(ev.Hash, p)
		return
	}

	go p.Send(peer.BitFieldMessage{BitField: bitField})
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
	addr := p.RemoteAddr().(*net.TCPAddr)
	t := s.torrents[hash]

	for _, pieceIdx := range t.GenFastSet(addr.IP, 10) {
		go p.Send(peer.AllowedFastMessage{Index: uint32(pieceIdx)})
	}
}
