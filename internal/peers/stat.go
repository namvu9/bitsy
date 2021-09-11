package peers

type SwarmStat struct {
	Peers       []PeerStat
	Choked      int
	Blocking    int
	Interested  int
	Interesting int
}

type PeerStat struct {
	IP         string `json:"ip"`
	UploadRate int    `json:"uploadRate"`
	Uploaded   int    `json:"uploaded"`
	Downloaded int    `json:"downloaded"`
}
