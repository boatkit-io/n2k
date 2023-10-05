// Package canadapter implements the adapter interface for n2k endpoints.
package canadapter

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/adapter"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

// CanAdapter instances read canbus frames from its input and outputs complete Packets.
type CanAdapter struct {
	frameC  chan adapter.Message // input channel
	packetC chan pkt.Packet      // output channel
	multi   *MultiBuilder        // combines multiple frames into a complete Packet.
	current *pkt.Packet
	log     *logrus.Logger
}

// NewCanAdapter instantiates a new CanAdapter
func NewCanAdapter(log *logrus.Logger) *CanAdapter {
	return &CanAdapter{
		multi: NewMultiBuilder(log),
		log:   log,
	}
}

// SetInChannel method sets the input channel
func (c *CanAdapter) SetInChannel(in chan adapter.Message) {
	c.frameC = in
}

// SetOutChannel method sets the output channel
func (c *CanAdapter) SetOutChannel(out chan pkt.Packet) {
	c.packetC = out
}

// Run method kicks off a goroutine that reads messages from the input channel and writes complete packets to the output channel.
func (c *CanAdapter) Run(wg *sync.WaitGroup) error {
	go func() {
		defer wg.Done()
		for {

			m, more := <-c.frameC
			if !more {
				close(c.packetC)
				return
			}
			switch f := m.(type) {
			case Frame:
				pInfo := NewPacketInfo(f)
				c.current = pkt.NewPacket(pInfo, f.Data[:])
				c.process()
			default:
				c.log.Warnf("CanAdapter expected Frame, received: %T\n", f)
			}
		}

	}()
	return nil
}

// process method is the worker function for Run
func (c *CanAdapter) process() {
	// https://endige.com/2050/nmea-2000-pgns-deciphered/

	if len(c.current.ParseErrors) > 0 {
		c.packetC <- *c.current
		return
	}

	if c.current.Fast {
		c.multi.Add(c.current)
	} else {
		c.current.Complete = true
	}
	if c.current.Complete {
		c.current.AddDecoders()
		c.packetC <- *c.current
	}
}
