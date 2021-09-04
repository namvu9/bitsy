package pieces

import (
	"fmt"
	"os"
	"path"

	"github.com/namvu9/bitsy/pkg/btorrent"
)

type Service interface {
	Verify([20]byte, int, []byte) error

	Save([20]byte, int, []byte) error

	// Load reads the specified piece into memory and returns
	// it
	Load([20]byte, int) ([]byte, error)

	Init() error
	Register(btorrent.Torrent)
}

// TODO: Make safe for concurrent use
type pieceManager struct {
	torrents map[[20]byte]btorrent.Torrent
	data     map[[20]byte]map[int][]byte
	requests map[[20]byte]map[int]int

	baseDir string
	memory  uint64
}

func (pm *pieceManager) Save(hash [20]byte, idx int, data []byte) error {
	t, ok := pm.torrents[hash]
	if !ok {
		return fmt.Errorf("unknown hash: %x", hash)
	}

	filePath := path.Join(pm.baseDir, t.HexHash(), fmt.Sprintf("%d.part", idx))
	os.WriteFile(filePath, data, 0777)

	return nil
}

func (pm *pieceManager) Load(hash [20]byte, idx int) ([]byte, error) {
	t, ok := pm.torrents[hash]
	if !ok {
		return []byte{}, fmt.Errorf("unknown hash: %x", hash)
	}

	pm.requests[hash][idx]++

	if piece, ok := pm.data[hash][idx]; ok {
		return piece, nil
	}

	filePath := path.Join(pm.baseDir, t.HexHash(), fmt.Sprintf("%d.part", idx))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return []byte{}, err
	}

	pm.data[hash][idx] = data

	return data, nil
}

func (pm *pieceManager) Verify(hash [20]byte, idx int, data []byte) error {
	t, ok := pm.torrents[hash]
	if !ok {
		return fmt.Errorf("unknown hash: %x", hash)
	}

	verified := t.VerifyPiece(idx, data)
	if !verified {
		return fmt.Errorf("piece %d is corrupted", idx)
	}

	return nil
}

func (pm *pieceManager) Register(t btorrent.Torrent) {
	pm.torrents[t.InfoHash()] = t
	pm.data[t.InfoHash()] = make(map[int][]byte)
	pm.requests[t.InfoHash()] = make(map[int]int)
}

func (pm *pieceManager) Init() error {
	return nil
}

type Config struct {
	BaseDir string
	Memory  uint64
}

func NewService(cfg Config) Service {
	return &pieceManager{
		torrents: make(map[[20]byte]btorrent.Torrent),
		data:     make(map[[20]byte]map[int][]byte),
		requests: make(map[[20]byte]map[int]int),

		baseDir: cfg.BaseDir,
		memory:  cfg.Memory,
	}
}
