package n2k

import (
	"fmt"
	"time"

	"github.com/brutella/can"
	// "github.com/sirupsen/logrus"
)

// PacketInfo contains info from the packet that will later go into the PGN structure as context
// /around/ the PGN struct's sending.
type PacketInfo struct {
	// when did we get the message
	Timestamp time.Time

	// 3-bit
	Priority uint8

	// 19-bit number
	PGN uint32

	// actually 8-bit
	SourceId uint8

	// target address, when relevant (PGNs with PF < 240)
	TargetId uint8
}

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

	Possibles []*pgnInfo // possible matches for current.Info.PGN

	// Decoders the caller can run once the full packet is assembled (if fast).
	Decoders []func(PacketInfo, *PGNDataStream) (interface{}, error)

	// Keep track of errors attempting to parse this
	ParseErrors []error
}

func NewPacket(message can.Frame) *Packet {
	p := Packet{}
	p.Data = message.Data[:]
	p.newInfo(message)
	p.Complete = true
	if p.valid() {
		p.getSeqFrame()
		p.Proprietary = IsProprietaryPGN(p.Info.PGN)
		p.Possibles = pgnInfoLookup[p.Info.PGN]
		if len(p.Possibles) == 0 {
			// not found, an unknown PGN
			p.ParseErrors = append(p.ParseErrors, fmt.Errorf("no data for pgn"))
		} else {
			p.Fast = p.Possibles[0].Fast // only misleading for PGN 130824
		}
		if p.Fast {
			p.Complete = false
			p.getSeqFrame()
		}
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

func (p *Packet) newInfo(message can.Frame) {
	p.Info = PacketInfo{
		Timestamp: time.Now(),
		SourceId:  uint8(message.ID & 0xFF),
		PGN:       (message.ID & 0x3FFFF00) >> 8,
		Priority:  uint8((message.ID & 0x1C000000) >> 26),
	}

	pduFormat := uint8((p.Info.PGN & 0xFF00) >> 8)
	if pduFormat < 240 {
		// This is a targeted packet, and the lower PS has the address
		p.Info.TargetId = uint8(p.Info.PGN & 0xFF)
		p.Info.PGN &= 0xFFF00
	}
}

func (p *Packet) FilterOnManufacturer(fast bool) {
	var s *PGNDataStream
	if fast {
		s = NewPgnDataStream(p.Data[2:])
	} else {
		s = NewPgnDataStream(p.Data)
	}
	manCode, err := getManCode(s)
	if err != nil {
		p.ParseErrors = append(p.ParseErrors, fmt.Errorf("couldn't read manufacturer for packet"))
		return
	}
	for _, d := range p.Possibles {
		if d.ManId == manCode {
			p.Decoders = append(p.Decoders, d.Decoder)
		}
	}
}

func (p *Packet) addDecoders() {
	for _, d := range p.Possibles {
		p.Decoders = append(p.Decoders, d.Decoder)
	}
}

func getManCode(stream *PGNDataStream) (code ManufacturerCodeConst, err error) {
	v, err := stream.ReadLookupField(11)
	code = ManufacturerCodeConst(v)
	stream.ResetToStart()
	return
}
