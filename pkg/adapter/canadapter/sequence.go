package canadapter

import (
	"fmt"
	"time"

	"github.com/boatkit-io/n2k/pkg/pkt"
	"github.com/sirupsen/logrus"
)

// MaxFrameNum is the maximum frame number in a multipart NMEA message.
const MaxFrameNum = 31

// sequence defines data and methods to combine a sequence of packets into a single complete packet.
// NMEA 2000 sends messages with >8 bytes of Data in multiple frames.
// An adapter outputs a fully assembled, complete message.
// Multipart message frames have a common sequence ID and a frame number between 0 and 31 inclusive
// These are encoded in the first Data byte.
// the second byte of frame 0 has the total number of Data bytes (excluding bytes 0 and 1 of frame 0 and byte
// 0 of subsequent (continuation) frames).
// Since every can frame contains 8 bytes, the final frame might have unused bytes
// that are trimmed when constructing the complete packet.
// The sequence ID is 3 bits, so 0-7.
// The frame number is 5 bits, so 0-31.
// Sequence frames can be received in any order.
// we track the time started to allow for releasing incomplete sequences (TBD).
type sequence struct {
	started  int64
	recent   int64
	log      *logrus.Logger
	zero     *pkt.Packet // packet 0 of sequence
	expected uint8
	received uint8
	contents [MaxFrameNum + 1][]uint8 // need arrays since packets can be received out of order
}

// add method copies the frame's data into the sequence.
// if it's frame 0 it sets sequence info (time, expected length) and copies out 6 bytes of data.
// else it copies 7 bytes of data.
// it warns if a packet in the sequence has been received twice and resets the sequence.
func (s *sequence) add(p *pkt.Packet) {
	s.recent = time.Now().Unix()
	if p.FrameNum == 0 {
		if s.zero != nil { // we've received frame zero for a new sequence before completing the previous one.
			s.log.Warn("duplicate zero detected. Resetting")
			s.reset() // so we toss the old one and start anew
		}
		s.started = time.Now().Unix()
		s.zero = p
		s.expected = p.Data[1]
		s.contents[p.FrameNum] = p.Data[2:]
		s.received += 6
	} else {
		if s.zero == nil { // we've received a subsequent frame before getting the first one
			s.log.Warn("received subsequent frame before zeroth. Resetting")
			s.reset()
		} else if s.contents[p.FrameNum] != nil { // uh-oh, we've already seen this frame
			s.log.Warnf("received duplicate frame. Source: %d PGN: %d Sequence #: %d FrameNum #: %d, resetting sequence", p.Info.SourceId, p.Info.PGN, p.SeqId, p.FrameNum)
			s.log.Warnf("expected:%v", s.expected)
			s.log.Warnf("received:%d", s.calcReceived())
			s.reset()
		} else {
			s.contents[p.FrameNum] = p.Data[1:]
			s.received += 7
		}
	}
}

// complete method tests if all of the expected data has been received.
// if so it assures packets received are consecutive, copies the complete data into the current packet
// and marks it complete.
func (s *sequence) complete(p *pkt.Packet) bool {
	if s.zero != nil {
		if s.received >= s.expected {
			//  consolidate Data
			results := make([]uint8, 0)
			for i, d := range s.contents {
				if d == nil { // don't allow sparse nodes
					p.ParseErrors = append(p.ParseErrors, fmt.Errorf("sparse Data in multi"))
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
			p.Complete = true
			return true
		}
	}
	p.Complete = false
	return false
}

// reset method clears the sequence to try again.
// Called if we receive a duplicate packet, assuming it belongs to a new sequence.
func (s *sequence) reset() {
	s.started = time.Now().Unix()
	s.zero = nil
	s.expected = 0
	s.received = 0
	for i := range s.contents {
		s.contents[i] = nil
	}
}

func (s *sequence) calcReceived() int {

	received := 0
	for i := range s.contents {

		if s.contents[i] != nil {
			received += len(s.contents[i])
		} else { // no sparse nodes
			break
		}
	}
	return received

}
