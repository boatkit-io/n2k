// Package n2kendpoint provides reads n2k log files and sends canbus frames to a channel.
// To use it connect its output channel to a canadapter instance.
package n2kendpoint

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os"
	"path"
	"time"

	"github.com/boatkit-io/goatutils/pkg/subscribableevent"
	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// N2kEndpoint reads an n2k log file and sends canbus frames to its output channel.
type N2kEndpoint struct {
	log    *logrus.Logger
	inFile string

	frameReady subscribableevent.Event[func(adapter.Message)]
}

// NewN2kEndpoint creates a new n2k endpoint.
func NewN2kEndpoint(fName string, log *logrus.Logger) *N2kEndpoint {
	return &N2kEndpoint{
		log:    log,
		inFile: fName,

		frameReady: subscribableevent.NewEvent[func(adapter.Message)](),
	}
}

// SubscribeToFrameReady subscribes a callback function for whenever a frame is ready
func (n *N2kEndpoint) SubscribeToFrameReady(f func(adapter.Message)) subscribableevent.SubscriptionId {
	return n.frameReady.Subscribe(f)
}

// UnsubscribeFromFrameReady unsubscribes a previous subscription for ready frames
func (n *N2kEndpoint) UnsubscribeFromFrameReady(t subscribableevent.SubscriptionId) error {
	return n.frameReady.Unsubscribe(t)
}

// Run method opens the specified log file and kicks off a goroutine that sends frames to OutChannel.
func (n *N2kEndpoint) Run(ctx context.Context) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	file, err := os.Open(path.Join(wd, "n2kreplays", n.inFile))
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
		var frame canadapter.Frame
		var canDead string
		var timeDelta float32
		fmt.Sscanf(line, " (%f)  %s  %8X   [%d]  %X %X %X %X %X %X %X %X", &timeDelta, &canDead, &frame.ID, &frame.Length, &frame.Data[0], &frame.Data[1], &frame.Data[2], &frame.Data[3], &frame.Data[4], &frame.Data[5], &frame.Data[6], &frame.Data[7])
		// Pause until the timeDelta has expired, so this all replays in "real-time" (relative to start, obvs)
		for {
			//				if n.shuttingDown {
			//					return
			//				}

			curDelta := time.Since(startTime).Seconds()
			nextTime := timeDelta - float32(curDelta)
			// Make sure we wait at most 0.5 seconds to gracefully quit as needed
			time.Sleep(time.Duration(math.Min(500, float64(nextTime)*1000.0)) * time.Millisecond)

			if time.Since(startTime) > time.Duration(timeDelta) {
				break
			}
		}

		n.frameReady.Fire(frame)
	}

	if err := scanner.Err(); err != nil {
		n.log.Warn(errors.Wrap(err, "error while scanning n2k replay file"))
	}

	n.log.Info("n2k file playback complete")

	return nil
}
