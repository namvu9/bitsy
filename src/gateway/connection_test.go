package gateway_test

import (
	"net"
	"testing"
	"time"

	"github.com/namvu9/bitsy/src/gateway"
)

func TestProxyDial(t *testing.T) {
	dialer := gateway.New(5)

	conn, err := dialer.Dial("tcp", "google.com:80")
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func(conn net.Conn) {
		time.Sleep(1 * time.Second)
		conn.Close()
		done <- struct{}{}
	}(conn)

	dialer.Dial("tcp", "google.com")
	dialer.Dial("tcp", "google.com")
	dialer.Dial("tcp", "google.com")
	dialer.Dial("tcp", "google.com")
	dialer.Dial("tcp", "google.com")

	<-done

	if got := dialer.Connections(); got != 1 {
		t.Errorf("Not all connections were properly closed: Want=%d Gor=%d", 1, got)
	}
}

func main() {
	// loadTorrents()
	// connection.New(cfg.MaxConns)
	// For each torrent:
	// - New TrackerGroup(torrent, Dialer)
	// - New MessagingService(torrent, Dialer/Listener)

}
