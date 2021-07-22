package tracker_test

import (
	"net"
	"time"
)

type NetConnMock struct {
	values map[string]interface{}

	writeFn func(NetConnMock, []byte) (int, error)
	readFn  func(NetConnMock, []byte) (int, error)
}

func (mock NetConnMock) Close() error { return nil }
func (mock NetConnMock) Write(src []byte) (int, error) {
	if mock.writeFn != nil {
		return mock.writeFn(mock, src)
	}

	return 0, nil
}
func (mock NetConnMock) Read(b []byte) (int, error) {
	if mock.readFn != nil {
		return mock.readFn(mock, b)
	}

	return 0, nil
}
func (mock NetConnMock) LocalAddr() net.Addr                   { return &net.IPAddr{} }
func (mock NetConnMock) RemoteAddr() net.Addr                  { return &net.IPAddr{} }
func (mock NetConnMock) SetDeadline(time.Time) error           { return nil }
func (mock NetConnMock) SetReadDeadline(time.Time) error       { return nil }
func (mock NetConnMock) SetWriteDeadline(time.Time) error      { return nil }
func (mock NetConnMock) Dial(string, string) (net.Conn, error) { return mock, nil }
func (mock NetConnMock) CanDial() bool                         { return true }

//func TestUDPConnect(t *testing.T) {
	//input := "udp://tracker.opentrackr.org:1337"
	//url, _ := url.Parse(input)

	//connID, err := tracker.UDPConnect(url, d)

	//if err != nil {
		//t.Fatal(err)
	//}

	//if connID != 1337 {
		//t.Errorf("Want connID=%d got=%d", 1337, connID)
	//}

//}

//func TestUDPAnnounce(t *testing.T) {
//input := "udp://tracker.opentrackr.org:1337"
//url, _ := url.Parse(input)

//connID, err := tracker.UDPConnect(url, tracker.DialFunc(net.Dial))
//if err != nil {
//t.Fatal(err)
//}

//fmt.Println("CONNECTED", connID)

//var hash [20]byte
//hashBytes, _ := hex.DecodeString("e096a53d5ce9ef53c7d5c47b9086833b1e8a1778")
//copy(hash[:], hashBytes)

//req := tracker.NewUDPAnnounceReq(hash, connID, 6991)

//resp, err := tracker.UDPAnnounce(url, req)
//if err != nil {
//t.Fatal(err)
//}

//fmt.Printf("ANNOUNCED: %+v\n", resp)
//}

//func TestTrackerGroup(t *testing.T) {
	//url := `magnet:?xt=urn:btih:e096a53d5ce9ef53c7d5c47b9086833b1e8a1778&dn=%5BTorrentCouch.com%5D.Silicon.Valley.S05.Complete.720p.BRRip.x264.ESubs.%5B1.6GB%5D.%5BSeason.5.Full%5D&tr=udp%3A%2F%2Ftracker.coppersurfer.tk%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.openbittorrent.com%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.opentrackr.org%3A1337&tr=udp%3A%2F%2Fmovies.zsw.ca%3A6969%2Fannounce&tr=udp%3A%2F%2Ftracker.dler.org%3A6969%2Fannounce&tr=udp%3A%2F%2Fopentracker.i2p.rocks%3A6969%2Fannounce&tr=udp%3A%2F%2Fopen.stealth.si%3A80%2Fannounce&tr=udp%3A%2F%2Ftracker.0x.tf%3A6969%2Fannounce`
	//torr, err := torrent.Load(url)
	//if err != nil {
		//t.Fatal(err)
	//}

	//trg := tracker.New(*torr, nil)
	//ch := trg.Listen()

	//resp := <-ch
//}
