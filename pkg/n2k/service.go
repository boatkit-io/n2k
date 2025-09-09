// Package n2k provides the public API for NMEA 2000 message processing.
package n2k

import (
	"context"

	"github.com/boatkit-io/n2k/internal/n2kInternal"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/pkg/endpoint"
)

// N2kService provides the main public API for NMEA 2000 operations
type N2kService struct {
	impl *n2kInternal.N2kService
}

// NewN2kService creates a new N2K service with the specified endpoint
func NewN2kService(ep endpoint.Endpoint) *N2kService {
	return &N2kService{
		impl: n2kInternal.NewN2kService(ep),
	}
}

// Write sends a PGN struct to the bus
func (s *N2kService) Write(pgnStruct any) error {
	return s.impl.Write(pgnStruct.(pgn.PgnStruct))
}

// Start begins processing messages from the endpoint
func (s *N2kService) Start(ctx context.Context) error {
	return s.impl.Start(ctx)
}

// Stop stops processing messages
func (s *N2kService) Stop() error {
	return s.impl.Stop()
}

// UpdateEndpoint updates the endpoint used by the service
func (s *N2kService) UpdateEndpoint(ep endpoint.Endpoint) error {
	return s.impl.UpdateEndpoint(ep)
}
