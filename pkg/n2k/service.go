// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package n2k provides the public API for NMEA 2000 message processing.
package n2k

import (
	"context"
	"time"

	"github.com/boatkit-io/n2k/internal/n2kinternal"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

// DefaultMessageQueueMaxAge is the default maximum live CAN message lag allowed
// before queued messages are dropped.
const DefaultMessageQueueMaxAge = n2kinternal.DefaultMessageQueueMaxAge

type serviceOptions struct {
	messageQueueMaxAge    time.Duration
	hasMessageQueueMaxAge bool
}

// ServiceOption configures an N2K service.
type ServiceOption func(*serviceOptions)

// WithMessageQueueMaxAge sets how stale queued live CAN messages may become before the
// service rejects new messages and discards queued stale messages.
func WithMessageQueueMaxAge(maxAge time.Duration) ServiceOption {
	return func(options *serviceOptions) {
		options.messageQueueMaxAge = maxAge
		options.hasMessageQueueMaxAge = true
	}
}

// N2kService provides the main public API for NMEA 2000 operations
type N2kService struct {
	impl *n2kinternal.N2kService
}

// NewN2kService creates a new N2K service with the specified endpoint
func NewN2kService(ep endpoint.Endpoint, log *logrus.Logger, opts ...ServiceOption) *N2kService {
	options := serviceOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	internalOptions := []n2kinternal.ServiceOption{}
	if options.hasMessageQueueMaxAge {
		internalOptions = append(internalOptions, n2kinternal.WithMessageQueueMaxAge(options.messageQueueMaxAge))
	}

	return &N2kService{
		impl: n2kinternal.NewN2kService(ep, log, internalOptions...),
	}
}

// Write sends a PGN struct to the bus
func (s *N2kService) Write(pgnStruct any) error {
	return s.impl.Write(pgnStruct)
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

// SubscribeToAllStructs subscribes to all PGN struct types and calls the callback when any message is received.
func (s *N2kService) SubscribeToAllStructs(callback any) (uint, error) {
	return s.impl.SubscribeToAllStructs(callback)
}

// SubscribeToStruct subscribes to a specific PGN struct type and calls the callback when messages of that type are received.
func (s *N2kService) SubscribeToStruct(t, callback any) (uint, error) {
	return s.impl.SubscribeToStruct(t, callback)
}

// Unsubscribe removes a subscription by its ID.
func (s *N2kService) Unsubscribe(id uint) error {
	return s.impl.Unsubscribe(id)
}

// SetReceivedCANFrameHook registers a callback invoked for each live CAN frame before decode.
func (s *N2kService) SetReceivedCANFrameHook(fn func(*can.Frame)) {
	s.impl.SetReceivedCANFrameHook(fn)
}

// MessageQueueLag returns the current age of the oldest live CAN message waiting
// in or moving through the serial handler path.
func (s *N2kService) MessageQueueLag() time.Duration {
	return s.impl.MessageQueueLag()
}

// MessageQueueMaxAge returns the configured maximum tolerated live CAN message queue lag.
func (s *N2kService) MessageQueueMaxAge() time.Duration {
	return s.impl.MessageQueueMaxAge()
}

// HandleReplayCANFrame feeds a captured CAN frame into the shared decode pipeline.
func (s *N2kService) HandleReplayCANFrame(frame *can.Frame) error {
	return s.impl.HandleReplayCANFrame(frame)
}
