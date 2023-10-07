// Package endpoint declares an interface. Create a type satisfying it to support a new gateway or log file format.
package endpoint

import (
	"context"

	"github.com/boatkit-io/n2k/pkg/adapter"
)

// Endpoint declares the interface for endpoints.
type Endpoint interface {
	Run(ctx context.Context) error
	SetOutput(MessageHandler)
}

// MessageHandler is an interface for the handler of an Endpoint that takes a finished Message object
type MessageHandler interface {
	HandleMessage(adapter.Message)
}
