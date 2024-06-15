// Package usbcanendpoint contains the USBCANEndpoint struct described below
package usbcanendpoint

import (
	"context"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/boatkit-io/tugboat/pkg/canbus"
	"github.com/brutella/can"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

// USBCANEndpoint is an endpoint backed by a USBCAN interface, pulling down CAN frames
type USBCANEndpoint struct {
	log *logrus.Logger

	channel canbus.Interface

	handler endpoint.MessageHandler
}

// NewUSBCANEndpoint builds a new SocketCANEndpoint for the given CAN interface name
func NewUSBCANEndpoint(log *logrus.Logger, serialPortName string) endpoint.Endpoint {
	c := USBCANEndpoint{
		log: log,
	}

	channelOpts := canbus.USBCANChannelOptions{
		SerialPortName: serialPortName,
		SerialBaudRate: 2000000,
		BitRate:        250000,
		FrameHandler:   c.frameReady,
	}

	c.channel = canbus.NewUSBCANChannel(log, channelOpts)

	return &c
}

// Run should, in theory, run the endpoint and block until completion/error, but the canbus implementation doesn't work like that
// right now unfortunately, so it just spawns in the background and keeps running until Shutdown kills it...
func (c *USBCANEndpoint) Run(ctx context.Context) error {
	return c.channel.Run(ctx)
}

// SetOutput subscribes a callback handler for whenever a message is ready
func (c *USBCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close will stop the endpoint from processing further frames
func (c *USBCANEndpoint) Close() error {
	if c.channel != nil {
		var errs []error

		if err := c.channel.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "closing n2k canbus channel"))
		}

		if len(errs) > 0 {
			err := errs[0]
			for i := 1; i < len(errs); i++ {
				err = errors.Wrap(err, errs[i].Error())
			}
			return err
		}
	}

	return nil
}

// frameReady is a helper to handle passing completed frames to the handler
func (c *USBCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		c.handler.HandleMessage(adapter.Message(&frame))
	}
}
