// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

// Package rawendpoint turns CAN frames written to pgn.Writer into RAW format and saves them to a file.
package rawendpoint

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// RawEndpoint writes a raw log file from canbus frames sent through the write pipeline.
// Initially through stdout
type RawEndpoint struct {
	log         *logrus.Logger
	outFilePath string
	mu          sync.Mutex
	file        *os.File
	done        chan struct{}
	closeOnce   sync.Once
	running     bool
	closed      bool
	handler     endpoint.MessageHandler
}

// RawFileEndpoint reads a raw log file and sends canbus frames to its output channel.
type RawFileEndpoint struct {
	log        *logrus.Logger
	inFilePath string
	mu         sync.Mutex
	inFile     *os.File
	running    bool
	closed     bool
	handler    endpoint.MessageHandler
	rand       *rand.Rand
}

// NewRawEndpoint creates a new RAW endpoint
func NewRawEndpoint(outFilePath string, log *logrus.Logger) *RawEndpoint {
	return &RawEndpoint{
		log:         log,
		outFilePath: outFilePath,
		done:        make(chan struct{}),
	}
}

// NewRawFileEndpoint creates a new raw file endpoint for replaying raw log files
func NewRawFileEndpoint(file string, log *logrus.Logger) *RawFileEndpoint {
	return &RawFileEndpoint{
		log:        log,
		inFilePath: file,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Start synchronously opens the configured raw output file.
func (r *RawEndpoint) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("raw output endpoint is closed")
	}
	if r.file != nil || r.outFilePath == "" {
		return nil
	}
	file, err := os.Create(r.outFilePath)
	if err != nil {
		return fmt.Errorf("open RAW output file: %w", err)
	}
	r.file = file
	return nil
}

// Run keeps the raw output endpoint active until the context ends.
func (r *RawEndpoint) Run(ctx context.Context) error {
	if err := r.Start(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("raw output endpoint is closed")
	}
	if r.running {
		r.mu.Unlock()
		return errors.New("raw output endpoint is already running")
	}
	r.running = true
	done := r.done
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()
	r.log.Info("starting raw endpoint")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

// SetOutput sets the output struct for handling when a message is ready
func (r *RawEndpoint) SetOutput(mh endpoint.MessageHandler) {
	r.handler = mh
}

// WriteFrame is invoked by CanAdapter, converts the frame into a RAW string, and writes it to the file.
func (r *RawEndpoint) WriteFrame(frame can.Frame) {
	outStr := converter.RawFromCanFrame(frame)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.file != nil {
		if _, err := r.file.WriteString(outStr); err != nil {
			r.log.WithError(err).Error("failed to write raw frame")
		}
	} else {
		r.log.Info(outStr)
	}
}

// Close closes the endpoint
func (r *RawEndpoint) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	file := r.file
	r.file = nil
	r.mu.Unlock()
	r.closeOnce.Do(func() {
		close(r.done)
	})
	if file != nil {
		if err := file.Close(); err != nil {
			return errors.Wrap(err, "failed to close raw endpoint file")
		}
	}
	return nil
}

// Start synchronously verifies that the raw input file can be opened.
func (r *RawFileEndpoint) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("raw input endpoint is closed")
	}
	if r.inFile != nil {
		return nil
	}
	file, err := os.Open(r.inFilePath)
	if err != nil {
		return err
	}
	r.inFile = file
	return nil
}

// Run replays frames from the opened raw input file until playback or the context ends.
func (r *RawFileEndpoint) Run(ctx context.Context) error {
	if err := r.Start(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return errors.New("raw input endpoint is closed")
	}
	if r.running {
		r.mu.Unlock()
		return errors.New("raw input endpoint is already running")
	}
	file := r.inFile
	r.running = true
	r.mu.Unlock()
	if file == nil {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
		return errors.New("raw input file is not open")
	}
	defer func() {
		if r.finishRun(file) {
			if err := file.Close(); err != nil {
				r.log.WithError(err).Errorf("failed to close raw input file %s", r.inFilePath)
			}
		}
	}()

	r.log.Info("starting raw file playback")

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		frames, err := converter.CanFrameFromRaw(line)
		if err != nil {
			r.log.Warnf("Error parsing raw line: %v", err)
			continue
		}

		// If this is a multi-frame message, generate a random sequence ID
		if len(frames) > 1 {
			seqID := uint8(r.rand.Intn(7)) // Generate random number 0-6
			for _, frame := range frames {
				// For all frames: replace bits 5-7 with sequence ID
				frame.Data[0] = (seqID << 5) | (frame.Data[0] & 0x1F)
				r.frameReady(frame)
			}
		} else {
			r.frameReady(frames[0])
		}
	}

	if err := scanner.Err(); err != nil {
		r.log.Warn(errors.Wrap(err, "error while scanning raw replay file"))
	}

	r.log.Info("raw file playback complete")
	return nil
}

// Close closes an open raw input file.
func (r *RawFileEndpoint) Close() error {
	r.mu.Lock()
	r.closed = true
	file := r.inFile
	r.inFile = nil
	r.mu.Unlock()
	if file == nil {
		return nil
	}
	return file.Close()
}

func (r *RawFileEndpoint) finishRun(file *os.File) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	if r.inFile == file {
		r.inFile = nil
		return true
	}
	return false
}

// frameReady is a helper to handle passing completed frames to the handler
func (r *RawFileEndpoint) frameReady(frame endpoint.Message) {
	if r.handler != nil {
		r.handler.HandleMessage(frame)
	}
}

// SetOutput sets the output struct for handling when a message is ready
func (r *RawFileEndpoint) SetOutput(mh endpoint.MessageHandler) {
	r.handler = mh
}

// WriteFrame is not implemented for RawFileEndpoint as it's read-only
func (r *RawFileEndpoint) WriteFrame(_ can.Frame) {
	// RawFileEndpoint is read-only, so this is a no-op
}
