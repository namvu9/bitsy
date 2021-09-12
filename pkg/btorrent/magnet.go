package btorrent

import (
	"encoding/hex"
	"fmt"
	stdurl "net/url"
	"strings"

	"github.com/namvu9/bencode"
)

func LoadMagnetURL(url *stdurl.URL) (*Torrent, error) {
	var dict bencode.Dictionary

	trackers, err := getTrackers(url)
	if err != nil {
		return nil, err
	}

	dict.SetStringKey("announce", trackers[0])
	dict.SetStringKey("announce-list", trackers)

	hash, err := getExactTopic(url)
	if err != nil {
		return nil, err
	}

	dict.SetStringKey("info-hash", bencode.Bytes(hash))
	dict.SetStringKey("dn", bencode.Bytes(url.Query().Get("dn")))

	return &Torrent{
		dict: &dict,
	}, nil
}

func getExactTopic(url *stdurl.URL) ([]byte, error) {
	queryParams := url.Query()
	var (
		xt       = strings.Split(queryParams.Get("xt"), ":")
		protocol = xt[1]
		urn      = xt[2]
	)
	if protocol != "btih" {
		return nil, fmt.Errorf("Only BitTorrent URNs (btih) are supported")
	}
	hash, err := hex.DecodeString(urn)
	return hash, err
}

func getTrackers(url *stdurl.URL) (bencode.List, error) {
	queryParams := url.Query()

	var trackerTier bencode.List
	trs, ok := queryParams["tr"]
	if !ok || len(trs) == 0 {
		return nil, fmt.Errorf("magnet link must specify at least 1 tracker")
	}

	for _, tracker := range queryParams["tr"] {
		trackerTier = append(trackerTier, bencode.Bytes(tracker))
	}

	return trackerTier, nil
}

