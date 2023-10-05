// Package pkt converts input messages to an intermediate (Packet) form, and outputs equivalent golang structs.
package pkt

import (
	"fmt"
	// "github.com/sirupsen/logrus"
	"github.com/boatkit-io/n2k/pkg/pgn"
)

// Packet is the core data type used in the package.
// When complete a Packet contains the complete message (coallescing multiple fast packets if needed).
// It connects the encoded frame format used as the NMNEA 2000 wire format with the
// generic PGN data description derived from canboat.json.
// In our data flow it fits like this:
// NMEA 2000/Canbus wire format
// Gateway to Endpoint (various representations)
// Endpoint to Adapter (various Message implentations)
// Packet (Adapter intermediate format) <--- this type
// Golang datatypes for each known PGN, or UnknownPGN (output from Adapter)
type Packet struct {
	// Info provides (for known PGNs), the generic description of the PGN derived from canboat.json.
	Info pgn.MessageInfo

	// Data is (when complete) the data payload for a PGN, ready to decode.
	Data []uint8

	// Fast (when complete) indicates if matching pgn variants are all fast or all slow.
	Fast bool

	// SeqId (for fast packets) is the sequence identifier that connects partial packets.
	SeqId uint8

	// FrameNum (for fast packets) indicates the position of the partial packet in the complete message.
	FrameNum uint8

	// Proprietary indicates if the PGN is proprietary (see canboat documentation).
	Proprietary bool

	// Complete is true for single messages and for fast messages when all packets have been received.
	Complete bool

	// Manufacturer is the Manufacturer ID (for fast messages only)
	Manufacturer pgn.ManufacturerCodeConst

	// Candidates is a list of possible decoders for this PGN
	Candidates []*pgn.PgnInfo

	// Decoders reduces the list of candidate decoders to those that match the complete Packet.
	// We eliminate possible matches with different Manufacturer IDs.
	// And fast decoders for single packets (and vice versa).
	Decoders []func(pgn.MessageInfo, *pgn.PGNDataStream) (interface{}, error)

	// ParseErrors track errors in processing the input (we might try multiple decoders)
	ParseErrors []error
}

// NewPacket returns a pointer to an initialized new packet,
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

// Valid does light sanity checking on a packet.
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

// GetSeqFrame extracts the frame number from a packet.
// We can't always know if it's a partial packet, so these values aren't always valid.
func (p *Packet) GetSeqFrame() {
	p.SeqId = (p.Data[0] & 0xE0) >> 5
	p.FrameNum = p.Data[0] & 0x1F

}

// UnknownPGN creates a new instance of UnknownPGN.
func (p *Packet) UnknownPGN() pgn.UnknownPGN {
	return buildUnknownPGN(p)
}

// AddDecoders filters candidate decoders when the manufacturer of a proprietary messasge doesn't match.
func (p *Packet) AddDecoders() {
	p.GetManCode() // sets p.Manufacturer
	for _, d := range p.Candidates {
		if p.Proprietary && p.Manufacturer != d.ManId {
			continue
		}
		p.Decoders = append(p.Decoders, d.Decoder)
	}
}

// GetManCode sets the Manufacturer for proprietary PGNs.
func (p *Packet) GetManCode() {
	m, _, err := pgn.GetProprietaryInfo(p.Data)
	if err == nil {
		p.Manufacturer = m
	}
}
