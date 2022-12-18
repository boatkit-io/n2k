package n2k

import (
	"fmt"

	"github.com/brutella/can"
	// "github.com/sirupsen/logrus"
)

type Packet struct {
	Info PacketInfo

	// almost always 8 bytes, but treat it generically anyway
	Data []uint8

	// matching pgn variants are all fast or all slow.
	Fast bool

	SeqId uint8

	FrameNum uint8

	Proprietary bool

	Complete bool

	Manufacturer ManufacturerCodeConst // if Proprietary is true

	Candidates []*pgnInfo // possible matches for current.Info.PGN

	// Decoders the caller can run once the full packet is assembled (if fast).
	Decoders []func(PacketInfo, *PGNDataStream) (interface{}, error)

	// Keep track of errors attempting to parse this
	ParseErrors []error
}

func NewPacket(message can.Frame) *Packet {
	p := Packet{}
	p.Data = message.Data[:]
	p.Info = newPacketInfo(message)
	if p.valid() {
		p.getSeqFrame()
		p.Proprietary = IsProprietaryPGN(p.Info.PGN)
		p.Candidates = pgnInfoLookup[p.Info.PGN]
		if len(p.Candidates) == 0 {
			// not found, an unknown PGN
			p.ParseErrors = append(p.ParseErrors, fmt.Errorf("no data for pgn"))
		} else {
			p.Fast = p.Candidates[0].Fast // only misleading for PGN 130824
		}
		p.Complete = !p.Fast
	}
	return &p
}

func (p *Packet) valid() bool {
	result := true
	if p.Info.PGN == 0 {
		p.ParseErrors = append(p.ParseErrors, fmt.Errorf("PGN = 0"))
		result = false
	}
	if len(p.Data) == 0 {
		p.ParseErrors = append(p.ParseErrors, fmt.Errorf("Packet data is empty"))
		result = false
	}
	return result
}

func (p *Packet) getSeqFrame() {
	p.SeqId = (p.Data[0] & 0xE0) >> 5
	p.FrameNum = p.Data[0] & 0x1F

}

func (p *Packet) unknownPGN() UnknownPGN {
	return BuildUnknownPGN(p)
}

func (p *Packet) addDecoders() {
	p.getManCode() // sets p.Manufacturer
	for _, d := range p.Candidates {
		if p.Proprietary && p.Manufacturer != d.ManId {
			continue
		}
		p.Decoders = append(p.Decoders, d.Decoder)
	}
}

func (p *Packet) getManCode() {
	s := NewPgnDataStream(p.Data)
	v, err := s.ReadLookupField(11)
	if err == nil {
		p.Manufacturer = ManufacturerCodeConst(v)
	}
	s.ResetToStart()
}
