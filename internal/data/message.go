package data

import (
	"fmt"

	"github.com/namvu9/bitsy/pkg/btorrent/peer"
)

type messageReceived struct {
	sender *peer.Peer
	msg    peer.Message
}

type RequestMessage struct {
	Hash InfoHash
	peer.RequestMessage
}

func (c *Client) handlePieceMessage(msg peer.PieceMessage, workers map[int]*worker) (bool, error) {
	w, ok := workers[int(msg.Index)]
	if !ok {
		return false, fmt.Errorf("%d: unknown piece", msg.Index)
	}

	go func() {
		w.in <- msg
	}()

	return false, nil
}

func (c *Client) handleMessage(msg messageReceived) {
	switch v := msg.msg.(type) {
	case peer.PieceMessage:
		go func() {
			c.DataIn <- v
		}()
		// TODO: This isn't used anymore. Move to Session
	case peer.AllowedFastMessage:
		c.handleAllowedFastMessage(v, msg.sender)
	}
}

func (c *Client) handleAllowedFastMessage(msg peer.AllowedFastMessage, p *peer.Peer) {
	if c.pieces.Get(int(msg.Index)) {
		return
	}

	if int(msg.Index) >= len(c.torrent.Pieces()) {
		return
	}

	go c.downloadPiece(msg.Index, true)
}

func (c *Client) handleRequestMessage(req peer.RequestMessage) ([]byte, error) {
	if !c.pieces.Get(int(req.Index)) {
		return []byte{}, fmt.Errorf("client does not have piece %d", req.Index)
	}

	data, err := c.pieceMgr.Load(c.torrent.InfoHash(), int(req.Index))
	if err != nil {
		return []byte{}, err
	}

	data = data[req.Offset : req.Offset+req.Length]
	c.Uploaded += len(data)

	return data, nil
}
