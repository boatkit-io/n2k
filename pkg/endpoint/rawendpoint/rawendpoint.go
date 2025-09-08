// Package rawendpoint turns CAN frames written to pgn.Writer into RAW format and saves them to a file.
package rawendpoint

import (
	"bufio"
	"context"
	"math/rand"
	"os"
	"time"

	"github.com/boatkit-io/n2k/internal/adapter"
	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// RawEndpoint writes a raw log file from canbus frames sent through the write pipeline.
// Initially through stdout
type RawEndpoint struct {
	log     *logrus.Logger
	file    *os.File
	handler endpoint.MessageHandler
}

// RawFileEndpoint reads a raw log file and sends canbus frames to its output channel.
type RawFileEndpoint struct {
	log        *logrus.Logger
	inFilePath string
	handler    endpoint.MessageHandler
	rand       *rand.Rand
}

// NewRawEndpoint creates a new RAW endpoint
func NewRawEndpoint(outFilePath string, log *logrus.Logger) *RawEndpoint {
	retval := RawEndpoint{}
	if outFilePath != "" {
		file, err := os.Create(outFilePath)
		if err != nil {
			log.Infof("RAW output file failed to open: %s", err)
		} else {
			retval.file = file
		}

	}
	return &retval
}

// NewRawFileEndpoint creates a new raw file endpoint for replaying raw log files
func NewRawFileEndpoint(file string, log *logrus.Logger) *RawFileEndpoint {
	return &RawFileEndpoint{
		log:        log,
		inFilePath: file,
		rand:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run method opens the specified log file and kicks off a goroutine that sends frames to the handler
func (r *RawEndpoint) Run(ctx context.Context) error {
	if r.file != nil {
		defer func() {
			if err := r.file.Close(); err != nil {
				r.log.WithError(err).Error("failed to close raw endpoint file")
			}
		}()
	}

	go func() {
		<-ctx.Done()
	}()

	return nil
}

// SetOutput sets the output struct for handling when a message is ready
func (r *RawEndpoint) SetOutput(mh endpoint.MessageHandler) {
	r.handler = mh
}

// WriteFrame is invoked by CanAdapter, converts the frame into a RAW string, and writes it to the file.
func (r *RawEndpoint) WriteFrame(frame can.Frame) {
	outStr := converter.RawFromCanFrame(frame)
	if r.file != nil {
		_, _ = r.file.WriteString(outStr)
	} else {
		r.log.Info(outStr)

	}

}

// Run method opens the specified log file and kicks off a goroutine that sends frames to the handler
func (r *RawFileEndpoint) Run(ctx context.Context) error {
	file, err := os.Open(r.inFilePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			r.log.WithError(err).Errorf("failed to close raw input file %s", r.inFilePath)
		}
	}()

	r.log.Info("starting raw file playback")

	canceled := false
	go func() {
		<-ctx.Done()
		canceled = true
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if canceled {
			break
		}

		line := scanner.Text()
		if len(line) == 0 {
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

// frameReady is a helper to handle passing completed frames to the handler
func (r *RawFileEndpoint) frameReady(frame adapter.Message) {
	if r.handler != nil {
		r.handler.HandleMessage(frame)
	}
}

// SetOutput sets the output struct for handling when a message is ready
func (r *RawFileEndpoint) SetOutput(mh endpoint.MessageHandler) {
	r.handler = mh
}
