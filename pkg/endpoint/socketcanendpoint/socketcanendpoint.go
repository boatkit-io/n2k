// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package socketcanendpoint contains the SocketCANEndpoint struct described below
package socketcanendpoint

import (
	"context"
	stderrors "errors"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/tugboat/pkg/canbus"
	"github.com/brutella/can"
	pkgerrors "github.com/pkg/errors"

	"github.com/sirupsen/logrus"
)

const (
	socketCANOutboundQueueSize = 2048
	socketCANOutboundLagTTL    = 5 * time.Second
)

// SocketCANEndpoint is an endpoint backed by a live SocketCAN interface, pulling down CAN frames
type SocketCANEndpoint struct {
	log *logrus.Logger

	channel canbus.Interface

	handler endpoint.MessageHandler

	outboundOnce sync.Once
	outboundHigh chan outboundSocketCANFrame
	outboundLow  chan outboundSocketCANFrame

	outboundWriterMutex  sync.Mutex
	outboundWriterCancel context.CancelFunc
	outboundWriterDone   chan struct{}

	outboundLagNano        atomic.Int64
	outboundLagUpdatedNano atomic.Int64
}

type outboundSocketCANFrame struct {
	frame      can.Frame
	enqueuedAt time.Time
	attempt    int
}

type socketCANRetryPolicy struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	requeueDelay bool
}

// NewSocketCANEndpoint builds a new SocketCANEndpoint for the given CAN interface name
func NewSocketCANEndpoint(log *logrus.Logger, canInterfaceName string) endpoint.Endpoint {
	c := SocketCANEndpoint{
		log: log,
	}

	// vcan interfaces are now supported with the modified tugboat package

	channelOpts := canbus.SocketCANChannelOptions{
		InterfaceName:  canInterfaceName,
		BitRate:        250000,
		MessageHandler: c.frameReady,
	}

	c.channel = canbus.NewSocketCANChannel(log, channelOpts)
	c.initOutboundQueues()

	return &c
}

// Run should, in theory, run the endpoint and block until completion/error, but the canbus implementation doesn't work like that
// right now unfortunately, so it just spawns in the background and keeps running until Shutdown kills it...
func (c *SocketCANEndpoint) Run(ctx context.Context) error {
	c.initOutboundQueues()
	writerCtx, cancelWriter := context.WithCancel(ctx)
	writerDone := make(chan struct{})
	c.outboundWriterMutex.Lock()
	if c.outboundWriterCancel != nil {
		c.outboundWriterCancel()
	}
	c.outboundWriterCancel = cancelWriter
	c.outboundWriterDone = writerDone
	c.outboundWriterMutex.Unlock()

	go func() {
		defer func() {
			close(writerDone)
			c.outboundWriterMutex.Lock()
			if c.outboundWriterDone == writerDone {
				c.outboundWriterCancel = nil
				c.outboundWriterDone = nil
			}
			c.outboundWriterMutex.Unlock()
		}()
		c.runOutboundWriter(writerCtx)
	}()

	err := c.channel.Run(ctx)
	if err != nil {
		c.stopOutboundWriter()
	}
	return err
}

// SetOutput subscribes a callback handler for whenever a message is ready
func (c *SocketCANEndpoint) SetOutput(mh endpoint.MessageHandler) {
	c.handler = mh
}

// Close will stop the endpoint from processing further frames
func (c *SocketCANEndpoint) Close() error {
	if c.channel != nil {
		var errs []error

		if err := c.channel.Close(); err != nil {
			errs = append(errs, pkgerrors.Wrap(err, "closing n2k canbus channel"))
		}
		c.stopOutboundWriter()

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

func (c *SocketCANEndpoint) stopOutboundWriter() {
	c.outboundWriterMutex.Lock()
	cancel := c.outboundWriterCancel
	done := c.outboundWriterDone
	c.outboundWriterMutex.Unlock()

	if cancel == nil || done == nil {
		return
	}
	cancel()
	<-done
}

// WriteFrame sends a CAN frame to the SocketCAN interface
func (c *SocketCANEndpoint) WriteFrame(frame can.Frame) {
	if c.channel == nil {
		return
	}
	c.initOutboundQueues()
	item := outboundSocketCANFrame{
		frame:      frame,
		enqueuedAt: time.Now(),
	}

	if isLowPrioritySocketCANFrame(frame) {
		select {
		case c.outboundLow <- item:
		default:
			c.log.Warn("dropping low-priority SocketCAN frame because the outbound queue is full")
		}
		return
	}

	select {
	case c.outboundHigh <- item:
	default:
		c.log.Warn("SocketCAN outbound queue is full; applying backpressure")
		c.outboundHigh <- item
	}
}

func (c *SocketCANEndpoint) initOutboundQueues() {
	c.outboundOnce.Do(func() {
		c.outboundHigh = make(chan outboundSocketCANFrame, socketCANOutboundQueueSize)
		c.outboundLow = make(chan outboundSocketCANFrame, socketCANOutboundQueueSize)
	})
}

func (c *SocketCANEndpoint) runOutboundWriter(ctx context.Context) {
	c.initOutboundQueues()
	for {
		select {
		case item := <-c.outboundHigh:
			c.writeQueuedFrame(ctx, item)
			continue
		default:
		}

		select {
		case item := <-c.outboundHigh:
			c.writeQueuedFrame(ctx, item)
		case item := <-c.outboundLow:
			c.writeQueuedFrame(ctx, item)
		case <-ctx.Done():
			return
		}
	}
}

func (c *SocketCANEndpoint) writeQueuedFrame(ctx context.Context, item outboundSocketCANFrame) {
	policy := retryPolicyForSocketCANFrame(item.frame)
	if item.enqueuedAt.IsZero() {
		item.enqueuedAt = time.Now()
	}

	for {
		err := c.channel.WriteFrame(item.frame)
		if err == nil {
			c.recordOutboundLag(item)
			return
		}
		if !isSocketCANTxBufferFull(err) {
			c.log.WithError(err).Error("failed to send frame to SocketCAN interface")
			return
		}
		c.recordOutboundLag(item)

		delay := policy.delay(item.attempt)
		if policy.requeueDelay {
			item.attempt++
			c.requeueLowPriorityFrame(ctx, item, delay)
			return
		}

		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
		item.attempt++
	}
}

func retryPolicyForSocketCANFrame(frame can.Frame) socketCANRetryPolicy {
	if isLowPrioritySocketCANFrame(frame) {
		return socketCANRetryPolicy{
			initialDelay: 250 * time.Millisecond,
			maxDelay:     2 * time.Second,
			requeueDelay: true,
		}
	}
	return socketCANRetryPolicy{
		initialDelay: time.Millisecond,
		maxDelay:     50 * time.Millisecond,
	}
}

func (p socketCANRetryPolicy) delay(attempt int) time.Duration {
	delay := p.initialDelay
	for range attempt {
		delay *= 2
		if delay >= p.maxDelay {
			return p.maxDelay
		}
	}
	return delay
}

func (c *SocketCANEndpoint) requeueLowPriorityFrame(ctx context.Context, item outboundSocketCANFrame, delay time.Duration) {
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}

		select {
		case c.outboundLow <- item:
		default:
			c.log.Warn("dropping low-priority SocketCAN frame because the outbound queue is full")
		case <-ctx.Done():
		}
	}()
}

// OutboundQueueLag returns recent SocketCAN outbound queue/send latency.
func (c *SocketCANEndpoint) OutboundQueueLag() time.Duration {
	updated := c.outboundLagUpdatedNano.Load()
	if updated == 0 || time.Since(time.Unix(0, updated)) > socketCANOutboundLagTTL {
		return 0
	}
	return time.Duration(c.outboundLagNano.Load())
}

func (c *SocketCANEndpoint) recordOutboundLag(item outboundSocketCANFrame) {
	if item.enqueuedAt.IsZero() {
		return
	}
	lag := time.Since(item.enqueuedAt)
	if lag < 0 {
		lag = 0
	}
	c.outboundLagNano.Store(int64(lag))
	c.outboundLagUpdatedNano.Store(time.Now().UnixNano())
}

func isSocketCANTxBufferFull(err error) bool {
	return stderrors.Is(err, syscall.ENOBUFS) || strings.Contains(err.Error(), "no buffer space available")
}

func isLowPrioritySocketCANFrame(frame can.Frame) bool {
	header := converter.DecodeCanID(frame.ID)
	switch header.PGN {
	case pgn.ISORequestPGN:
		requestedPGN, ok := requestedPGNFromISORequestFrame(frame)
		return ok && isDiscoveryRequestedPGN(requestedPGN)
	case pgn.ProductInformationPGN, pgn.ConfigurationInformationPGN, pgn.PGNListTransmitAndReceivePGN:
		return true
	default:
		return false
	}
}

func requestedPGNFromISORequestFrame(frame can.Frame) (uint32, bool) {
	if frame.Length < 3 {
		return 0, false
	}
	return uint32(frame.Data[0]) | uint32(frame.Data[1])<<8 | uint32(frame.Data[2])<<16, true
}

func isDiscoveryRequestedPGN(requestedPGN uint32) bool {
	switch requestedPGN {
	case pgn.ISOAddressClaimPGN,
		pgn.ProductInformationPGN,
		pgn.ConfigurationInformationPGN,
		pgn.PGNListTransmitAndReceivePGN:
		return true
	default:
		return false
	}
}

// frameReady is a helper to handle passing completed frames to the handler
func (c *SocketCANEndpoint) frameReady(frame can.Frame) {
	if c.handler != nil {
		c.handler.HandleMessage(endpoint.Message(&frame))
	}
}
