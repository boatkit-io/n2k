// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package n2kfileendpoint provides reads n2k log files and sends canbus frames to a channel.
// To use it connect its output channel to a canadapter instance.
package n2kfileendpoint

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// N2kFileEndpoint reads an n2k log file and sends canbus frames to its output channel.
type N2kFileEndpoint struct {
	log        *logrus.Logger
	inFilePath string

	mu      sync.Mutex
	inFile  *os.File
	running bool
	closed  bool
	handler endpoint.MessageHandler
}

// NewN2kFileEndpoint creates a new n2k endpoint.
func NewN2kFileEndpoint(file string, log *logrus.Logger) *N2kFileEndpoint {
	return &N2kFileEndpoint{
		log:        log,
		inFilePath: file,
	}
}

// SetOutput sets the output struct for handling when a message is ready
func (n *N2kFileEndpoint) SetOutput(mh endpoint.MessageHandler) {
	n.handler = mh
}

// Start synchronously verifies that the input log can be opened.
func (n *N2kFileEndpoint) Start(_ context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return errors.New("n2k file endpoint is closed")
	}
	if n.inFile != nil {
		return nil
	}
	file, err := os.Open(n.inFilePath)
	if err != nil {
		return err
	}
	n.inFile = file
	return nil
}

// Run replays frames from the opened input log until playback or the context ends.
func (n *N2kFileEndpoint) Run(ctx context.Context) error {
	if err := n.Start(ctx); err != nil {
		return err
	}
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return errors.New("n2k file endpoint is closed")
	}
	if n.running {
		n.mu.Unlock()
		return errors.New("n2k file endpoint is already running")
	}
	file := n.inFile
	n.running = true
	n.mu.Unlock()
	if file == nil {
		n.mu.Lock()
		n.running = false
		n.mu.Unlock()
		return errors.New("n2k input file is not open")
	}
	defer func() {
		if n.finishRun(file) {
			if err := file.Close(); err != nil {
				n.log.WithError(err).Warnf("failed to close n2k file %s", n.inFilePath)
			}
		}
	}()

	startTime := time.Now()

	n.log.Info("starting n2k file playback")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		// Sample line:
		// (010.139585)  can1  08FF0401   [8]  AC 98 21 FC 5E FD 64 FF
		line := scanner.Text()
		if line == "" {
			continue
		}
		var frame can.Frame
		var canDead string
		var timeDelta float32
		_, err := fmt.Sscanf(line, " (%f)  %s  %8X   [%d]", &timeDelta, &canDead, &frame.ID, &frame.Length)
		if err != nil {
			return err
		}
		_, tail, ok := strings.Cut(line, "]")
		if !ok {
			return fmt.Errorf("failed to cut line")
		}
		tail = strings.TrimSpace(tail)
		bts := strings.Split(tail, " ")
		for i := range frame.Length {
			_, err := fmt.Sscanf(bts[i], "%X", &frame.Data[i])
			if err != nil {
				return err
			}
		}
		// Pause until the timeDelta has expired, so this all replays in "real-time" (relative to start, obvs)
		for {
			curDelta := time.Since(startTime).Seconds()
			nextTime := timeDelta - float32(curDelta)
			wait := time.Duration(math.Min(500, float64(nextTime)*1000.0)) * time.Millisecond
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
					return nil
				}
			}

			if time.Since(startTime) > time.Duration(timeDelta) {
				break
			}
		}

		n.frameReady(&frame)
	}

	if err := scanner.Err(); err != nil {
		n.log.Warn(errors.Wrap(err, "error while scanning n2k replay file"))
	}

	n.log.Info("n2k file playback complete")

	return nil
}

// Close closes the endpoint
func (n *N2kFileEndpoint) Close() error {
	n.mu.Lock()
	n.closed = true
	file := n.inFile
	n.inFile = nil
	n.mu.Unlock()
	if file == nil {
		return nil
	}
	return file.Close()
}

// WriteFrame writes a CAN frame to the endpoint
func (n *N2kFileEndpoint) WriteFrame(_ can.Frame) {
	// For file endpoints, we don't support writing frames
	// This is a read-only endpoint
}

// frameReady is a helper to handle passing completed frames to the handler
func (n *N2kFileEndpoint) frameReady(frame endpoint.Message) {
	if n.handler != nil {
		n.handler.HandleMessage(frame)
	}
}

func (n *N2kFileEndpoint) finishRun(file *os.File) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.running = false
	if n.inFile == file {
		n.inFile = nil
		return true
	}
	return false
}
