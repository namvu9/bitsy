package peer

import "fmt"

var mainlineID = map[string]string{
	"M": "Mainline",
}

func peerIDStr(id []byte) string {
	if len(id) != 20 {
		return "Unknown"
	}

	if id[0] == 'M' {
		return fmt.Sprintf("Mainline %s", id[1:])
	}

	tag := string(id[1:3])

	if idFunc, ok := dashID[tag]; ok {
		return idFunc(id)
	}

	return "Unknown"
}

func qBittorrentClient(id []byte) string {
	major := string(id[3])
	minor := string(id[4])
	patch := string(id[5])

	return fmt.Sprintf("qBittorrent %s.%s.%s", major, minor, patch)
}

func utorrentClient(id []byte) string {
	major := string(id[3])
	minor := string(id[4])
	patch := string(id[5])

	var edition string
	if c := string(id[6]); c == "0" {
		edition = "stable"
	} else if c == "N" {
		edition = "nightly"
	} else if c == "B" {
		edition = "beta"
	}

	return fmt.Sprintf("µTorrent %s.%s.%s %s", major, minor, patch, edition)
}

func utorrentWebClient(id []byte) string {
	major := string(id[3])
	minor := string(id[4])
	patch := string(id[5])

	var edition string
	if c := string(id[6]); c == "0" {
		edition = "stable"
	} else if c == "N" {
		edition = "nightly"
	} else if c == "B" {
		edition = "beta"
	}

	return fmt.Sprintf("µTorrent Web %s.%s.%s %s", major, minor, patch, edition)
}

func transmissionClient(id []byte) string {
	major := string(id[3])
	minor := string(id[4])
	patch := string(id[5])

	var edition string
	if c := string(id[6]); c == "0" {
		edition = "stable"
	} else if c == "Z" {
		edition = "nightly"
	} else {
		edition = "prerelease"
	}

	return fmt.Sprintf("Transmission %s.%s.%s %s", major, minor, patch, edition)
}

var dashID = map[string]func([]byte) string{
	"AG": func(peerID []byte) string { return "Ares" },
	"A~": func(peerID []byte) string { return "Ares" },
	"AR": func(peerID []byte) string { return "Arctic" },
	"AV": func(peerID []byte) string { return "Avicora" },
	"AX": func(peerID []byte) string { return "BitPump" },
	"AZ": func(peerID []byte) string { return "Azureus" },
	"BB": func(peerID []byte) string { return "BitBuddy" },
	"BC": func(peerID []byte) string { return "BitComet" },
	"BF": func(peerID []byte) string { return "Bitflu" },
	"BG": func(peerID []byte) string { return "BTG (uses Rasterbar libtorrent)" },
	"BR": func(peerID []byte) string { return "BitRocket" },
	"BS": func(peerID []byte) string { return "BTSlave" },
	"BX": func(peerID []byte) string { return "~Bittorrent X" },
	"CD": func(peerID []byte) string { return "Enhanced CTorrent" },
	"CT": func(peerID []byte) string { return "CTorrent" },
	"DE": func(peerID []byte) string { return "DelugeTorrent" },
	"DP": func(peerID []byte) string { return "Propagate Data Client" },
	"EB": func(peerID []byte) string { return "EBit" },
	"ES": func(peerID []byte) string { return "electric sheep" },
	"FT": func(peerID []byte) string { return "FoxTorrent" },
	"FW": func(peerID []byte) string { return "FrostWire" },
	"FX": func(peerID []byte) string { return "Freebox BitTorrent" },
	"GS": func(peerID []byte) string { return "GSTorrent" },
	"HL": func(peerID []byte) string { return "Halite" },
	"HN": func(peerID []byte) string { return "Hydranode" },
	"KG": func(peerID []byte) string { return "KGet" },
	"KT": func(peerID []byte) string { return "KTorrent" },
	"LH": func(peerID []byte) string { return "LH-ABC" },
	"LP": func(peerID []byte) string { return "Lphant" },
	"LT": func(peerID []byte) string { return "libtorrent" },
	"lt": func(peerID []byte) string { return "libTorrent" },
	"LW": func(peerID []byte) string { return "LimeWire" },
	"MO": func(peerID []byte) string { return "MonoTorrent" },
	"MP": func(peerID []byte) string { return "MooPolice" },
	"MR": func(peerID []byte) string { return "Miro" },
	"MT": func(peerID []byte) string { return "MoonlightTorrent" },
	"NX": func(peerID []byte) string { return "Net Transport" },
	"PD": func(peerID []byte) string { return "Pando" },
	"qB": qBittorrentClient,
	"QD": func(peerID []byte) string { return "QQDownload" },
	"QT": func(peerID []byte) string { return "Qt 4 Torrent example" },
	"RT": func(peerID []byte) string { return "Retriever" },
	"S~": func(peerID []byte) string { return "Shareaza alpha/beta" },
	"SB": func(peerID []byte) string { return "~Swiftbit" },
	"SS": func(peerID []byte) string { return "SwarmScope" },
	"ST": func(peerID []byte) string { return "SymTorrent" },
	"st": func(peerID []byte) string { return "sharktorrent" },
	"SZ": func(peerID []byte) string { return "Shareaza" },
	"TN": func(peerID []byte) string { return "TorrentDotNET" },
	"TR": transmissionClient,
	"TS": func(peerID []byte) string { return "Torrentstorm" },
	"TT": func(peerID []byte) string { return "TuoTu" },
	"UL": func(peerID []byte) string { return "uLeecher!" },
	"UT": utorrentClient,
	"UW": utorrentWebClient,
	"VG": func(peerID []byte) string { return "Vagaa" },
	"WD": func(peerID []byte) string { return "WebTorrent Desktop" },
	"WT": func(peerID []byte) string { return "BitLet" },
	"WW": func(peerID []byte) string { return "WebTorrent" },
	"WY": func(peerID []byte) string { return "FireTorrent" },
	"XL": func(peerID []byte) string { return "Xunlei" },
	"XT": func(peerID []byte) string { return "XanTorrent" },
	"XX": func(peerID []byte) string { return "Xtorrent" },
	"ZT": func(peerID []byte) string { return "ZipTorrent" },
}
