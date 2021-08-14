package ch

import (
	"context"
)

func Pipe(ctx context.Context, in, out chan interface{}) {
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

type MapFn func(interface{}) interface{}

func PipeFn(ctx context.Context, fn MapFn, in, out chan interface{}) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case v, ok := <-in:
				if !ok {
					return
				}

				out <- fn(v)
			}
		}
	}()

}

func Spread(ctx context.Context, in chan interface{}, out ...chan interface{}) {
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
