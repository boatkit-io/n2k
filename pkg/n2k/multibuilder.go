package n2k

import (
	"github.com/sirupsen/logrus"
)

// Manages the list of sequences used to combine multipacket PGNs
// Instantiated by PGNBuilder
// Uses sequence to do the work

// we track sequences separately for each nmea source
// sequence ids are 0-7, so each source can have 8 sequences in simultaneous transmission
// sequences map[sourceid]map[pgn]map[seqId]sequence
type multiBuilder struct {
	log       *logrus.Logger
	sequences map[uint8]map[uint32]map[uint8]*sequence
}

// create an instance
func newMultiBuilder(log *logrus.Logger) *multiBuilder {
	mBuilder := multiBuilder{
		log:       log,
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

// select (creating if needed) the sequence for this source id and pgn
// add the packet to the sequence
// if the packet is now complete, delete the sequence
func (m *multiBuilder) add(p *packet) {
	seq := m.seqFor(p)
	seq.add(p)
	if seq.complete(p) {
		delete(m.sequences[p.info.SourceId][p.info.PGN], p.seqId)
	}
}

// walk down the maps of source id, pgn, and seqid, creating if needed
func (m *multiBuilder) seqFor(p *packet) *sequence {
	if _, t := m.sequences[p.info.SourceId]; !t {
		m.sequences[p.info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	if _, t := m.sequences[p.info.SourceId][p.info.PGN]; !t {
		m.sequences[p.info.SourceId][p.info.PGN] = make(map[uint8]*sequence)
	}
	seq := m.sequences[p.info.SourceId][p.info.PGN][p.seqId]
	if seq == nil {
		seq = &sequence{
			log: m.log,
		}
		m.sequences[p.info.SourceId][p.info.PGN][p.seqId] = seq
	}
	return seq
}
