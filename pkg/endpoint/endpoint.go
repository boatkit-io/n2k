// Package endpoint declares an interface. Create a type satisfying it to support a new gateway or log file format.
package endpoint

import (
	"context"

	"github.com/boatkit-io/goatutils/pkg/subscribableevent"
	"github.com/boatkit-io/n2k/pkg/adapter"
)

// Endpoint declares the interface for endpoints.
type Endpoint interface {
	Run(ctx context.Context) error
	SubscribeToFrameReady(f func(adapter.Message)) subscribableevent.SubscriptionId
	UnsubscribeFromFrameReady(t subscribableevent.SubscriptionId)
}
