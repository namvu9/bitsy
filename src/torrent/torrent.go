package torrent

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/namvu9/bitsy/src/bencode"
)

// Torrent contains metadata for one or more files and wraps
// a bencoded dictionary.
type Torrent struct {
	dict *bencode.Dictionary
}

// Dict returns the torrent's underlying bencoded dictionary
func (t *Torrent) Dict() *bencode.Dictionary {
	return t.dict
}

// Info returns the torrent's info dictionary and true if
// the dictionary exists, otherwise it returns nil and false
func (t *Torrent) Info() (*bencode.Dictionary, bool) {
	v, ok := t.dict.GetDict("info")
	if !ok {
		return nil, false
	}

	return v, true
}

// Name returns the name of the torrent if it exists.
// Returns the empty string otherwise
func (t *Torrent) Name() string {
	// First try to get the value of the 'name' field from the
	// info dictionary
	info, ok := t.Info()
	if ok {
		name, _ := info.GetString("name")
		return name
	}

	// Try the 'dn' field if the torrent was created from a
	// magnet link
	name, _ := t.dict.GetString("dn")
	return name
}

// InfoHash returns the SHA-1 hash of the bencoded value of the
// torrent's info field. The hash uniquely identifies the
// torrent.
func (t *Torrent) InfoHash() *[20]byte {
	var out = new([20]byte)
	b, ok := t.dict.GetBytes("info-hash")
	if ok {
		copy(out[:], b)
		return out
	}

	d, _ := t.Info()
	data, err := bencode.Marshal(d)
	if err != nil {
		return nil
	}

	hash := sha1.Sum(data)
	t.dict.SetStringKey("info-hash", bencode.Bytes(hash[:]))
	return &hash
}

// HexHash returns the hex-encoded SHA-1 hash of the
// bencoded value of the torrent's info field
func (t *Torrent) HexHash() string {
	hash := t.InfoHash()
	return hex.EncodeToString(hash[:])
}

// Announce returns the url of the torrent's tracker
func (t *Torrent) Announce() string {
	s, ok := t.dict.GetString("announce")
	if !ok {
		return ""
	}

	return s
}

// AnnounceList returns a list of lists of tracker URLs, as
// defined in BEP-12
func (t *Torrent) AnnounceList() [][]string {
	var out [][]string

	l, ok := t.dict.GetList("announce-list")
	if !ok {
		return out
	}

	for _, tier := range l {
		v, ok := tier.ToList()
		if !ok {
			continue
		}

		var trackers []string

		for _, s := range v {
			trackerURL, _ := s.ToBytes()
			trackers = append(trackers, string(trackerURL))
		}

		out = append(out, trackers)
	}

	return out
}

func (t *Torrent) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Torrent: %s\n", t.Name()))
	sb.WriteString(fmt.Sprintf("Info Hash: %s\n", t.HexHash()))
	sb.WriteString("Trackers:\n")

	for _, tracker := range t.AnnounceList() {
		sb.WriteString(fmt.Sprintf("%s\n", tracker))
	}

	return sb.String()
}
