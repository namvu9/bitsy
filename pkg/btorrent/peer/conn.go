package peer

import (
	"context"
	"net"
	"sync"
	"time"
)

type DialConfig struct {
	PStr       string
	InfoHash   [20]byte
	PeerID     [20]byte
	Extensions *Extensions
	Timeout    time.Duration
}

// TODO: Do not handshake inside dial?
func Dial(ctx context.Context, addr net.Addr, cfg DialConfig) (*Peer, error) {
	conn, err := net.DialTimeout("tcp", addr.String(), cfg.Timeout)
	if err != nil {
		return nil, err
	}

	if isDone(ctx) {
		conn.Close()
		return nil, ctx.Err()
	}

	conn.SetDeadline(time.Now().Add(time.Second))
	msg := HandshakeMessage{
		PStrLen:  byte(len(cfg.PStr)),
		PStr:     cfg.PStr,
		InfoHash: cfg.InfoHash,
		PeerID:   cfg.PeerID,
		Reserved: cfg.Extensions.ReservedBytes(),
	}

	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = conn.Write(msg.Bytes())
	if err != nil {
		conn.Close()
		return nil, err
	}

	p := New(conn)
	err = p.Init()
	if err != nil {
		conn.Close()
		return nil, err
	}

	return p, nil
}

func Chunk(pl []net.Addr, size int) (out [][]net.Addr) {
	var chunk []net.Addr
	for i := 0; i < len(pl); i++ {
		if len(chunk) == size {
			out = append(out, chunk)
			chunk = []net.Addr{}
		}

		chunk = append(chunk, pl[i])
	}

	if len(chunk) > 0 {
		out = append(out, chunk)
	}

	return out
}

type PeerStream chan *Peer

// TODO: add context to cancel early
func DialMany(ctx context.Context, addrs []net.Addr, batchSize int, cfg DialConfig) PeerStream {
	out := make(chan *Peer, len(addrs))

	go func() {
		chunks := Chunk(addrs, batchSize)
		for _, chunk := range chunks {
			var wg sync.WaitGroup

			for _, addr := range chunk {
				wg.Add(1)
				go func(addr net.Addr) {
					defer wg.Done()
					pPeer, err := Dial(ctx, addr, cfg)
					if err != nil {
						return
					}
					out <- pPeer
				}(addr)
			}
			wg.Wait()
		}
		close(out)
	}()

	return out
}

