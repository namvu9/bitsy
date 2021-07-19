package torrent

import (
	"encoding/hex"
	"io/ioutil"
	"net/url"
	"strings"

	"github.com/namvu9/bitsy/src/bencode"
	"github.com/namvu9/bitsy/src/errors"
)

// Load a torrent into memory from either a magnet link or a
// file on disk
func Load(location string) (*Torrent, error) {
	var op errors.Op = "torrent.Load"

	url, err := url.Parse(location)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	if url.Scheme == "magnet" {
		return loadFromMagnetLink(url)
	}

	return loadFromFile(location)
}

func loadFromMagnetLink(u *url.URL) (*Torrent, error) {
	var op errors.Op = "torrent.loadFromMagnetLink"
	var dict bencode.Dictionary

	queries := u.Query()

	var trackerTier bencode.List
	trs, ok := queries["tr"]
	if !ok || len(trs) == 0 {
		err := errors.New("magnet link must specify at least 1 tracker")
		return nil, errors.Wrap(err, op, errors.BadArgument)
	}

	for _, tracker := range queries["tr"] {
		trackerTier = append(trackerTier, bencode.Bytes(tracker))
	}

	dict.SetStringKey("announce", trackerTier[0])
	dict.SetStringKey("announce-list", bencode.List{trackerTier})

	var (
		xt       = strings.Split(queries.Get("xt"), ":")
		protocol = xt[1]
		urn      = xt[2]
	)
	if protocol != "btih" {
		err := errors.New("Only BitTorrent URNs (btih) are supported")
		return nil, errors.Wrap(err, op, errors.BadArgument)
	}
	hash, err := hex.DecodeString(urn)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	dict.SetStringKey("info-hash", bencode.Bytes(hash))
	dict.SetStringKey("dn", bencode.Bytes(queries.Get("dn")))

	return &Torrent{
		dict: &dict,
	}, nil
}

func loadFromFile(path string) (*Torrent, error) {
	var op errors.Op = "torrent.loadFromFile"

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.IO)
	}

	var v bencode.Value
	err = bencode.Unmarshal(data, &v)
	if err != nil {
		return nil, errors.Wrap(err, op)
	}

	d, ok := v.ToDict()
	if !ok {
		err := errors.New("bad torrent file")
		return nil, errors.Wrap(err, op, errors.BadArgument)
	}

	t := &Torrent{dict: d}

	return t, nil
}

func Save(path string, t *Torrent) error {
	var op errors.Op = "torrent.Save"

	data, err := bencode.Marshal(t.Dict())
	if err != nil {
		return errors.Wrap(err, op)
	}

	err = ioutil.WriteFile(path, data, 0755)
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	return nil
}
