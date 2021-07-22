package bitsy_test

import (
	"net"
	"sync"
	"testing"

	"github.com/namvu9/bitsy"
)

func testServer(t *testing.T, network, addr string) func() {
	t.Helper()
	listener, _ := net.Listen(network, addr)
	go func() {
		for {
			listener.Accept()
		}
	}()

	return func() {
		listener.Close()
	}
}

func TestBoundedDial(t *testing.T) {
	addr := "localhost:8081"
	stop := testServer(t, "tcp", addr)
	defer stop()

	for i, tc := range []struct {
		name     string
		calls    int
		nCalls   int
		closes   int
		timeouts int
	}{
		{
			calls:    6,
			nCalls:   5,
			timeouts: 1,
		},
		{
			calls:    9,
			nCalls:   5,
			timeouts: 4,
		},
		{
			calls:    5,
			nCalls:   5,
			timeouts: 0,
		},
		{
			calls:    9,
			nCalls:   8,
			timeouts: 1,
			closes:   3,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bn := bitsy.NewBoundedNet(5)
			defer bn.Stop()

			var timeouts = make(chan struct{}, tc.calls)
			var calls = make(chan struct{}, tc.calls)
			var wg sync.WaitGroup

			for i := 0; i < tc.calls; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					conn, err := bn.Dial("tcp", addr)
					if _, ok := err.(bitsy.TimeoutErr); ok {
						timeouts <- struct{}{}
					}

					if err == nil {
						calls <- struct{}{}
					}

					if i < tc.closes {
						conn.Close()
					}
				}(i)
			}

			wg.Wait()

			if got := len(timeouts); tc.timeouts != got {
				t.Errorf("%d: Timeouts want %d got %d", i, tc.timeouts, got)
			}

			if got := len(calls); tc.nCalls != got {
				t.Errorf("%d: Calls want %d got %d", i, tc.nCalls, got)
			}
		})

	}
}

func TestBoundedListener(t *testing.T) {
	for i, tc := range []struct {
		name     string
		calls    int
		nCalls   int
		closes   int
		timeouts int
	}{
		{
			calls:    5,
			nCalls:   2,
			timeouts: 3,
		},
		{
			calls:    2,
			nCalls:   2,
			timeouts: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addr := "localhost:8080"
			bn := bitsy.NewBoundedNet(5)
			defer bn.Stop()

			listener, err := bn.Listen("tcp", addr)
			if err != nil {
				t.Error(err)
			}
			defer listener.Close()

			go func() {
				for {
					listener.Accept()
				}
			}()

			var timeouts = make(chan struct{}, tc.calls)
			var calls = make(chan struct{}, tc.calls)

			for i := 0; i < tc.calls; i++ {
				_, err := bn.Dial("tcp", addr)
				if _, ok := err.(bitsy.TimeoutErr); ok {
					timeouts <- struct{}{}
				}

				if err == nil {
					calls <- struct{}{}
				}
			}

			if got := len(timeouts); tc.timeouts != got {
				t.Errorf("%d: Timeouts want %d got %d", i, tc.timeouts, got)
			}

			if got := len(calls); tc.nCalls != got {
				t.Errorf("%d: Calls want %d got %d", i, tc.nCalls, got)
			}
		})

	}
}
