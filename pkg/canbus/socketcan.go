package canbus

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"

	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/vishvananda/netlink"
)

// SocketCANChannelOptions contains the configuration required to open and operate a SocketCAN channel.
//
// SocketCAN is the native Linux kernel CAN bus interface. It represents CAN controllers as
// regular network interfaces (e.g., "can0", "can1") that can be configured using standard
// networking tools (ip link) and accessed through socket APIs.
type SocketCANChannelOptions struct {
	// InterfaceName is the Linux network interface name for the CAN controller, e.g. "can0".
	// This corresponds to the interface shown by `ip link show` and is typically assigned
	// by the kernel when a CAN controller driver (such as MCP2515 over SPI) is loaded.
	InterfaceName string

	// BitRate is the CAN bus bitrate in bits per second. For NMEA 2000 networks, this is
	// typically 250000 (250 kbps). The bitrate is configured via netlink when bringing
	// the interface up, using the equivalent of `ip link set canX up type can bitrate 250000`.
	BitRate int

	// ForceBounceInterface, when true, forces the CAN interface to be brought down and back up
	// even if it is already in the OperUp state. This can be useful to reset the CAN controller
	// after errors or to ensure a clean state on startup.
	ForceBounceInterface bool

	// MessageHandler is the callback function invoked for each CAN frame received on the bus.
	// The handler receives a can.Frame containing the CAN ID and up to 8 bytes of payload.
	MessageHandler can.HandlerFunc
}

// SocketCANChannel represents a single SocketCAN-based canbus channel for sending/receiving CAN frames.
// It uses the Linux netlink API to configure the CAN interface (bitrate, up/down state) and the
// brutella/can library for the actual CAN socket I/O.
type SocketCANChannel struct {
	// options holds the user-provided configuration (interface name, bitrate, handler, etc.)
	options SocketCANChannelOptions

	// bus is the underlying brutella/can bus object that manages the CAN socket connection.
	// It handles subscribing to incoming frames and publishing outgoing frames.
	bus *can.Bus

	// busHandler wraps the user's MessageHandler callback into a can.Handler interface
	// so it can be registered with the brutella/can bus subscription system.
	busHandler can.Handler

	// log is the structured logger for diagnostic output about interface state changes and errors.
	log *slog.Logger
}

// NewSocketCANChannel creates and returns a new SocketCANChannel configured with the given options.
// The channel is not opened until Run() is called.
//
// Parameters:
//   - log: structured logger for diagnostic output
//   - options: required configuration including interface name, CAN bitrate, and message handler
//
// Returns a *SocketCANChannel (concrete type, not Interface) because SocketCAN-specific
// callers may need access to SocketCAN-specific functionality.
func NewSocketCANChannel(log *slog.Logger, options SocketCANChannelOptions) *SocketCANChannel {
	c := SocketCANChannel{
		options: options,
		log:     log,
	}

	return &c
}

// Run opens the SocketCAN interface and starts listening for CAN frames. This method performs
// several steps using the Linux netlink API to ensure the interface is properly configured:
//
//  1. Looks up the network interface by name using netlink and verifies it is a CAN type interface.
//  2. If the interface is already up, checks whether the bitrate matches the desired configuration.
//     If the bitrate is wrong or ForceBounceInterface is set, the interface is brought down first
//     so it can be reconfigured (CAN bitrate can only be set while the interface is down).
//  3. If the interface is down, brings it up with the correct bitrate using `ip link set ... up type can bitrate ...`.
//     Note: This uses exec.Command("ip", ...) rather than pure netlink calls because the netlink
//     library's CAN bitrate setting was unreliable at the time of writing.
//  4. Opens a CAN socket via the brutella/can library and subscribes to incoming frames.
//  5. Blocks on ConnectAndPublish(), which reads frames from the socket and dispatches them
//     to the subscribed handler.
//
// This method blocks until an error occurs or the connection is closed.
func (c *SocketCANChannel) Run(ctx context.Context) error {
	// Referencing https://github.com/angelodlfrtr/go-can/blob/master/transports/socketcan.go

	// Use the Linux netlink API to look up the CAN network interface by name.
	// netlink is the kernel's interface for network configuration (similar to ioctl but more modern).
	link, err := netlink.LinkByName(c.options.InterfaceName)
	if err != nil {
		return fmt.Errorf("no link found for %v: %v", c.options.InterfaceName, err)
	}

	// Verify that the interface is actually a CAN interface, not an ethernet or other type.
	if link.Type() != "can" {
		return fmt.Errorf("invalid linktype %q", link.Type())
	}

	// Type-assert to *netlink.Can to access CAN-specific attributes like BitRate.
	canLink := link.(*netlink.Can)

	// If the interface is already up, we may need to bounce it (bring down then up) in two cases:
	// 1. The current bitrate doesn't match what we want (CAN bitrate can only be changed while down)
	// 2. The caller explicitly requested a bounce via ForceBounceInterface
	if canLink.Attrs().OperState == netlink.OperUp {
		bounce := false
		if canLink.BitRate != uint32(c.options.BitRate) {
			c.log.Info("Channel currently has wrong bitrate, bringing down", "bitRate", canLink.BitRate)
			bounce = true
		} else if c.options.ForceBounceInterface {
			c.log.Info("Bouncing channel")
			bounce = true
		}

		if bounce {
			// Bring the interface down using `ip link set <name> down`.
			// We use exec.Command here rather than netlink.LinkSetDown because the ip command
			// provides better error reporting and handles edge cases more reliably.
			cmd := exec.CommandContext(ctx, "ip", "link", "set", c.options.InterfaceName, "down")
			if output, err := cmd.Output(); err != nil {
				attrs := []any{"cmd", strings.Join(cmd.Args, " "), "output", string(output)}
				// If the command failed with a non-zero exit code, capture stderr for diagnostics.
				if errCast, worked := err.(*exec.ExitError); worked {
					attrs = append(attrs, "stderr", string(errCast.Stderr))
				}
				c.log.Error("Ip link set down failed", attrs...)
				return err
			}

			// Re-fetch the link info after bringing it down, since the state has changed.
			link, err = netlink.LinkByName(c.options.InterfaceName)
			if err != nil {
				return fmt.Errorf("no link found for %v: %v", c.options.InterfaceName, err)
			}

			canLink = link.(*netlink.Can)
		}
	}

	// If the interface is currently down (either it was already down, or we just bounced it),
	// bring it up with the desired CAN bitrate.
	if canLink.Attrs().OperState == netlink.OperDown {
		c.log.Info("Link is down, bringing up link", "canName", c.options.InterfaceName, "bitRate", c.options.BitRate)

		// Equivalent to: ip link set can1 up type can bitrate 250000
		// The "type can bitrate ..." part configures CAN-specific parameters before the interface goes up.
		cmd := exec.CommandContext(ctx, "ip", "link", "set", c.options.InterfaceName, "up", "type", "can", "bitrate",
			strconv.Itoa(int(c.options.BitRate)))
		if output, err := cmd.Output(); err != nil {
			attrs := []any{"cmd", strings.Join(cmd.Args, " "), "output", string(output)}
			if errCast, worked := err.(*exec.ExitError); worked {
				attrs = append(attrs, "stderr", string(errCast.Stderr))
			}
			c.log.Error("Ip link set up failed", attrs...)
			return err
		}
	}

	// Open a CAN socket using the brutella/can library. This creates a raw CAN socket
	// bound to the specified network interface and provides a higher-level pub/sub API.
	bus, err := can.NewBusForInterfaceWithName(c.options.InterfaceName)
	if err != nil {
		return err
	}

	c.bus = bus

	// Wrap the user's handler callback and subscribe it to receive all incoming CAN frames.
	c.busHandler = can.NewHandler(c.options.MessageHandler)
	c.bus.Subscribe(c.busHandler)

	c.log.Info("Opened SocketCAN and listening", "interfaceName", c.options.InterfaceName)

	// ConnectAndPublish opens the socket and enters a blocking read loop.
	// Each received CAN frame is published to all subscribed handlers.
	// This call blocks until the socket is closed or an error occurs.
	return bus.ConnectAndPublish()
}

// Close shuts down the SocketCAN channel by unsubscribing the frame handler and disconnecting
// the underlying CAN socket. It is safe to call Close() if the bus was never opened (bus is nil).
func (c *SocketCANChannel) Close() error {
	if c.bus == nil {
		return nil
	}

	// Unsubscribe first to stop receiving frames, then disconnect the socket.
	c.bus.Unsubscribe(c.busHandler)
	if err := c.bus.Disconnect(); err != nil {
		return errors.Wrap(err, "close underlying bus connection")
	}

	return nil
}

// WriteFrame sends a single CAN frame out on the SocketCAN bus.
// The brutella/can library handles encoding the frame into the Linux SocketCAN wire format
// and writing it to the raw CAN socket.
func (c *SocketCANChannel) WriteFrame(frame can.Frame) error {
	return c.bus.Publish(frame)
}
