package canadapter

import (
	"fmt"
	"log/slog"

	"github.com/open-ships/n2k/pkg/pkt"
)

// MaxFrameNum is the maximum frame number in a multipart NMEA 2000 fast-packet message.
// Frame numbers are encoded as 5 bits (0-31), so a single fast-packet sequence can span
// up to 32 CAN frames. With 6 data bytes in frame 0 and 7 in each subsequent frame,
// this allows payloads up to 6 + (31 * 7) = 223 bytes.
const MaxFrameNum = 31

// sequence defines the data and methods needed to assemble a series of CAN frames into a
// single complete NMEA 2000 fast-packet message.
//
// NMEA 2000 fast-packet protocol overview:
//   - Messages with more than 8 bytes of payload are split across multiple CAN frames.
//   - Each frame's first data byte encodes the sequence ID (bits 7-5) and frame number (bits 4-0).
//   - Frame 0 is special: its second byte contains the total payload length (excluding the
//     header bytes), and it carries 6 bytes of actual data (bytes 2-7).
//   - Continuation frames (1-31) carry 7 bytes of data each (bytes 1-7), with byte 0 used
//     for the sequence ID / frame number header.
//   - The final frame may have unused padding bytes that are trimmed based on the expected
//     length declared in frame 0.
//   - Frame 0 must be received first; other frames may arrive in any order.
//   - The 3-bit sequence ID (0-7) allows up to 8 concurrent sequences for the same PGN/source.
//   - The 5-bit frame number (0-31) identifies each frame's position within the sequence.
type sequence struct {
	// zero holds the initial frame (frame number 0) of this sequence. It must be received
	// before any continuation frames can be accepted. If nil, no frame 0 has been received yet.
	zero *pkt.Packet

	// expected is the total number of payload bytes expected for this message, as declared
	// in byte 1 of frame 0. This is used to determine when all data has been received and
	// to trim padding bytes from the final frame.
	expected uint8

	// received tracks the running total of payload bytes accumulated so far across all frames.
	// Frame 0 contributes 6 bytes; each continuation frame contributes 7 bytes.
	received uint8

	// contents stores the data bytes from each frame, indexed by frame number. This array
	// allows frames to be received out of order (except frame 0 which must come first).
	// A nil entry means that frame has not been received yet. The array size is MaxFrameNum+1
	// (32 slots) to accommodate frame numbers 0-31.
	contents [MaxFrameNum + 1][]uint8 // need arrays since packets can be received out of order
}

// add copies the payload data from a CAN frame into the appropriate slot in the sequence.
//
// For frame 0:
//   - Stores the packet as the sequence's zero reference
//   - Reads byte 1 as the expected total payload length
//   - Copies bytes 2-7 (6 bytes of data) into contents[0]
//
// For continuation frames (frame number > 0):
//   - Requires frame 0 to have been received first (resets if not)
//   - Detects and resets on duplicate frame numbers (assumes a new sequence started)
//   - Copies bytes 1-7 (7 bytes of data) into contents[frameNum]
//
// If a duplicate frame 0 arrives before the previous sequence completes, the old sequence
// is discarded and a new one starts. Similarly, receiving a duplicate continuation frame
// resets the entire sequence under the assumption that a new transmission has started.
//
// Parameters:
//   - p: The Packet containing the raw CAN frame data with sequence/frame header in byte 0.
func (s *sequence) add(p *pkt.Packet) {
	if p.FrameNum == 0 {
		if s.zero != nil { // we've received frame zero for a new sequence before completing the previous one.
			slog.Debug("Fast sequence duplicate frame zero detected. Resetting")
			s.reset() // so we toss the old one and start anew
		}
		s.zero = p
		// Byte 1 of frame 0 contains the total expected payload length (not counting
		// the 2 header bytes of frame 0 or the 1 header byte of continuation frames).
		s.expected = p.Data[1]
		// Bytes 2-7 of frame 0 contain the first 6 bytes of the actual payload.
		s.contents[p.FrameNum] = p.Data[2:]
		s.received += 6
	} else {
		if s.zero == nil { // we've received a subsequent frame before getting the first one
			slog.Debug("fast sequence received subsequent frame before zero frame, resetting",
				"source", p.Info.SourceId, "pgn", p.Info.PGN, "seqId", p.SeqId, "frameNum", p.FrameNum)
			s.reset()
		} else if s.contents[p.FrameNum] != nil { // uh-oh, we've already seen this frame
			// Duplicate frame detected -- likely a new sequence has started with the same
			// sequence ID before the old one completed. Reset to avoid mixing data from
			// two different transmissions.
			slog.Debug("fast sequence received duplicate frame, resetting sequence",
				"source", p.Info.SourceId, "pgn", p.Info.PGN, "seqId", p.SeqId, "frameNum", p.FrameNum)
			s.reset()
		} else {
			// Normal continuation frame: copy bytes 1-7 (7 data bytes, skipping the
			// sequence/frame header in byte 0).
			s.contents[p.FrameNum] = p.Data[1:]
			s.received += 7
		}
	}
}

// complete checks whether all expected payload bytes have been received and, if so,
// assembles the final contiguous data buffer from the per-frame contents.
//
// Assembly process:
//  1. Verify that frame 0 has been received (s.zero != nil).
//  2. Check if received bytes >= expected bytes.
//  3. Concatenate frame contents in order (0, 1, 2, ...), stopping when enough bytes
//     are collected. If any intermediate frame is missing (nil), this indicates a sparse
//     sequence and a parse error is recorded.
//  4. Trim the assembled buffer to exactly s.expected bytes (removes padding from the
//     last CAN frame).
//  5. Store the assembled data in p.Data and set p.Complete = true.
//
// Parameters:
//   - p: The Packet to finalize. On success, p.Data is replaced with the assembled payload
//     and p.Complete is set to true. On sparse-data errors, p.ParseErrors is populated.
//
// Returns true if the sequence is complete (either successfully or with errors), false
// if more frames are still needed.
func (s *sequence) complete(p *pkt.Packet) bool {
	if s.zero != nil {
		if s.received >= s.expected {
			// All expected data has been received. Consolidate the per-frame data arrays
			// into a single contiguous buffer.
			results := make([]uint8, 0)
			for i, d := range s.contents {
				if d == nil { // don't allow sparse nodes
					// A nil entry before we've collected enough bytes means frames arrived
					// out of order with gaps -- this is a malformed sequence.
					p.ParseErrors = append(p.ParseErrors, fmt.Errorf("sparse Data in multi"))
					return true
				} else {
					results = append(results, s.contents[i]...)
					// Once we've collected at least as many bytes as expected, stop
					// processing further frames.
					if len(results) >= int(s.expected) {
						break
					}
				}
			}
			// Trim to exactly the expected length to remove padding bytes from the final
			// CAN frame (which always carries a full 8 bytes even if not all are needed).
			results = results[:s.expected]
			p.Data = results
			p.Complete = true
			return true
		}
	}
	// Not yet complete -- still waiting for more frames.
	p.Complete = false
	return false
}

// reset clears all sequence state to allow reuse of the sequence slot for a new
// transmission. This is called when duplicate frames are detected, indicating that the
// sender has started a new sequence with the same sequence ID before the previous one
// completed (e.g., due to bus errors or missed frames).
func (s *sequence) reset() {
	s.zero = nil
	s.expected = 0
	s.received = 0
	// Clear all stored frame data to prevent stale data from mixing with new frames.
	for i := range s.contents {
		s.contents[i] = nil
	}
}
