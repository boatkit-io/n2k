// Package socketcanendpoint provides a CAN bus endpoint backed by a Linux SocketCAN interface.
//
// This endpoint connects to a SocketCAN network interface (e.g., "can0") and reads CAN frames
// using the native Linux kernel CAN socket API via the brutella/can library. Received CAN frames
// are adapted into adapter.Message objects and dispatched to a registered MessageHandler.
//
// SocketCAN interfaces are typically provided by hardware CAN controllers connected via SPI
// (e.g., MCP2515 chips on Raspberry Pi HATs) or USB (e.g., PEAK PCAN-USB). The kernel exposes
// them as regular network interfaces that can be managed with standard tools like `ip link`.
//
// The CAN bus bitrate is fixed at 250000 bps, which is the standard rate for NMEA 2000 networks.
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

// SocketCANEndpoint is an endpoint backed by a live Linux SocketCAN interface, pulling CAN frames
// off the bus using the kernel's native CAN socket API.
type SocketCANEndpoint struct {
	// log is the structured logger for diagnostic output.
	log *slog.Logger

	// channel is the underlying SocketCAN channel that handles interface configuration via netlink
	// and CAN socket I/O via the brutella/can library. It satisfies the canbus.Interface.
	channel canbus.Interface

	// handler is the registered callback that receives completed adapter.Message objects.
	// It is set via SetOutput() and must be set before Run() is called.
	handler endpoint.MessageHandler
}

// NewSocketCANEndpoint creates a new SocketCANEndpoint that will read CAN frames from the
// specified Linux CAN network interface.
//
// Parameters:
//   - log: structured logger for diagnostic output
//   - canInterfaceName: the Linux network interface name, e.g. "can0" or "can1"
//
// The CAN bus bitrate is hardcoded to 250000 bps (NMEA 2000 standard). The SocketCAN channel
// will use netlink and the `ip` command to bring the interface up with the correct bitrate
// if needed.
//
// Returns an endpoint.Endpoint interface so the caller can use it generically.
func NewSocketCANEndpoint(log *slog.Logger, canInterfaceName string) endpoint.Endpoint {
	c := SocketCANEndpoint{
		log: log,
	}

	channelOpts := canbus.SocketCANChannelOptions{
		InterfaceName:  canInterfaceName,
		BitRate:        250000, // 250 kbps -- NMEA 2000 CAN bus bitrate
		MessageHandler: c.frameReady,
	}

	c.channel = canbus.NewSocketCANChannel(log, channelOpts)

	return &c
}

// Run starts the SocketCAN endpoint by configuring the network interface (via netlink/ip commands),
// opening a CAN socket, and entering a blocking read loop. This method delegates to the underlying
// SocketCANChannel.Run() and blocks until an error occurs or the socket is closed.
//
// Note: The current canbus implementation does not cleanly support context cancellation --
// ConnectAndPublish blocks on socket reads. The caller should use Close() to terminate the loop.
func (c *SocketCANEndpoint) Run(ctx context.Context) error {
	return c.channel.Run(ctx)
}

// SetOutput registers the MessageHandler callback that will receive completed messages.
// This must be called before Run() so that incoming CAN frames are dispatched to the handler.
func (c *SocketCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close stops the endpoint by closing the underlying SocketCAN channel (which disconnects the
// CAN socket and unsubscribes from frame events). It is safe to call Close() even if the
// channel was never opened.
func (c *SocketCANEndpoint) Close() error {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			return fmt.Errorf("closing n2k canbus channel: %w", err)
		}
	}

	return nil
}

// frameReady is the internal callback registered with the SocketCAN channel. It is called
// for each CAN frame received on the bus. It wraps the raw can.Frame as an adapter.Message
// and forwards it to the registered MessageHandler.
//
// If no handler has been registered (SetOutput was not called), the frame is silently dropped.
func (c *SocketCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		// adapter.Message wraps a *can.Frame pointer, providing a common message type
		// that can be used by upstream processors regardless of the CAN transport backend.
		c.handler.HandleMessage(adapter.Message(&frame))
	}
}
