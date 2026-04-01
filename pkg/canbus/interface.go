package canbus

import (
	"context"

	"github.com/brutella/can"
)

// Interface is a basic interface for a CANbus implementation.
// Any CAN bus transport (USB-CAN dongle, SocketCAN, etc.) must implement these three methods
// to be usable as a CAN channel in the system.
//
// Implementations are responsible for:
//   - Opening and configuring the underlying hardware/OS interface
//   - Continuously reading incoming CAN frames and dispatching them to a handler callback
//   - Providing the ability to send outgoing CAN frames
//   - Cleanly shutting down when requested
type Interface interface {
	// Run opens the CAN bus channel and starts listening for incoming frames.
	// This method blocks until the context is cancelled or an unrecoverable error occurs.
	// It is the caller's responsibility to invoke Close() after Run() returns.
	Run(ctx context.Context) error

	// Close shuts down the CAN bus channel, releasing any underlying resources
	// (serial ports, sockets, etc.). It is safe to call Close() even if Run() was never called.
	Close() error

	// WriteFrame sends a single CAN frame out on the bus. The frame's ID and data payload
	// are encoded according to the transport's wire format (USB-CAN protocol or SocketCAN).
	WriteFrame(frame can.Frame) error
}
