package canadapter

import (
	"github.com/sirupsen/logrus"

	"github.com/boatkit-io/n2k/pkg/pkt"
)

// MultiBuilder assembles a sequence of packets into a comple Packet.
// Manages the list of sequences used to combine multipacket PGNs
// Instantiated by PGNBuilder
// Uses sequence to do the work
// we track sequences separately for each nmea source
// sequence ids are 0-7, so each source|PGN can have 8 sequences in simultaneous transmission
// sequences map[sourceid]map[pgn]map[SeqId]sequence
type MultiBuilder struct {
	log       *logrus.Logger
	sequences map[uint8]map[uint32]map[uint8]*sequence
}

// NewMultiBuilder creates a new instance.
func NewMultiBuilder(log *logrus.Logger) *MultiBuilder {
	mBuilder := MultiBuilder{
		log:       log,
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

// Add method adds a packet to a (new or existing) sequence.
// if the sequence (and resulting packet) is now complete, delete the sequence.
func (m *MultiBuilder) Add(p *pkt.Packet) {
	p.GetSeqFrame()
	seq := m.SeqFor(p)
	seq.add(p)
	if seq.complete(p) {
		delete(m.sequences[p.Info.SourceId][p.Info.PGN], p.SeqId)
	}
}

// SeqFor method returns the sequence for the specified packet, creating it it needed.
func (m *MultiBuilder) SeqFor(p *pkt.Packet) *sequence {
	if _, t := m.sequences[p.Info.SourceId]; !t {
		m.sequences[p.Info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	if _, t := m.sequences[p.Info.SourceId][p.Info.PGN]; !t {
		m.sequences[p.Info.SourceId][p.Info.PGN] = make(map[uint8]*sequence)
	}
	seq := m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId]
	if seq == nil {
		seq = &sequence{
			log: m.log,
		}
		m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId] = seq
	}
	return seq
}
