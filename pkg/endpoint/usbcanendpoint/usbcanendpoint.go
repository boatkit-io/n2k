// Package usbcanendpoint provides a CAN bus endpoint backed by a USB-CAN Analyzer dongle.
//
// This endpoint connects to a USB-CAN Analyzer device via a serial port and reads CAN frames
// using the proprietary USB-CAN binary protocol (see the canbus package for protocol details).
// Received CAN frames are adapted into adapter.Message objects and dispatched to a registered
// MessageHandler.
//
// The USB-CAN Analyzer is a cheap, widely-available USB-to-CAN dongle that communicates
// at 2 Mbaud over a virtual serial port. The CAN bus bitrate is fixed at 250000 bps,
// which is the standard rate for NMEA 2000 marine networks.
package usbcanendpoint

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/open-ships/n2k/pkg/adapter"
	"github.com/open-ships/n2k/pkg/canbus"
	"github.com/open-ships/n2k/pkg/endpoint"
	"github.com/brutella/can"
)

// USBCANEndpoint is an endpoint backed by a USB-CAN Analyzer dongle, pulling CAN frames
// off the bus via a serial port connection.
type USBCANEndpoint struct {
	// log is the structured logger for diagnostic output.
	log *slog.Logger

	// channel is the underlying USB-CAN channel that handles serial port I/O and
	// the USB-CAN binary protocol framing. It satisfies the canbus.Interface.
	channel canbus.Interface

	// handler is the registered callback that receives completed adapter.Message objects.
	// It is set via SetOutput() and must be set before Run() is called.
	handler endpoint.MessageHandler
}

// NewUSBCANEndpoint creates a new USBCANEndpoint that will read CAN frames from the
// specified serial port.
//
// Parameters:
//   - log: structured logger for diagnostic output
//   - serialPortName: OS path to the serial device, e.g. "/dev/ttyUSB0" or "/dev/cu.usbserial-..."
//
// The serial baud rate is hardcoded to 2000000 (2 Mbaud), which is the standard rate for
// the USB-CAN Analyzer device. The CAN bus bitrate is hardcoded to 250000 bps (NMEA 2000 standard).
//
// Returns an endpoint.Endpoint interface so the caller can use it generically.
func NewUSBCANEndpoint(log *slog.Logger, serialPortName string) endpoint.Endpoint {
	c := USBCANEndpoint{
		log: log,
	}

	channelOpts := canbus.USBCANChannelOptions{
		SerialPortName: serialPortName,
		SerialBaudRate: 2000000, // 2 Mbaud -- standard for USB-CAN Analyzer serial link
		BitRate:        250000,  // 250 kbps -- NMEA 2000 CAN bus bitrate
		FrameHandler:   c.frameReady,
	}

	c.channel = canbus.NewUSBCANChannel(log, channelOpts)

	return &c
}

// Run starts the USB-CAN endpoint by opening the serial port, configuring the device,
// and entering a blocking read loop. This method delegates to the underlying USBCANChannel.Run()
// and blocks until an error occurs or the serial port is closed.
//
// Note: The current canbus implementation does not support context cancellation --
// it blocks on serial reads and only exits on I/O errors. The caller should use Close()
// to terminate the read loop.
func (c *USBCANEndpoint) Run(ctx context.Context) error {
	return c.channel.Run(ctx)
}

// SetOutput registers the MessageHandler callback that will receive completed messages.
// This must be called before Run() so that incoming CAN frames are dispatched to the handler.
func (c *USBCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close stops the endpoint by closing the underlying USB-CAN channel (which closes the serial port).
// It is safe to call Close() even if the channel was never opened.
func (c *USBCANEndpoint) Close() error {
	if c.channel != nil {
		if err := c.channel.Close(); err != nil {
			return fmt.Errorf("closing n2k canbus channel: %w", err)
		}
	}

	return nil
}

// frameReady is the internal callback registered with the USB-CAN channel. It is called
// for each successfully parsed CAN frame. It wraps the raw can.Frame as an adapter.Message
// and forwards it to the registered MessageHandler.
//
// If no handler has been registered (SetOutput was not called), the frame is silently dropped.
func (c *USBCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		// adapter.Message wraps a *can.Frame pointer, providing a common message type
		// that can be used by upstream processors regardless of the CAN transport backend.
		c.handler.HandleMessage(adapter.Message(&frame))
	}
}
