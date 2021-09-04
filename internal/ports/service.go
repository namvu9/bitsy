package ports

import (
	"fmt"

	"gitlab.com/NebulousLabs/go-upnp"
)

type Service interface {
	Forward(uint16) error
	ForwardMany([]uint16) (uint16, error)
}

type ports struct {
	d *upnp.IGD
}

func (p *ports) Forward(port uint16) error {
	if p.d == nil {
		// Discover UPnP-supporting routers
		d, err := upnp.Discover()
		if err != nil {
			return err
		}

		p.d = d
	}

	err := p.d.Forward(port, "Bitsy BitTorrent client")
	if err != nil {
		return err
	}

	return nil
}

func (p *ports) ForwardMany(ports []uint16) (uint16, error) {
	for _, port := range ports {
		err := p.Forward(port)
		if err != nil {
			continue
		}

		return port, nil
	}

	return 0, fmt.Errorf("could not forward any of the specified ports")
}

func NewService() Service {
	return &ports{}
}
