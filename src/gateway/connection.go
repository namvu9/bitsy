package gateway

import (
	"fmt"
	"net"
	"time"

	"github.com/namvu9/bitsy/src/errors"
	"github.com/rs/zerolog/log"
)

// Gateway handles incoming/outgoing connections and limits
// the number of active connections
//
// Satisfies the net.Conn interface
type Gateway struct {
	net.Conn
	openConns chan struct{}
	closeCh   chan struct{}
}

// Connections returns the number of currently open
// connections
func (gate *Gateway) Connections() int {
	return len(gate.openConns)
}

// Capacity returns the maximum number of open connections
// allowed by the gateway
func (gate *Gateway) Capacity() int {
	return cap(gate.openConns)
}

// CanDial reports whether the number of open connections is
// less than the number of maximum allowed connections
func (gate *Gateway) CanDial() bool {
	return gate.Connections() < gate.Capacity()
}

func (gate *Gateway) Dial(network string, addr string) (net.Conn, error) {
	var op errors.Op = "(*Gateway).Dial"

	gate.openConns <- struct{}{}
	conn, err := net.DialTimeout(network, addr, 10*time.Second)
	if err != nil {
		gate.closeCh <- struct{}{}
		return nil, errors.Wrap(err, op, errors.IO)
	}

	gate.Conn = conn
	return gate, nil
}

func (gate *Gateway) Close() error {
	var op errors.Op = "(*Gateway).Close"

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recovered from ", r)
			gate.closeCh <- struct{}{}
		}
	}()

	err := gate.Conn.Close()
	if err != nil {
		return errors.Wrap(err, op, errors.IO)
	}

	return err
}

func (gate *Gateway) Listen(network string, addr string) (net.Listener, error) {
	var op errors.Op = "(*Gateway).Listen"

	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, errors.Wrap(err, op, errors.Network)
	}

	log.Info().Str("op", op.String()).Msgf("Listening on %s://%s", network, addr)

	return &GatewayListener{
		Listener:  listener,
		openConns: gate.openConns,
		closeCh:   gate.closeCh,
	}, nil
}

func New(maxConns int) Gateway {
	var (
		openConns = make(chan struct{}, maxConns)
		closeCh   = make(chan struct{})
	)

	go func(closeCh, openCh chan struct{}) {
		for {
			<-closeCh
			if len(openConns) == 0 {
				log.Error().Msg("Tried to close Gateway connection with 0 open connections")
				continue
			}
			<-openConns
		}
	}(closeCh, openConns)

	return Gateway{
		openConns: openConns,
		closeCh:   closeCh,
	}
}

type GatewayListener struct {
	net.Listener
	openConns chan struct{}
	closeCh   chan struct{}
}

func (gl *GatewayListener) Accept() (net.Conn, error) {
	var op errors.Op = "(*GatewayListener).Accept"

	gl.openConns <- struct{}{}

	conn, err := gl.Listener.Accept()
	if err != nil {
		gl.closeCh <- struct{}{}
		return nil, errors.Wrap(err, op, errors.Network)
	}

	log.Printf("Accepted incoming connection: %s (%d/%d)", conn.RemoteAddr(), len(gl.openConns), cap(gl.openConns))

	return &Gateway{
		Conn:      conn,
		openConns: gl.openConns,
		closeCh:   gl.closeCh,
	}, nil
}
