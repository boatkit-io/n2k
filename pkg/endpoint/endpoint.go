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

	"github.com/brutella/can"
)

// Message is a generic type for messages passed between and endpoint
// and an adapter.
type Message interface {
}

// Endpoint declares the interface for endpoints.
type Endpoint interface {
	Run(ctx context.Context) error
	Close() error
	SetOutput(MessageHandler)
	WriteFrame(can.Frame)
}

// MessageHandler is an interface for the handler of an Endpoint that takes a finished Message object
type MessageHandler interface {
	HandleMessage(message Message)
}
