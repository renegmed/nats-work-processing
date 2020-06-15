package main

import (
	"fmt"
	"sync"

	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nuid"
)

/*
	NATS connector
*/

type Connector struct {
	cmu  sync.Mutex
	id   string
	nc   *nats.Conn
	kind string
}

func NewConnector(kind string) *Connector {
	id := nuid.Next()
	return &Connector{
		id:   id,
		kind: kind,
	}
}

// SetupConnectionToNATS connects to NATS
func (c *Connector) SetupConnectionToNATS(servers string, options ...nats.Option) error {
	options = append(options, nats.Name(c.Name()))
	c.cmu.Lock()
	defer c.cmu.Unlock()

	nc, err := nats.Connect(servers, options...)
	if err != nil {
		return err
	}
	c.nc = nc

	return nil
}

// NATS returns the current NATS connection.
func (c *Connector) NATS() *nats.Conn {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	return c.nc
}

// ID returns the ID from the component.
func (c *Connector) ID() string {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	return c.id
}

// Name is the label used to identify the NATS connection.
func (c *Connector) Name() string {
	c.cmu.Lock()
	defer c.cmu.Unlock()
	return fmt.Sprintf("%s:%s", c.kind, c.id)
}

// Shutdown makes the component go away.
func (c *Connector) Shutdown() error {
	c.NATS().Close()
	return nil
}
