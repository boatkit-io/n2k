package n2k

// Manages the list of sequences used to combine multipacket PGNs
// Instantiated by PGNBuilder
// Uses sequence to do the work

type multiBuilder struct {
	// sequences map[sourceid]map[pgn]map[seqId]Sequence
	sequences map[uint8]map[uint32]map[uint8]*sequence
	current   *packet
}

func newMultiBuilder() *multiBuilder {
	mBuilder := multiBuilder{
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

func (m *multiBuilder) add(p *packet) {
	m.current = p
	seq := m.seqFor(p)
	seq.add(p)
	if seq.complete(p) {
		delete(m.sequences[p.info.SourceId][p.info.PGN], p.seqId)
	}
}

func (m *multiBuilder) seqFor(p *packet) *sequence {
	if _, t := m.sequences[p.info.SourceId]; !t {
		m.sequences[p.info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	if _, t := m.sequences[p.info.SourceId][p.info.PGN]; !t {
		m.sequences[p.info.SourceId][p.info.PGN] = make(map[uint8]*sequence)
	}
	seq := m.sequences[p.info.SourceId][p.info.PGN][p.seqId]
	if seq == nil {
		seq = &sequence{}
		m.sequences[p.info.SourceId][p.info.PGN][p.seqId] = seq
	}
	return seq
}

/*
func (m *MultiBuilder) awaiting(p *Packet) bool {
	if source, exists := m.sequences[p.Info.SourceId]; exists {
		if pgn, exists := source[p.Info.PGN]; exists {
			if seq, exists := pgn[p.SeqId]; exists {
				if seq.contents[p.FrameNum] == nil {
					return true
				}
			}
		}
	}
	return false
}
*/
