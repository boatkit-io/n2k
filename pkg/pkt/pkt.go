// Package pkt converts input messages to an intermediate (Packet) form, and outputs equivalent golang structs.
//
// The pkt package sits in the middle of the NMEA 2000 decoding pipeline. It receives raw
// frame data from an adapter layer and produces decoded Go structs representing specific
// PGN (Parameter Group Number) messages. The Packet type is the central data structure
// that accumulates frame data, tracks fast-packet assembly state, and holds candidate
// decoders for the final PGN interpretation.
package pkt

import (
	"fmt"

	"github.com/open-ships/n2k/pkg/pgn"
)

// Packet is the core data type used in the package.
// When complete a Packet contains the complete message (coallescing multiple fast packets if needed).
// It connects the encoded frame format used as the NMNEA 2000 wire format with the
// generic PGN data description derived from canboat.json.
//
// In our data flow it fits like this:
//
//	NMEA 2000/Canbus wire format
//	Gateway to Endpoint (various representations)
//	Endpoint to Adapter (various Message implementations)
//	Packet (Adapter intermediate format) <--- this type
//	Golang datatypes for each known PGN, or UnknownPGN (output from Adapter)
//
// A Packet starts life when a CAN frame arrives. For single-frame PGNs, the Packet is
// immediately complete. For fast-packet (multi-frame) PGNs, multiple CAN frames are
// assembled into a single Packet via the sequence/MultiBuilder system before the Packet
// is marked Complete and ready for decoding.
type Packet struct {
	// Info provides (for known PGNs), the generic description of the PGN derived from canboat.json.
	// This includes metadata like PGN number, source address, priority, destination, and timestamp.
	Info pgn.MessageInfo

	// Data is (when complete) the data payload for a PGN, ready to decode.
	// For single-frame messages this is the raw 8-byte CAN frame data.
	// For fast-packet messages this is the reassembled payload from multiple frames,
	// with the sequence/frame header bytes stripped and the result trimmed to the
	// expected length declared in frame 0.
	Data []uint8

	// Fast (when complete) indicates if matching pgn variants are all fast or all slow.
	// This is determined by looking up the PGN in the canboat-derived PgnInfoLookup table.
	// A "fast" PGN carries more than 8 bytes and must be split across multiple CAN frames.
	// Note: PGN 130824 is a special case where some variants are fast and some are slow.
	Fast bool

	// SeqId (for fast packets) is the 3-bit sequence identifier (0-7) that connects
	// partial packets belonging to the same logical message. Multiple in-flight sequences
	// for the same PGN/source can coexist using different SeqIds.
	SeqId uint8

	// FrameNum (for fast packets) is the 5-bit frame number (0-31) indicating the position
	// of this partial packet within the complete multi-frame message. Frame 0 carries a
	// length byte; subsequent frames are continuation frames.
	FrameNum uint8

	// Proprietary indicates if the PGN falls in the NMEA 2000 proprietary range.
	// Proprietary PGNs encode a manufacturer ID and industry code in their first two data
	// bytes, which is used to select the correct decoder from multiple candidates.
	Proprietary bool

	// Complete is true for single-frame messages and for fast-packet messages when all
	// frames in the sequence have been received and assembled. Only complete packets are
	// forwarded downstream for decoding.
	Complete bool

	// Manufacturer is the Manufacturer ID extracted from the first two bytes of proprietary
	// PGN data. Used to filter candidate decoders so only the matching manufacturer's
	// decoder is applied.
	Manufacturer pgn.ManufacturerCodeConst

	// Candidates is the list of all possible PGN definitions that match this PGN number.
	// Multiple candidates exist when different manufacturers define their own proprietary
	// messages under the same PGN number (e.g., PGN 130820 has many vendor-specific variants).
	Candidates []*pgn.PgnInfo

	// Decoders is the filtered list of decoder functions derived from Candidates.
	// After filtering by manufacturer ID (for proprietary PGNs), each remaining candidate's
	// Decoder function is added here. Each decoder takes a MessageInfo and a PGNDataStream
	// and returns a typed Go struct or an error if the data doesn't match.
	Decoders []func(pgn.MessageInfo, *pgn.PGNDataStream) (any, error)

	// ParseErrors tracks errors encountered during packet processing. Errors accumulate
	// as validation fails, decoders are attempted, or assembly issues arise. If all decoders
	// fail, these errors are bundled into the resulting UnknownPGN for debugging.
	ParseErrors []error
}

// NewPacket creates and returns a pointer to an initialized Packet from a MessageInfo and
// raw data bytes. It performs initial validation, determines if the PGN is proprietary,
// looks up candidate decoders from the canboat-derived PGN registry, and sets the Fast
// flag based on the first candidate's metadata.
//
// Parameters:
//   - info: MessageInfo containing PGN number, source, priority, destination, and timestamp.
//   - data: Raw payload bytes from the CAN frame.
//
// Returns a *Packet. If the PGN is unknown (not in the registry), ParseErrors will contain
// a "no data for pgn" error but the packet is still returned for downstream handling as
// an UnknownPGN.
func NewPacket(info pgn.MessageInfo, data []byte) *Packet {
	p := Packet{}
	p.Data = data
	p.Info = info
	if p.Valid() {
		// Check if this PGN falls in the NMEA 2000 proprietary range (manufacturer-specific).
		p.Proprietary = pgn.IsProprietaryPGN(p.Info.PGN)

		// Look up all known PGN definitions that match this PGN number.
		// For non-proprietary PGNs there is typically one candidate; for proprietary PGNs
		// there may be many (one per manufacturer that defines messages under this PGN).
		p.Candidates = pgn.PgnInfoLookup[p.Info.PGN]
		if len(p.Candidates) == 0 {
			// PGN not found in the canboat-derived registry -- treat as unknown.
			p.ParseErrors = append(p.ParseErrors, fmt.Errorf("no data for pgn"))
		} else {
			// Use the first candidate's Fast flag to determine if this is a multi-frame PGN.
			// This is only misleading for PGN 130824, where some variants are fast and some slow.
			p.Fast = p.Candidates[0].Fast // only misleading for PGN 130824
		}
	}
	return &p
}

// Valid performs lightweight sanity checking on the packet's essential fields.
// It verifies that the PGN number is non-zero and that the data payload is non-empty.
// Any validation failures are recorded in ParseErrors.
//
// Returns true if the packet passes all checks, false otherwise.
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

// GetSeqFrame extracts the sequence ID and frame number from the first byte of the packet data.
// In NMEA 2000 fast-packet encoding, the first byte of each CAN frame encodes:
//   - Bits 7-5 (upper 3 bits): Sequence ID (0-7), identifying which logical sequence this
//     frame belongs to. Multiple concurrent sequences for the same PGN/source use different IDs.
//   - Bits 4-0 (lower 5 bits): Frame number (0-31), indicating this frame's position in the
//     multi-frame message. Frame 0 is the initial frame containing the total payload length.
//
// Note: We can't always know if a packet is actually a fast-packet partial frame, so these
// extracted values are not always meaningful. They are only valid when the PGN is known to
// be a fast-packet type.
func (p *Packet) GetSeqFrame() {
	// Extract 3-bit sequence ID from bits 7-5 of the first data byte.
	p.SeqId = (p.Data[0] & 0xE0) >> 5
	// Extract 5-bit frame number from bits 4-0 of the first data byte.
	p.FrameNum = p.Data[0] & 0x1F

}

// UnknownPGN creates a new instance of UnknownPGN from this packet. This is used as a
// fallback when no decoder can successfully parse the packet data, preserving the raw
// data and accumulated error information for debugging and logging.
func (p *Packet) UnknownPGN() pgn.UnknownPGN {
	return buildUnknownPGN(p)
}

// AddDecoders populates the Decoders slice by filtering the Candidates list.
// For proprietary PGNs, it first extracts the manufacturer code from the data and then
// skips any candidate whose ManId doesn't match the packet's manufacturer. This ensures
// that only the correct manufacturer's decoder is attempted.
//
// For non-proprietary PGNs, all candidates pass through (the Proprietary flag is false,
// so the manufacturer check is skipped).
func (p *Packet) AddDecoders() {
	// Extract manufacturer code from the data payload (only meaningful for proprietary PGNs).
	p.GetManCode() // sets p.Manufacturer
	for _, d := range p.Candidates {
		// For proprietary PGNs, skip candidates from a different manufacturer.
		if p.Proprietary && p.Manufacturer != d.ManId {
			continue
		}
		p.Decoders = append(p.Decoders, d.Decoder)
	}
}

// GetManCode extracts and sets the Manufacturer code from the packet data.
// For proprietary PGNs, the first 11 bits of the data payload contain the manufacturer
// code (little-endian), and bits 13-15 contain the industry code. This method calls
// pgn.GetProprietaryInfo to perform the extraction and sets p.Manufacturer on success.
// If the extraction fails (e.g., insufficient data), the Manufacturer field is left at
// its zero value.
func (p *Packet) GetManCode() {
	m, _, err := pgn.GetProprietaryInfo(p.Data)
	if err == nil {
		p.Manufacturer = m
	}
}
