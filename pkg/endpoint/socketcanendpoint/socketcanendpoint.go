// Package socketcanendpoint contains the SocketCANEndpoint struct described below
package socketcanendpoint

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-ships/n2k/pkg/adapter"
	"github.com/open-ships/n2k/pkg/canbus"
	"github.com/open-ships/n2k/pkg/endpoint"
	"github.com/brutella/can"
)

// SocketCANEndpoint is an endpoint backed by a live SocketCAN interface, pulling down CAN frames
type SocketCANEndpoint struct {
	log *slog.Logger

	channel canbus.Interface

	handler endpoint.MessageHandler
}

// NewSocketCANEndpoint builds a new SocketCANEndpoint for the given CAN interface name
func NewSocketCANEndpoint(log *slog.Logger, canInterfaceName string) endpoint.Endpoint {
	c := SocketCANEndpoint{
		log: log,
	}

	channelOpts := canbus.SocketCANChannelOptions{
		InterfaceName:  canInterfaceName,
		BitRate:        250000,
		MessageHandler: c.frameReady,
	}

	c.channel = canbus.NewSocketCANChannel(log, channelOpts)

	return &c
}

// Run should, in theory, run the endpoint and block until completion/error, but the canbus implementation doesn't work like that
// right now unfortunately, so it just spawns in the background and keeps running until Shutdown kills it...
func (c *SocketCANEndpoint) Run(ctx context.Context) error {
	return c.channel.Run(ctx)
}

// SetOutput subscribes a callback handler for whenever a message is ready
func (c *SocketCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close will stop the endpoint from processing further frames
func (c *SocketCANEndpoint) Close() error {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			return fmt.Errorf("closing n2k canbus channel: %w", err)
		}
	}

	return nil
}

// frameReady is a helper to handle passing completed frames to the handler
func (c *SocketCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		c.handler.HandleMessage(adapter.Message(&frame))
	}
}
