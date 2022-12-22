package n2k

import (
	"fmt"

	"github.com/brutella/can"
	// "github.com/sirupsen/logrus"
)

type packet struct {
	info PacketInfo

	// almost always 8 bytes, but treat it generically anyway
	data []uint8

	// matching pgn variants are all fast or all slow.
	fast bool

	seqId uint8

	frameNum uint8

	proprietary bool

	complete bool

	manufacturer ManufacturerCodeConst // if Proprietary is true

	candidates []*PgnInfo // possible matches for current.Info.PGN

	// decoders the caller can run once the full packet is assembled (if fast).
	decoders []func(PacketInfo, *pGNDataStream) (interface{}, error)

	// Keep track of errors attempting to parse this
	parseErrors []error
}

func newPacket(message can.Frame) *packet {
	p := packet{}
	p.data = message.Data[:]
	p.info = newPacketInfo(message)
	if p.valid() {
		p.getSeqFrame()
		p.proprietary = IsProprietaryPGN(p.info.PGN)
		p.candidates = pgnInfoLookup[p.info.PGN]
		if len(p.candidates) == 0 {
			// not found, an unknown PGN
			p.parseErrors = append(p.parseErrors, fmt.Errorf("no data for pgn"))
		} else {
			p.fast = p.candidates[0].Fast // only misleading for PGN 130824
		}
		p.complete = !p.fast
	}
	return &p
}

func (p *packet) valid() bool {
	result := true
	if p.info.PGN == 0 {
		p.parseErrors = append(p.parseErrors, fmt.Errorf("PGN = 0"))
		result = false
	}
	if len(p.data) == 0 {
		p.parseErrors = append(p.parseErrors, fmt.Errorf("packet data is empty"))
		result = false
	}
	return result
}

func (p *packet) getSeqFrame() {
	p.seqId = (p.data[0] & 0xE0) >> 5
	p.frameNum = p.data[0] & 0x1F

}

func (p *packet) unknownPGN() UnknownPGN {
	return buildUnknownPGN(p)
}

func (p *packet) addDecoders() {
	p.getManCode() // sets p.Manufacturer
	for _, d := range p.candidates {
		if p.proprietary && p.manufacturer != d.ManId {
			continue
		}
		p.decoders = append(p.decoders, d.Decoder)
	}
}

func (p *packet) getManCode() {
	s := newPgnDataStream(p.data)
	v, err := s.readLookupField(11)
	if err == nil {
		p.manufacturer = ManufacturerCodeConst(v)
	}
	s.resetToStart()
}
