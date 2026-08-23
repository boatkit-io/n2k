// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package endpoint declares an interface. Create a type satisfying it
// to support a new gateway or log file format.
package endpoint

import (
	"context"
	"time"

	"github.com/brutella/can"
)

// Message is a generic type for messages passed between and endpoint
// and an adapter.
type Message interface {
}

// PGNMessage is one complete, reassembled NMEA 2000 message. Gateways such as
// the Actisense NGT-1 transport this representation instead of individual CAN
// frames.
type PGNMessage struct {
	Timestamp   time.Time
	Priority    uint8
	PGN         uint32
	Source      uint8
	Destination uint8
	Data        []byte
}

// PGNWriter is implemented by endpoints that can transmit complete NMEA 2000
// messages without first fragmenting them into CAN frames.
type PGNWriter interface {
	WritePGN(PGNMessage) error
}

// PGNWriterCapability lets an endpoint type that supports multiple transport
// variants select complete-PGN writes only for the variants that need them.
// An endpoint implementing PGNWriter without this interface is assumed to
// support complete-PGN writes unconditionally.
type PGNWriterCapability interface {
	SupportsPGNWrites() bool
}

// ExternalAddressState is the source address claimed by a gateway on behalf of
// host software. The host may publish application PGNs through that address,
// but must not run a second address-claim state machine for it.
type ExternalAddressState struct {
	Address uint8
	Claimed bool
}

// ExternalAddressProvider is implemented by gateways such as the Actisense
// NGT-1 that own NMEA 2000 address claiming for their host software.
type ExternalAddressProvider interface {
	ExternalAddressState() ExternalAddressState
	SetExternalAddressHandler(func(ExternalAddressState))
}

// Endpoint declares the interface for endpoints.
type Endpoint interface {
	// Start synchronously prepares the endpoint for reads and writes. It returns
	// once startup has either completed successfully or failed.
	Start(ctx context.Context) error
	Run(ctx context.Context) error
	Close() error
	SetOutput(MessageHandler)
	WriteFrame(can.Frame)
}

// OutboundLagReporter is implemented by endpoints that can report outbound
// queue/send latency.
type OutboundLagReporter interface {
	OutboundQueueLag() time.Duration
}

// MessageHandler is an interface for the handler of an Endpoint that takes a finished Message object
type MessageHandler interface {
	HandleMessage(message Message)
}
