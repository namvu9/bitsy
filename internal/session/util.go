package session

import (
	"context"

	"github.com/namvu9/bitsy/pkg/btorrent/swarm"
)

// Stuff I don't know where to put -_-

func Pipe(ctx context.Context, in, out chan swarm.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
				out <- ev
			}
		}
	}()
}

func Spread(ctx context.Context, in chan swarm.Event, out ...chan swarm.Event) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-in:
				for _, outCh := range out {
					select {
					case outCh <- ev:
					default:
					}
				}
			}
		}
	}()
}

func Merge(ctx context.Context, in ...chan swarm.Event) chan swarm.Event {
	out := make(chan swarm.Event)

	go func() {
		var open int
		for {
			for _, c := range in {
				select {
				case e, ok := <-c:
					if !ok {
						continue
					}

					open++
					out <- e
				default:
				}
			}

			if open == 0 {
				return
			}
		}
	}()

	return out
}
