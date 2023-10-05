// Package n2kendpoint provides reads n2k log files and sends canbus frames to a channel.
// To use it connect its output channel to a canadapter instance.
package n2kendpoint

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path"
	"sync"
	"time"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/adapter/canadapter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// N2kEndpoint reads an n2k log file and sends canbus frames to its output channel.
type N2kEndpoint struct {
	frameC chan adapter.Message
	inFile string
	log    *logrus.Logger
}

// NewN2kEndpoint creates a new n2k endpoint.
func NewN2kEndpoint(fName string, log *logrus.Logger) *N2kEndpoint {
	return &N2kEndpoint{
		frameC: make(chan adapter.Message, 100),
		inFile: fName,
		log:    log,
	}
}

// OutChannel method returns the channel where it sends frames.
func (n *N2kEndpoint) OutChannel() chan adapter.Message {
	return n.frameC
}

// Run method opens the specified log file and kicks off a goroutine that sends frames to OutChannel.
func (n *N2kEndpoint) Run(wg *sync.WaitGroup) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	file, err := os.Open(path.Join(wd, "n2kreplays", n.inFile))
	if err != nil {
		return err
	}

	go func() {
		defer wg.Done()
		defer file.Close()

		startTime := time.Now()

		n.log.Info("starting n2k file playback")

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
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

			n.frameC <- frame
		}

		n.log.Info("n2k file playback complete")
		close(n.frameC)
		if err := scanner.Err(); err != nil {
			n.log.Warn(errors.Wrap(err, "error while scanning n2k replay file"))
		}
	}()
	return nil
}
