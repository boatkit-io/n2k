package pkt

import (
	"fmt"
	// "github.com/sirupsen/logrus"
	"github.com/boatkit-io/n2k/pkg/pgn"
)

type Packet struct {
	Info pgn.MessageInfo

	// almost always 8 bytes, but treat it generically anyway
	Data []uint8

	// matching pgn variants are all fast or all slow.
	Fast bool

	SeqId uint8

	FrameNum uint8

	Proprietary bool

	Complete bool

	Manufacturer pgn.ManufacturerCodeConst // if Proprietary is true

	Candidates []*pgn.PgnInfo // possible matches for current.Info.PGN

	// Decoders the caller can run once the full packet is assembled (if fast).
	Decoders []func(pgn.MessageInfo, *pgn.PGNDataStream) (interface{}, error)

	// Keep track of errors attempting to parse this
	ParseErrors []error
}

func NewPacket(info pgn.MessageInfo, data []byte) *Packet {
	p := Packet{}
	p.Data = data
	p.Info = info
	if p.Valid() {
		p.Proprietary = pgn.IsProprietaryPGN(p.Info.PGN)
		p.Candidates = pgn.PgnInfoLookup[p.Info.PGN]
		if len(p.Candidates) == 0 {
			// not found, an unknown PGN
			p.ParseErrors = append(p.ParseErrors, fmt.Errorf("no data for pgn"))
		} else {
			p.Fast = p.Candidates[0].Fast // only misleading for PGN 130824
		}
	}
	return &p
}

func (p *Packet) Valid() bool {
	result := true
	if p.Info.PGN == 0 {
		p.ParseErrors = append(p.ParseErrors, fmt.Errorf("PGN = 0"))
		result = false
	}
	if len(p.Data) == 0 {
		p.ParseErrors = append(p.ParseErrors, fmt.Errorf("packet data is empty"))
		result = false
	}
	return result
}

func (p *Packet) GetSeqFrame() {
	p.SeqId = (p.Data[0] & 0xE0) >> 5
	p.FrameNum = p.Data[0] & 0x1F

}

func (p *Packet) UnknownPGN() pgn.UnknownPGN {
	return buildUnknownPGN(p)
}

func (p *Packet) AddDecoders() {
	p.GetManCode() // sets p.Manufacturer
	for _, d := range p.Candidates {
		if p.Proprietary && p.Manufacturer != d.ManId {
			continue
		}
		p.Decoders = append(p.Decoders, d.Decoder)
	}
}

func (p *Packet) GetManCode() {
	m, _, err := pgn.GetProprietaryInfo(p.Data)
	if err == nil {
		p.Manufacturer = m
	}
}
