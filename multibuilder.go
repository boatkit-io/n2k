package n2k

import (
	"fmt"
	"time"
)

// Calculated with math, but reference:
// https://copperhilltech.com/blog/what-is-the-difference-between-sae-j1939-and-nmea-2000/
const (
	MaxBytesInFastPacket = 223
	MaxFrameNum          = 31
)

type sequence struct {
	started  time.Time
	main     *Packet // packet 0 of sequence
	expected uint8
	received uint8
	contents [MaxFrameNum][]uint8 // need arrays since packets can be received out of order
}
type MultiBuilder struct {
	// sequences map[sourceid]map[pgn]map[seqId]sequence
	sequences map[uint8]map[uint32]map[uint8]*sequence
}

func NewMultiBuilder() *MultiBuilder {
	mBuilder := MultiBuilder{
		sequences: make(map[uint8]map[uint32]map[uint8]*sequence),
	}
	return &mBuilder
}

func (m *MultiBuilder) Add(p *Packet) {
	if _, t := m.sequences[p.Info.SourceId]; !t {
		m.sequences[p.Info.SourceId] = make(map[uint32]map[uint8]*sequence)
	}
	if _, t := m.sequences[p.Info.SourceId][p.Info.PGN]; !t {
		m.sequences[p.Info.SourceId][p.Info.PGN] = make(map[uint8]*sequence)
	}
	seq := m.sequences[p.Info.SourceId][p.Info.PGN][p.SeqId]
	if p.FrameNum == 0 {
		seq.main = p
		seq.expected = p.Data[1]
		seq.contents[p.FrameNum] = p.Data[2:]
		seq.received += 6
	} else {
		seq.contents[p.FrameNum] = p.Data[1:]
		seq.received += 7
	}
	if seq.main != nil {
		if seq.received >= seq.expected {
			//  consolidate data
			results := make([]uint8, 0)
			for i, d := range seq.contents {
				if d == nil { // don't allow sparse nodes
					p.ParseErrors = append(p.ParseErrors, fmt.Errorf("sparse data in multi"))
					delete(m.sequences[p.Info.SourceId][p.Info.PGN], p.SeqId)
					return
				} else {
					results = append(results, seq.contents[i]...)
					if len(results) >= int(seq.expected) {
						break
					}
				}
			}
			results = results[:seq.expected]
			seq.main.Data = results
			p = seq.main // sets b.current to complete packet
			p.Complete = true
			delete(m.sequences[p.Info.SourceId][p.Info.PGN], p.SeqId)
		}
	}
}
