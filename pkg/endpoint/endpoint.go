// Package endpoint declares the interface for CAN bus data sources (endpoints).
//
// An endpoint abstracts the source of CAN bus messages, allowing the system to work with
// different hardware backends (USB-CAN dongles, SocketCAN interfaces) or software sources
// (log file replay) through a uniform API.
//
// The data flow is: Endpoint -> reads raw CAN frames -> assembles/adapts them -> dispatches
// completed Message objects to a registered MessageHandler.
//
// To add support for a new gateway device or log file format, create a type that satisfies
// the Endpoint interface in its own sub-package (see usbcanendpoint and socketcanendpoint
// for examples).
package endpoint

import (
	"context"

	"github.com/open-ships/n2k/pkg/adapter"
)

// Endpoint declares the interface that all CAN bus data sources must implement.
// Implementations are responsible for:
//   - Opening and configuring the underlying transport (serial port, socket, file, etc.)
//   - Reading raw CAN frames from the transport
//   - Converting those frames into adapter.Message objects
//   - Dispatching messages to the registered MessageHandler
type Endpoint interface {
	// Run starts the endpoint and blocks until an error occurs or the context is cancelled.
	// Before calling Run, the caller should register a MessageHandler via SetOutput.
	Run(ctx context.Context) error

	// Close shuts down the endpoint, releasing any underlying resources.
	// It is safe to call Close() even if Run() was never called.
	Close() error

	// SetOutput registers the MessageHandler that will receive completed messages.
	// This must be called before Run() so that incoming CAN frames have somewhere to go.
	SetOutput(MessageHandler)
}

// MessageHandler is an interface for receiving completed Message objects from an Endpoint.
// The handler is called synchronously on the endpoint's read goroutine, so implementations
// should avoid blocking for extended periods to prevent dropping incoming CAN frames.
type MessageHandler interface {
	// HandleMessage is called for each completed message received by the endpoint.
	// The adapter.Message wraps a can.Frame with the CAN ID and up to 8 bytes of payload.
	HandleMessage(adapter.Message)
}
