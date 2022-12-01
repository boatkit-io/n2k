package n2k

import (
	"fmt"
	"time"
)

type sequence struct {
	started  time.Time
	zero     *Packet // packet 0 of sequence
	expected uint8
	received uint8
	contents [MaxFrameNum][]uint8 // need arrays since packets can be received out of order
}

func (s *sequence) add(p *Packet) {
	if p.FrameNum == 0 {
		s.started = time.Now()
		s.zero = p
		s.expected = p.Data[1]
		s.contents[p.FrameNum] = p.Data[2:]
		s.received += 6
	} else {
		s.contents[p.FrameNum] = p.Data[1:]
		s.received += 7
	}
}

func (s *sequence) complete(p *Packet) bool {
	if s.zero != nil {
		if s.received >= s.expected {
			//  consolidate data
			results := make([]uint8, 0)
			for i, d := range s.contents {
				if d == nil { // don't allow sparse nodes
					p.ParseErrors = append(p.ParseErrors, fmt.Errorf("sparse data in multi"))
					return true
				} else {
					results = append(results, s.contents[i]...)
					if len(results) >= int(s.expected) {
						break
					}
				}
			}
			results = results[:s.expected]
			p.Data = results
			p.Decoders = s.zero.Decoders // sets b.current to complete packet
			p.Complete = true
			return true
		}
	}
	return false
}
