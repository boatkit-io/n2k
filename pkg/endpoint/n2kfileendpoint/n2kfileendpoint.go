// Package n2kfileendpoint provides reads n2k log files and sends canbus frames to a channel.
// To use it connect its output channel to a canadapter instance.
package n2kfileendpoint

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// N2kFileEndpoint reads an n2k log file and sends canbus frames to its output channel.
type N2kFileEndpoint struct {
	log        *logrus.Logger
	inFilePath string

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

// Run method opens the specified log file and kicks off a goroutine that sends frames to the handler
func (n *N2kFileEndpoint) Run(ctx context.Context) error {
	file, err := os.Open(n.inFilePath)
	if err != nil {
		return err
	}

	defer file.Close()

	startTime := time.Now()

	n.log.Info("starting n2k file playback")

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

		// Sample line:
		// (010.139585)  can1  08FF0401   [8]  AC 98 21 FC 5E FD 64 FF
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		var frame can.Frame
		var canDead string
		var timeDelta float32
		_, err := fmt.Sscanf(line, " (%f)  %s  %8X   [%d]  %X %X %X %X %X %X %X %X", &timeDelta, &canDead, &frame.ID, &frame.Length, &frame.Data[0], &frame.Data[1], &frame.Data[2], &frame.Data[3], &frame.Data[4], &frame.Data[5], &frame.Data[6], &frame.Data[7])
		if err != nil {
			return err
		}
		// Pause until the timeDelta has expired, so this all replays in "real-time" (relative to start, obvs)
		for {
			if canceled {
				break
			}

			curDelta := time.Since(startTime).Seconds()
			nextTime := timeDelta - float32(curDelta)
			// Make sure we wait at most 0.5 seconds to gracefully quit as needed
			time.Sleep(time.Duration(math.Min(500, float64(nextTime)*1000.0)) * time.Millisecond)

			if time.Since(startTime) > time.Duration(timeDelta) {
				break
			}
		}
		if canceled {
			break
		}

		n.frameReady(&frame)
	}

	if err := scanner.Err(); err != nil {
		n.log.Warn(errors.Wrap(err, "error while scanning n2k replay file"))
	}

	n.log.Info("n2k file playback complete")

	return nil
}

// frameReady is a helper to handle passing completed frames to the handler
func (n *N2kFileEndpoint) frameReady(frame adapter.Message) {
	if n.handler != nil {
		n.handler.HandleMessage(frame)
	}
}
