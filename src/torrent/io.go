package torrent

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/url"
	"strings"

	"github.com/namvu9/bitsy/src/bencode"
)

// Load a torrent into memory from either a magnet link or a
// file on disk
func Load(location string) (*Torrent, error) {
	url, err := url.Parse(location)
	if err != nil {
		return nil, err
	}

	if url.Scheme == "magnet" {
		return loadFromMagnetLink(url)
	}

	return loadFromFile(location)
}

func loadFromMagnetLink(u *url.URL) (*Torrent, error) {
	var dict bencode.Dictionary

	queries := u.Query()
	var trackerTier bencode.List
	trs, ok := queries["tr"]
	if !ok || len(trs) == 0 {
		return nil, fmt.Errorf("magnet link must specify at least 1 tracker")
	}

	for _, tracker := range queries["tr"] {
		trackerTier = append(trackerTier, bencode.Bytes(tracker))
	}

	dict.SetStringKey("announce", trackerTier[0])
	dict.SetStringKey("announce-list", bencode.List{trackerTier})

	xt := strings.Split(queries.Get("xt"), ":")
	protocol := xt[1]
	urn := xt[2]
	if protocol != "btih" {
		return nil, fmt.Errorf("Only BitTorrent URNs (btih) are supported")
	}
	hash, err := hex.DecodeString(urn)
	if err != nil {
		return nil, err
	}

	dict.SetStringKey("info-hash", bencode.Bytes(hash))
	dict.SetStringKey("dn", bencode.Bytes(queries.Get("dn")))

	return &Torrent{
		&dict,
	}, nil
}

func loadFromFile(path string) (*Torrent, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var v bencode.Value
	err = bencode.Unmarshal(data, &v)
	if err != nil {
		return nil, err
	}

	d, ok := v.ToDict()
	if !ok {
		return nil, fmt.Errorf("Bad torrent file")
	}

	t := &Torrent{dict: d}

	return t, nil
}

func Save(path string, t *Torrent) error {
	data, err := bencode.Marshal(t.Dict())
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, data, 0755)
}
