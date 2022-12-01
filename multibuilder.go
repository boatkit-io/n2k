package n2k

// Manages the list of sequences used to combine multipacket PGNs
// Instantiated by PGNBuilder
// Uses sequence to do the work

// Calculated with math, but reference:
// https://copperhilltech.com/blog/what-is-the-difference-between-sae-j1939-and-nmea-2000/
const (
	MaxBytesInFastPacket = 223
	MaxFrameNum          = 31
)

type MultiBuilder struct {
	// sequences map[sourceid]map[pgn]map[seqId]Sequence
	sequences map[uint8]map[uint32]map[uint8]*sequence
	current   *Packet
}

func NewMultiBuilder() *MultiBuilder {
	mBuilder := MultiBuilder{
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

func (m *MultiBuilder) Add(p *Packet) {
	m.current = p
	seq := m.seqFor(p)
	seq.add(p)
	if seq.complete(p) {
		delete(m.sequences[p.Info.SourceId][p.Info.PGN], p.SeqId)
	}
}

func (m *MultiBuilder) seqFor(p *Packet) *sequence {
	if _, t := m.sequences[p.Info.SourceId]; !t {
		m.sequences[p.Info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	if _, t := m.sequences[p.Info.SourceId][p.Info.PGN]; !t {
		m.sequences[p.Info.SourceId][p.Info.PGN] = make(map[uint8]*sequence)
	}
	seq := m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId]
	if seq == nil {
		seq = &sequence{}
		m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId] = seq
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