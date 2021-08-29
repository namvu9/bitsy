package data

import (
	"fmt"
	"io/ioutil"
	"path"

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
	//if !c.pieces.Get(int(req.Index)) {
	//if p.Extensions.IsEnabled(peer.EXT_FAST) {
	//p.Send(peer.RejectRequestMessage{
	//Index:  req.Index,
	//Offset: req.Offset,
	//Length: req.Length,
	//})
	//}

	//return []byte{}, fmt.Errorf("Client does not have requested piece %d", req.Index)
	//}

	filePath := path.Join(c.baseDir, c.torrent.HexHash(), fmt.Sprintf("%d.part", req.Index))

	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return []byte{}, err
	}

	data = data[req.Offset : req.Offset+req.Length]
	c.Uploaded += len(data)

	return data, nil
}
