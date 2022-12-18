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

func (b *PGNBuilder) ProcessFrame(message can.Frame) {
	// Decent reference:
	// https://www.nmea.org/Assets/20090423%20rtcm%20white%20paper%20nmea%202000.pdf
	// https://forums.ni.com/t5/LabVIEW/How-do-I-read-the-larger-than-8-byte-messages-from-a-NMEA-2000/td-p/3132045#:~:text=The%20Fast%20Packet%20protocol%20defined,parameter%20group%20identity%20and%20priority.
	// NewPacket unpacks the can frame into a Packet, does some basic validation, and
	// initializes some of the Packet's fields. Subsequent processing may override these
	// settings as we establish more specifics in process()
	b.current = NewPacket(message)
	b.process()
}

// if NewPacket detected errors process() returns an unknownPGN
// it checks for the special case of PGN 130824, the only PGN with a Fast and a Single variant
// if the current packet is Fast it passes it to the object that completes the assembly
// if the Packet is complete it adds the decoders (filtering Candidates for matching
// manufacturer ID if proprietary) and then invokes them
// if none match an unknownPGN is returned
func (b *PGNBuilder) process() {
	// https://endige.com/2050/nmea-2000-pgns-deciphered/

	if len(b.current.ParseErrors) > 0 {
		b.pgnCallback(b.current.unknownPGN())
		return
	}

	if b.current.Info.PGN == 130824 {
		b.handlePGN130824()
	}
	if b.current.Fast {
		b.multi.Add(b.current)
	}
	if b.current.Complete {
		b.current.addDecoders()
		b.decode()
	}
}

// PGN 130824 has 1 fast and 1 slow variant. We validate this invariant
// on every import of canboat.json. If it changes we need to revisit this code.
// the slow variant starts 0x7D 0x81 (man code and industry). We'll look for this and if matched select
// it. The fast variant is length 9, fitting in 2 frames, so the first byte of either frame
// can't be 0x7D
func (b *PGNBuilder) handlePGN130824() {
	var pInfo *pgnInfo
	b.current.Fast = true      // if slow match fails, the normal code will process this
	b.current.Complete = false //
	b.current.getManCode()     // have to peak ahead in this special case
	for _, pInfo = range b.current.Candidates {
		if pInfo.Fast {
			break // we only want to check the slow variant here
		} else {
			if b.current.Manufacturer == pInfo.ManId {
				b.current.Fast = false
				b.current.Complete = true
				break
			}
		}
	}
}

// if no decoders, return an UnknownPGN
// else invoke them each in turn, and if one matches return the result to subscribers
func (b *PGNBuilder) decode() {

	if len(b.current.Decoders) > 0 {
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
	}
	b.current.ParseErrors = append(b.current.ParseErrors, fmt.Errorf("No matching Decoder"))
	b.pgnCallback(b.current.unknownPGN())
}
