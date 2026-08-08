// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package usbcanendpoint contains the USBCANEndpoint struct described
// below
package usbcanendpoint

import (
	"context"
	stderrors "errors"
	"sync"
	"sync/atomic"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/boatkit-io/tugboat/pkg/canbus"
	"github.com/brutella/can"
	pkgerrors "github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

// USBCANEndpoint is an endpoint backed by a USBCAN interface, pulling down CAN frames
type USBCANEndpoint struct {
	log *logrus.Logger

	channel canbus.Interface

	handler endpoint.MessageHandler
	closed  atomic.Bool
	runMu   sync.Mutex
	running bool
}

// NewUSBCANEndpoint builds a new SocketCANEndpoint for the given CAN interface name
func NewUSBCANEndpoint(log *logrus.Logger, serialPortName string) endpoint.Endpoint {
	c := USBCANEndpoint{
		log: log,
	}

	channelOpts := canbus.USBCANChannelOptions{
		SerialPortName: serialPortName,
		SerialBaudRate: 2000000,
		BitRate:        250000,
		FrameHandler:   c.frameReady,
	}

	c.channel = canbus.NewUSBCANChannel(log, channelOpts)

	return &c
}

// Start synchronously opens and configures the serial CAN channel.
func (c *USBCANEndpoint) Start(ctx context.Context) error {
	if c.closed.Load() {
		return stderrors.New("USBCAN endpoint is closed")
	}
	if err := c.channel.Start(ctx); err != nil {
		return err
	}
	if c.closed.Load() {
		_ = c.channel.Close()
		return stderrors.New("USBCAN endpoint is closed")
	}
	return nil
}

// Run processes serial CAN frames until completion or error.
func (c *USBCANEndpoint) Run(ctx context.Context) error {
	c.runMu.Lock()
	if c.running {
		c.runMu.Unlock()
		return stderrors.New("USBCAN endpoint is already running")
	}
	if c.closed.Load() {
		c.runMu.Unlock()
		return stderrors.New("USBCAN endpoint is closed")
	}
	c.running = true
	c.runMu.Unlock()
	defer func() {
		c.runMu.Lock()
		c.running = false
		c.runMu.Unlock()
	}()
	if err := c.Start(ctx); err != nil {
		return err
	}
	return c.channel.Run(ctx)
}

// SetOutput subscribes a callback handler for whenever a message is ready
func (c *USBCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close will stop the endpoint from processing further frames
func (c *USBCANEndpoint) Close() error {
	c.closed.Store(true)
	if c.channel != nil {
		var errs []error

		if err := c.channel.Close(); err != nil {
			errs = append(errs, pkgerrors.Wrap(err, "closing n2k canbus channel"))
		}

		if len(errs) > 0 {
			err := errs[0]
			for i := 1; i < len(errs); i++ {
				err = pkgerrors.Wrap(err, errs[i].Error())
			}
			return err
		}
	}

	return nil
}

// WriteFrame sends a CAN frame to the USBCAN interface
func (c *USBCANEndpoint) WriteFrame(frame can.Frame) {
	if c.channel != nil && !c.closed.Load() {
		if err := c.channel.WriteFrame(frame); err != nil {
			c.log.WithError(err).Error("failed to send frame to USBCAN interface")
		}
	}
}

// frameReady is a helper to handle passing completed frames to the handler
func (c *USBCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		c.handler.HandleMessage(endpoint.Message(&frame))
	}
}
