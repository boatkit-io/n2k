package n2k

import (
	"fmt"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
)

type PGNBuilder struct {
	log *logrus.Logger

	// Track fast packets per source id
	multi *MultiBuilder

	// Callback function for completed PGNs
	pgnCallback func(interface{})

	current *Packet
}

func NewPGNBuilder(log *logrus.Logger, pgnCallback func(interface{})) *PGNBuilder {
	result := PGNBuilder{
		log:         log,
		multi:       NewMultiBuilder(),
		pgnCallback: pgnCallback,
	}
	return &result
}

// PGN 130824 has 1 fast and 1 slow variant. We validate this invariant
// on every import of canboat.json. If it changes we need to revisit this code.
// The reason we special case it is the slow variant has as its first
// 2 bytes 01 87, the manufacturer id.
// That value is sequence 0, frame 1, a valid 2nd frame for a fast variant.
// fast frames can come out of sequence, so how can we tell which this is?
// we can't, but we can hope that a valid 2nd frame wouldn't have 0x87 as its
// first byte of data. could happen, and for extra credit we could
// try to decode it as a slow packet and if that fails add it to
// the multi sequence and wait. For now, it's a hole.
func (b *PGNBuilder) handlePGN130824() {
	var pInfo *pgnInfo
	for _, pInfo = range b.current.Possibles {
		if pInfo.Fast {
			b.current.FilterOnManufacturer(true)
			if len(b.current.Decoders) > 0 {
				b.current.Fast = true
				break
			}
		} else {
			b.current.FilterOnManufacturer(false)
			if len(b.current.Decoders) > 0 {
				b.current.Fast = false
				break
			}
		}
	}
	if len(b.current.Decoders) == 0 {
		b.current.ParseErrors = append(b.current.ParseErrors, fmt.Errorf("no matching Manufacturer for %d", b.current.Info.PGN))
		b.current.Complete = true
	}
}

func (b *PGNBuilder) decode() {
	// call frame decoders, pass a valid match to the callback
	stream := NewPgnDataStream(b.current.Data)
	for _, decoder := range b.current.Decoders {
		ret, err := decoder(b.current.Info, stream)
		if err != nil {
			b.current.ParseErrors = append(b.current.ParseErrors, err)
			stream.ResetToStart()
			continue
		} else {
			b.pgnCallback(ret)
		}
	}
	if len(b.current.ParseErrors) == 0 {
		b.current.ParseErrors = append(b.current.ParseErrors, fmt.Errorf("No matching Decoder"))
	}
	b.pgnCallback(b.current.unknownPGN())
}

func (b *PGNBuilder) process() {
	// https://endige.com/2050/nmea-2000-pgns-deciphered/

	if len(b.current.ParseErrors) > 0 {
		b.pgnCallback(b.current.unknownPGN())
		return
	}

	if b.current.Info.PGN == 130824 {
		b.handlePGN130824()
		if len(b.current.ParseErrors) > 0 {
			b.pgnCallback(b.current.unknownPGN())
			return
		}
	}
	if b.current.Fast {
		b.multi.Add(b.current)
		if b.current.FrameNum == 0 {
			if b.current.Proprietary {
				b.current.FilterOnManufacturer(true)
			} else {
				b.current.addDecoders()
			}
		}
	} else {
		if b.current.Proprietary {
			b.current.FilterOnManufacturer(false)
		} else {
			b.current.addDecoders()
		}
	}
	if len(b.current.ParseErrors) > 0 {
		b.pgnCallback(b.current.unknownPGN())
		return
	}
	if b.current.Complete { // all singles, a complete fast,
		b.decode()
	}
}

func (b *PGNBuilder) ProcessFrame(message can.Frame) {
	// Decent reference:
	// https://www.nmea.org/Assets/20090423%20rtcm%20white%20paper%20nmea%202000.pdf
	// https://forums.ni.com/t5/LabVIEW/How-do-I-read-the-larger-than-8-byte-messages-from-a-NMEA-2000/td-p/3132045#:~:text=The%20Fast%20Packet%20protocol%20defined,parameter%20group%20identity%20and%20priority.

	b.current = NewPacket(message)
	b.process()
}
