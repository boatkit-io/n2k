package canadapter

import (
	"github.com/open-ships/n2k/pkg/pkt"
)

// MultiBuilder manages the assembly of multi-frame NMEA 2000 fast packets into complete
// single Packets. NMEA 2000 messages with payloads larger than 8 bytes are transmitted
// as a series of CAN frames that must be reassembled.
//
// MultiBuilder maintains a three-level map of in-progress sequences, keyed by:
//   - Source ID (uint8): The NMEA 2000 bus address of the sending device
//   - PGN (uint32): The Parameter Group Number of the message
//   - Sequence ID (uint8, 0-7): A 3-bit identifier allowing up to 8 concurrent sequences
//     for the same source/PGN combination
//
// Each unique (source, PGN, seqId) tuple maps to a sequence that accumulates frames
// until the message is complete. Once complete, the sequence is deleted to free memory.
//
// This structure is instantiated by CANAdapter and delegates frame-level assembly to
// the sequence type.
type MultiBuilder struct {
	// sequences is the three-level map: source ID -> PGN -> sequence ID -> sequence.
	// This hierarchy allows efficient lookup and supports multiple simultaneous transmissions
	// from different sources, different PGNs, and multiple sequence IDs per source/PGN pair.
	sequences map[uint8]map[uint32]map[uint8]*sequence
}

// NewMultiBuilder creates and returns a new MultiBuilder with an initialized (empty)
// sequences map, ready to receive fast-packet frames.
func NewMultiBuilder() *MultiBuilder {
	mBuilder := MultiBuilder{
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

// Add processes an incoming fast-packet frame by extracting its sequence ID and frame
// number, locating (or creating) the appropriate sequence, and adding the frame data.
// If the sequence is now complete (all expected bytes received), the assembled data is
// copied into the packet, the packet is marked Complete, and the sequence is deleted
// from the map to free memory.
//
// Parameters:
//   - p: A pointer to the Packet containing the raw CAN frame data. On return, if the
//     sequence is complete, p.Data will contain the fully assembled payload and
//     p.Complete will be true.
func (m *MultiBuilder) Add(p *pkt.Packet) {
	// Extract the 3-bit sequence ID and 5-bit frame number from the first data byte.
	p.GetSeqFrame()
	// Find or create the sequence for this source/PGN/seqId combination.
	seq := m.SeqFor(p)
	// Add this frame's data to the sequence.
	seq.add(p)
	// Check if the sequence now has all expected data and finalize if so.
	if seq.complete(p) {
		// Sequence is done -- delete it from the map to free memory and prevent stale data.
		delete(m.sequences[p.Info.SourceId][p.Info.PGN], p.SeqId)
	}
}

// SeqFor returns the existing sequence for the given packet's (source, PGN, seqId) tuple,
// or creates a new one if it doesn't exist yet. It lazily initializes intermediate map
// levels as needed.
//
// Parameters:
//   - p: The Packet whose Info.SourceId, Info.PGN, and SeqId identify the target sequence.
//
// Returns a pointer to the sequence for this packet's source/PGN/seqId combination.
func (m *MultiBuilder) SeqFor(p *pkt.Packet) *sequence {
	// Lazily initialize the source-level map if this is the first packet from this source.
	if _, t := m.sequences[p.Info.SourceId]; !t {
		m.sequences[p.Info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	// Lazily initialize the PGN-level map if this is the first packet for this PGN from this source.
	if _, t := m.sequences[p.Info.SourceId][p.Info.PGN]; !t {
		m.sequences[p.Info.SourceId][p.Info.PGN] = make(map[uint8]*sequence)
	}
	// Look up or create the sequence for this specific sequence ID.
	seq := m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId]
	if seq == nil {
		seq = &sequence{}
		m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId] = seq
	}
	return seq
}
