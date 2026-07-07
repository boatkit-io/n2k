// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package idleendpoint provides an endpoint that never reads or writes CAN frames.
package idleendpoint

import (
	"context"
	"sync"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
)

// IdleEndpoint satisfies endpoint.Endpoint for disabled or unavailable N2K hardware.
type IdleEndpoint struct {
	mu      sync.Mutex
	handler endpoint.MessageHandler
	done    chan struct{}
	once    sync.Once
}

// New returns an endpoint that blocks in Run until the context is canceled or Close is called.
func New() *IdleEndpoint {
	return &IdleEndpoint{done: make(chan struct{})}
}

// Run waits until ctx is canceled or the endpoint is closed.
func (e *IdleEndpoint) Run(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.done:
		return nil
	}
}

// Close stops Run.
func (e *IdleEndpoint) Close() error {
	e.once.Do(func() {
		close(e.done)
	})
	return nil
}

// SetOutput stores the output handler to satisfy endpoint.Endpoint.
func (e *IdleEndpoint) SetOutput(handler endpoint.MessageHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handler = handler
}

// WriteFrame drops the frame.
func (e *IdleEndpoint) WriteFrame(_ can.Frame) {}
