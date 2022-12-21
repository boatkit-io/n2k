package n2k

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

// NMEA 2000 sends packets with >8 bytes of data in multiple packets
// A sequence has a common sequence ID and a frame number between 0 and 31 inclusive
// These are encoded in the first data byte
// Frame 0 has the total number of data bytes (excluding bytes 0 and 1 of frame 0 and byte
// 0 of subsequent (continuation) frames)
// Since every can frame contains 8 bytes, the final frame might have unused bytes
// that are trimmed when constructing the complete packet.
// The sequence ID is 3 bits, so 0-7
// The frame number is 5 bits, so 0-31

// Calculated with math, but reference:
// https://copperhilltech.com/blog/what-is-the-difference-between-sae-j1939-and-nmea-2000/
const (
	//	maxBytesInFastPacket = 223
	MaxFrameNum = 31
)

// sequence frames can be received in any order
// we track the time started to allow for releasing incomplete sequences (TBD)
type sequence struct {
	started  time.Time
	log      *logrus.Logger
	zero     *packet // packet 0 of sequence
	expected uint8
	received uint8
	contents [MaxFrameNum + 1][]uint8 // need arrays since packets can be received out of order
}

func (s *sequence) add(p *packet) {
	if s.contents[p.frameNum] != nil { // uh-oh, we've already seen this frame
		s.log.Warnf("received duplicate frame: %d %d %d, resetting sequence\n", p.info.SourceId, p.info.PGN, p.frameNum)
		s.reset()
	}
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
			return true
		}
	}
	return false
}

// clear the sequence and try again
func (s *sequence) reset() {
	s.started = time.Now()
	s.zero = nil
	s.expected = 0
	s.received = 0
	for i := range s.contents {
		s.contents[i] = nil
	}
}
