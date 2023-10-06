// Package canendpoint contains the CANEndpoint struct described below
package canendpoint

import (
	"context"

	"github.com/boatkit-io/goatutils/pkg/canbus"
	"github.com/boatkit-io/goatutils/pkg/subscribableevent"
	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/brutella/can"
	"github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

// CANEndpoint is an endpoint backed by a live CAN interface, pulling down CAN frames
type CANEndpoint struct {
	log *logrus.Logger

	canbus     *canbus.Channel
	canbusOpts canbus.ChannelOptions

	frameReady subscribableevent.Event[func(adapter.Message)]
}

// NewCANEndpoint builds a new CANEndpoint for the given CAN interface name
func NewCANEndpoint(log *logrus.Logger, canInterfaceName string) *CANEndpoint {
	c := CANEndpoint{
		log: log,

		canbusOpts: canbus.ChannelOptions{
			CanInterfaceName: canInterfaceName,
			MessageHandler:   nil,
		},

		frameReady: subscribableevent.NewEvent[func(adapter.Message)](),
	}

	c.canbusOpts.MessageHandler = c.handleFrame

	return &c
}

// Run should, in theory, run the endpoint and block until completion/error, but the canbus implementation doesn't work like that
// right now unfortunately, so it just spawns in the background and keeps running until Shutdown kills it...
func (c *CANEndpoint) Run(ctx context.Context) error {
	// TODO: canbus.NewChannel actually doesn't block, it makes its own goroutine, we should fix this threading model someday
	cc, err := canbus.NewChannel(ctx, c.log, c.canbusOpts)
	if err != nil {
		c.log.WithError(err).Warn("n2k channel creation failed")
		return err
	}

	c.canbus = cc

	return nil
}

// SubscribeToFrameReady subscribes a callback function for whenever a message is ready
func (c *CANEndpoint) SubscribeToFrameReady(f func(adapter.Message)) subscribableevent.SubscriptionId {
	return c.frameReady.Subscribe(f)
}

// UnsubscribeFromFrameReady unsubscribes a previous subscription for ready frames
func (c *CANEndpoint) UnsubscribeFromFrameReady(t subscribableevent.SubscriptionId) error {
	return c.frameReady.Unsubscribe(t)
}

// Shutdown will stop the endpoint from processing further frames
func (c *CANEndpoint) Shutdown() error {
	if c.canbus != nil {
		var errs []error

		// TODO: Fix this context thing, it's not used/needed
		if err := c.canbus.Close(context.Background()); err != nil {
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

// handleFrame is an internal handler function for frames from the CAN handler
func (c *CANEndpoint) handleFrame(message can.Frame) {
	c.frameReady.Fire(canadapter.Frame(message))
}
