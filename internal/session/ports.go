package session

import (
	"fmt"

	"github.com/namvu9/bitsy/internal/errors"
	"gitlab.com/NebulousLabs/go-upnp"
)

func ForwardPorts(from, to uint16) (uint16, error) {
	// Discover UPnP-supporting routers
	d, err := upnp.Discover()
	if err != nil {
		return 0, errors.Wrap(err, errors.Network)
	}

	// forward a port
	for port := from; port <= to; port++ {
		err = d.Forward(port, "Bitsy BitTorrent client")
		if err != nil {
			continue
		}

		return port, err
	}

	err = fmt.Errorf("could not forward any of the specified ports")
	return 0, errors.Wrap(err, errors.Network)
}
