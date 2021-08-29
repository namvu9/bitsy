package peer

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"

	"github.com/namvu9/bencode"
)

const (
	META_REQUEST = 0
	META_DATA    = 1
	META_REJECT  = 2
)

func RequestInfoDict(code, size int) []MetaRequestMessage {
	pieceSize := 16 * 1024
	var nPieces int
	if size%pieceSize == 0 {
		nPieces = size / pieceSize
	} else {
		nPieces = size/pieceSize + 1
	}

	var out []MetaRequestMessage

	for i := 0; i < nPieces; i++ {
		req := MetaRequestMessage{Piece: i, Code: byte(code)}
		out = append(out, req)
	}

	return out
}

type MetaPieceMessage struct {
	Index int
	Data  []byte
}

func (msg *MetaPieceMessage) Bytes() []byte {
	return []byte{}
}

func unmarshalMetaMsg(data []byte) (Message, error) {
	d, err := bencode.UnmarshalDict(data)
	if err != nil {
		return nil, err
	}

	msgType, ok := d.GetInteger("msg_type")
	if !ok {
		return nil, fmt.Errorf("malformed data")
	}

	switch msgType {
	case META_REQUEST:
	case META_DATA:
		index, _ := d.GetInteger("piece")
		return &MetaPieceMessage{
			Index: int(index),
			Data:  data[d.Size():],
		}, nil
	case META_REJECT:
	}

	return &MetaPieceMessage{}, nil
}

type MetaRequestMessage struct {
	Piece int
	Code  byte
}

func (msg *MetaRequestMessage) Bytes() []byte {
	var buf bytes.Buffer

	var d bencode.Dictionary
	d.SetStringKey("msg_type", bencode.Integer(META_REQUEST))
	d.SetStringKey("piece", bencode.Integer(msg.Piece))
	payload, err := bencode.Marshal(&d)
	if err != nil {
		return buf.Bytes()
	}

	binary.Write(&buf, binary.BigEndian, uint32(len(payload)+2))
	buf.WriteByte(Extended)
	buf.WriteByte(msg.Code)
	buf.Write(payload)

	return buf.Bytes()
}

func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func getInfoDict(ctx context.Context, p *Peer, code, size int) (*bencode.Dictionary, error) {
	if isDone(ctx) {
		return nil, ctx.Err()
	}

	reqs := RequestInfoDict(code, size)
	for _, req := range reqs {
		p.Send(&req)
	}

	pieces := make([][]byte, len(reqs))
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case msg := <-p.Msg:
			v, ok := msg.(*MetaPieceMessage)
			if !ok {
				continue
			}

			pieces[v.Index] = v.Data

			if sizeOf(pieces) == size {
				d, err := bencode.UnmarshalDict(flatten(pieces))
				if err != nil {
					return nil, err
				}

				return d, nil
			}
		}
	}
}

func GetInfoDict(ctx context.Context, peers []net.Addr, cfg DialConfig) (*bencode.Dictionary, error) {
	extHandshake := new(ExtHandshakeMsg)
	extHandshake.M().SetStringKey(EXT_UT_META, bencode.Integer(EXT_CODE_META))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		res       = make(chan *bencode.Dictionary)
		batchSize = 50
	)

	if len(peers) == 0 {
		panic("expected at least 1 peer address")
	}
	ch := DialMany(ctx, peers, batchSize, cfg)
	for {
		select {
		case out := <-res:
			return out, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case p := <-ch:
			if p == nil {
				continue
			}
			if !p.Extensions.IsEnabled(EXT_PROT) {
				p.Close("peer does not support EXT_PROT")
				continue
			}

			go func(p *Peer) {
				ctx, cancel := context.WithCancel(ctx)
				defer func() {
					p.Close("done")
					cancel()
				}()

				if err := p.Send(extHandshake); err != nil {
					return
				}

				msg, ok := (<-p.Msg).(*ExtHandshakeMsg)
				if !ok {
					return
				}

				size, ok := msg.D().GetInteger(EXT_UT_META_SIZE)
				if !ok {
					return
				}

				code, ok := msg.M().GetInteger(EXT_UT_META)
				if !ok {
					return
				}

				info, err := getInfoDict(ctx, p, int(code), int(size))
				if err != nil {
					return
				}

				res <- info
				return
			}(p)
		}
	}
}
