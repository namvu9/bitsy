package client

import (
	"fmt"
	"io/ioutil"
	"path"

	"github.com/namvu9/bitsy/internal/errors"
	"github.com/namvu9/bitsy/pkg/bits"
	"github.com/namvu9/bitsy/pkg/btorrent/peer"
	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
)

type messageReceived struct {
	sender *peer.Peer
	msg    peer.Message
}

func (c *Client) handlePieceMessage(msg peer.PieceMessage, workers map[int]*worker) (bool, error) {
	w, ok := workers[int(msg.Index)]
	if !ok {
		return false, fmt.Errorf("%d: unknown piece", msg.Index)
	}

	fmt.Println("BEFORE COMPLETE CHECK")
	if w.isComplete() {
		delete(workers, int(w.index))
		c.pieces.Set(int(w.index))

		c.MsgOut <- haveMessage(int(w.index))
		//c.msgOut <- CancelMessage(int(w.index))
		return true, nil
	}

	w.in <- msg

	return false, nil
}

func haveMessage(idx int) swarm.MulticastMessage {
	return swarm.MulticastMessage{
		Filter: func(p *peer.Peer) bool {
			return !p.HasPiece(idx)
		},
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				p.Send(peer.HaveMessage{Index: uint32(idx)})
			}
		},
	}
}

func cancelMessage(idx int) swarm.MulticastMessage {
	return swarm.MulticastMessage{
		Filter: func(p *peer.Peer) bool {
			return p.HasPiece(idx)
		},
		Handler: func(peers []*peer.Peer) {
			for _, p := range peers {
				p.Send(peer.CancelMessage{Index: uint32(idx)})
			}
		},
	}
}

func (c *Client) handleMessage(msg messageReceived) {
	switch v := msg.msg.(type) {
	case peer.HaveMessage:
		if !c.pieces.Get(int(v.Index)) {
			msg.sender.Send(peer.InterestedMessage{})
		}
	case peer.BitFieldMessage:
		c.handleBitfieldMessage(v, msg.sender)
	case peer.PieceMessage:
		go func() {
			c.DataIn <- v
		}()
	case peer.RequestMessage:
		c.handleRequestMessage(v, msg.sender)
	}
}

func (c *Client) subscribe(p *peer.Peer) {
	go func() {
		for {
			select {
			case msg := <-p.Msg:
				c.MsgIn <- messageReceived{
					sender: p,
					msg:    msg,
				}
			case <-c.doneCh:
				break
			}
		}
	}()
}

func (c *Client) handleBitfieldMessage(e peer.BitFieldMessage, p *peer.Peer) (bool, error) {
	var (
		t        = c.torrent
		maxIndex = bits.GetMaxIndex(e.BitField)
		pieces   = t.Pieces()
	)

	if maxIndex >= len(pieces) {
		err := errors.Newf("Invalid bitField length: Max index %d and len(pieces) = %d", maxIndex, len(pieces))
		return false, err
	}

	for i := range pieces {
		if !c.pieces.Get(i) && bits.BitField(e.BitField).Get(i) {
			go p.Send(peer.InterestedMessage{})

			return false, nil
		}
	}

	return false, nil
}

func (c *Client) handleRequestMessage(req peer.RequestMessage, p *peer.Peer) (bool, error) {
	filePath := path.Join(c.baseDir, c.torrent.HexHash(), fmt.Sprintf("%d.part", req.Index))

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return false, err
	}

	data = data[req.Offset : req.Offset+req.Length]

	msg := peer.PieceMessage{
		Index:  req.Index,
		Offset: req.Offset,
		Piece:  data,
	}
	c.Uploaded += len(data)

	go p.Send(msg)

	return true, nil
}
