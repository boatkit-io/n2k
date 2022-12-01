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
// the slow variant starts 0x87 0x01. We'll look for this and if matched select
// it. The fast variant will have a seqid/framenum in byte 0 and a length of 9 in byte 1.
// The data length of 1 is invalid for an
func (b *PGNBuilder) handlePGN130824() {
	var pInfo *pgnInfo
	for _, pInfo = range b.current.Candidates {
		if pInfo.Fast {
			b.current.Complete = false
			b.current.Fast = true
			if b.current.FrameNum == 0 {
				b.current.FilterOnManufacturer()
			}
			if len(b.current.Decoders) > 0 {
				break
			}
		} else {
			b.current.FilterOnManufacturer()
			if len(b.current.Decoders) > 0 {
				b.current.Fast = false
				b.current.Complete = true
				break
			}
		}
	}
	if b.current.Complete && len(b.current.Decoders) == 0 {
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
			return
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
		if b.current.Complete && len(b.current.ParseErrors) > 0 {
			b.pgnCallback(b.current.unknownPGN())
			return
		}
	}
	if b.current.Fast {
		b.multi.Add(b.current)
		if b.current.FrameNum == 0 && len(b.current.Decoders) == 0 {
			if b.current.Proprietary {
				b.current.FilterOnManufacturer()
			} else {
				b.current.addDecoders()
			}
		}
	} else {
		if b.current.Proprietary {
			b.current.FilterOnManufacturer()
		} else {
			b.current.addDecoders()
		}
	}
	if b.current.Complete && len(b.current.Decoders) == 0 {
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
