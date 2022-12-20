package n2k

import (
	"fmt"
	"time"
)

// Calculated with math, but reference:
// https://copperhilltech.com/blog/what-is-the-difference-between-sae-j1939-and-nmea-2000/
const (
	//	maxBytesInFastPacket = 223
	MaxFrameNum = 31
)

type sequence struct {
	started  time.Time
	zero     *packet // packet 0 of sequence
	expected uint8
	received uint8
	contents [MaxFrameNum + 1][]uint8 // need arrays since packets can be received out of order
}

func (s *sequence) add(p *packet) {
	if p.frameNum == 0 {
		s.started = time.Now()
		s.zero = p
		s.expected = p.data[1]
		s.contents[p.frameNum] = p.data[2:]
		s.received += 6
	} else {
		s.contents[p.frameNum] = p.data[1:]
		s.received += 7
	}
}

func (s *sequence) complete(p *packet) bool {
	if s.zero != nil {
		if s.received >= s.expected {
			//  consolidate data
			results := make([]uint8, 0)
			for i, d := range s.contents {
				if d == nil { // don't allow sparse nodes
					p.parseErrors = append(p.parseErrors, fmt.Errorf("sparse data in multi"))
					return true
				} else {
					results = append(results, s.contents[i]...)
					if len(results) >= int(s.expected) {
						break
					}
				}
			}
			results = results[:s.expected]
			p.data = results
			p.complete = true
			p.getManCode()
			return true
		}
	}
	return false
}
