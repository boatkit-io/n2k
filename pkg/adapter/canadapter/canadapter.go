package canadapter

import (
	"sync"

	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"
)

type CanAdapter struct {
	frameC  chan Frame
	packetC chan pkt.Packet
	multi   *MultiBuilder
	current *pkt.Packet
	log     *logrus.Logger
}

func NewCanAdapter(log *logrus.Logger) *CanAdapter {
	return &CanAdapter{
		multi: NewMultiBuilder(log),
		log:   log,
	}
}

func (c *CanAdapter) SetInChannel(in chan Frame) {
	c.frameC = in
}

func (c *CanAdapter) SetOutChannel(out chan pkt.Packet) {
	c.packetC = out
}

func (c *CanAdapter) Run(wg *sync.WaitGroup) error {
	go func() {
		defer wg.Done()
		for {

			f, more := <-c.frameC
			if !more {
				close(c.packetC)
				return
			}
			pInfo := NewPacketInfo(f)
			c.current = pkt.NewPacket(pInfo, f.Data[:])
			c.process()
		}

	}()
	return nil
}

func (c *CanAdapter) process() {
	// https://endige.com/2050/nmea-2000-pgns-deciphered/

	if len(c.current.ParseErrors) > 0 {
		c.packetC <- *c.current
		return
	}

	if c.current.Info.PGN == 130824 {
		c.handlePGN130824()
	}
	if c.current.Fast {
		c.multi.Add(c.current)
	}
	if c.current.Complete {
		c.current.AddDecoders()
		c.packetC <- *c.current
	}
}

// PGN 130824 has 1 fast and 1 slow variant. We validate this invariant
// on every import of canboat.json. If it changes we need to revisit this code.
// the slow variant starts 0x7D 0x81 (man code and industry). We'll look for this and if matched select
// it. The fast variant is length 9, fitting in 2 frames, so the first byte of either frame
// can't be 0x7D
func (c *CanAdapter) handlePGN130824() {
	var pInfo *pgn.PgnInfo
	c.current.Fast = true      // if slow match fails, the normal code will process this
	c.current.Complete = false //
	c.current.GetManCode()     // have to peak ahead in this special case
	for _, pInfo = range c.current.Candidates {
		if pInfo.Fast {
			break // we only want to check the slow variant here
		} else {
			if c.current.Manufacturer == pInfo.ManId {
				c.current.Fast = false
				c.current.Complete = true
				break
			}
		}
	}
}
