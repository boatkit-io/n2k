package canbus

import (
	"log/slog"
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

// TestMapBitRate_ValidRates verifies that every supported CAN bitrate correctly maps
// to its expected protocol byte code. The USB-CAN Analyzer protocol assigns a unique
// byte value for each supported bitrate (e.g., 250000 bps -> 0x05).
func TestMapBitRate_ValidRates(t *testing.T) {
	tests := []struct {
		rate     int
		expected byte
	}{
		{1000000, 0x01},
		{800000, 0x02},
		{500000, 0x03},
		{400000, 0x04},
		{250000, 0x05},
		{200000, 0x06},
		{125000, 0x07},
		{100000, 0x08},
		{50000, 0x09},
		{20000, 0x0a},
		{10000, 0x0b},
		{5000, 0x0c},
	}
	for _, tc := range tests {
		result, err := mapBitRate(tc.rate)
		assert.NoError(t, err, "bitrate %d should be valid", tc.rate)
		assert.Equal(t, tc.expected, result, "bitrate %d should map to 0x%02x", tc.rate, tc.expected)
	}
}

// TestMapBitRate_InvalidRate verifies that unsupported bitrate values return an error.
// The USB-CAN device only supports a fixed set of bitrates, so arbitrary values
// (including zero, negative, and values close to but not matching supported rates) must fail.
func TestMapBitRate_InvalidRate(t *testing.T) {
	invalidRates := []int{0, 1, 9999, 300000, 999999, -1}
	for _, rate := range invalidRates {
		_, err := mapBitRate(rate)
		assert.Error(t, err, "bitrate %d should return error", rate)
	}
}

// TestCalcChecksum verifies the 8-bit additive checksum used in USB-CAN settings frames.
// The checksum sums bytes in a given range and naturally wraps around at 256 (byte overflow).
func TestCalcChecksum(t *testing.T) {
	// Simple known sum: 0x01+0x02+0x03+0x04 = 0x0A
	buf := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	cs := calcChecksum(buf, 1, 4) // sum of 0x01+0x02+0x03+0x04 = 0x0A
	assert.Equal(t, byte(0x0A), cs)

	// All zeros: checksum of all-zero bytes is zero.
	buf = []byte{0x00, 0x00, 0x00, 0x00}
	cs = calcChecksum(buf, 0, 4)
	assert.Equal(t, byte(0x00), cs)

	// Overflow wraps around: 0xFF + 0x01 = 0x100, but as a byte it wraps to 0x00.
	// This verifies the checksum correctly uses natural byte overflow behavior.
	buf = []byte{0xFF, 0x01}
	cs = calcChecksum(buf, 0, 2) // 0xFF + 0x01 = 0x00 (overflow)
	assert.Equal(t, byte(0x00), cs)

	// Single byte: checksum of one byte is just that byte itself.
	buf = []byte{0x42}
	cs = calcChecksum(buf, 0, 1)
	assert.Equal(t, byte(0x42), cs)
}

// newTestChannel creates a USBCANChannel for testing with a no-op logger and a FrameHandler
// that appends received frames to a slice. Returns the channel and a pointer to the received
// frames slice so tests can inspect what the parser dispatched.
func newTestChannel() (*USBCANChannel, *[]can.Frame) {
	received := &[]can.Frame{}
	ch := NewUSBCANChannel(slog.Default(), USBCANChannelOptions{
		FrameHandler: func(f can.Frame) {
			*received = append(*received, f)
		},
	})
	return ch.(*USBCANChannel), received
}

// TestParseFrames_EmptyBuffer verifies that an empty buffer produces no frames and no errors.
// This is the base case for the parser -- it should be a no-op when there's nothing to parse.
func TestParseFrames_EmptyBuffer(t *testing.T) {
	usbChan, received := newTestChannel()
	buf := []byte{}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 0, len(buf))
}

// TestParseFrames_StandardDataFrame verifies parsing of a complete standard (11-bit ID) data frame.
//
// Standard data frame wire format:
//   - 0xAA: start-of-frame
//   - 0xC0 | dataLen: info byte (bits[7:6]=0b11 for data frame, bits[3:0]=data length)
//   - 2 bytes: CAN ID in little-endian (low byte first)
//   - N bytes: data payload
//   - 0x55: end-of-frame
func TestParseFrames_StandardDataFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Standard data frame: 0xAA, (0xC0 | dataLen), id_lo, id_hi, data..., 0x55
	// dataLen = 3, id = 0x0102 (little-endian: lo=0x02, hi=0x01)
	dataLen := byte(3)
	buf := []byte{
		0xAA,
		0xC0 | dataLen, // standard frame, 3 bytes of data
		0x02, 0x01,     // ID = 0x0102 (little-endian)
		0xAA, 0xBB, 0xCC, // 3 data bytes
		0x55, // end byte
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*received))
	f := (*received)[0]
	assert.Equal(t, uint32(0x0102), f.ID)
	assert.Equal(t, dataLen, f.Length)
	assert.Equal(t, byte(0xAA), f.Data[0])
	assert.Equal(t, byte(0xBB), f.Data[1])
	assert.Equal(t, byte(0xCC), f.Data[2])
	assert.Equal(t, 0, len(buf), "buffer should be consumed")
}

// TestParseFrames_ExtendedDataFrame verifies parsing of a complete extended (29-bit ID) data frame.
//
// Extended data frame wire format:
//   - 0xAA: start-of-frame
//   - 0xE0 | dataLen: info byte (bits[7:6]=0b11, bit[5]=1 for extended, bits[3:0]=data length)
//   - 4 bytes: CAN ID in little-endian
//   - N bytes: data payload
//   - 0x55: end-of-frame
//
// NMEA 2000 always uses extended 29-bit CAN IDs, so this is the most common frame type
// for marine network traffic.
func TestParseFrames_ExtendedDataFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Extended data frame: 0xAA, (0xE0 | dataLen), id bytes (4 LE), data..., 0x55
	dataLen := byte(2)
	buf := []byte{
		0xAA,
		0xE0 | dataLen,             // extended frame, 2 bytes of data
		0x83, 0x01, 0xF2, 0x09,     // ID = 0x09F20183 (little-endian)
		0xDE, 0xAD,                 // 2 data bytes
		0x55,                       // end byte
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*received))
	f := (*received)[0]
	assert.Equal(t, uint32(0x09F20183), f.ID)
	assert.Equal(t, dataLen, f.Length)
	assert.Equal(t, byte(0xDE), f.Data[0])
	assert.Equal(t, byte(0xAD), f.Data[1])
	assert.Equal(t, 0, len(buf))
}

// TestParseFrames_ExtendedDataFrame_8Bytes verifies parsing an extended frame with the maximum
// CAN payload size of 8 bytes. CAN 2.0 frames can carry at most 8 bytes of data.
func TestParseFrames_ExtendedDataFrame_8Bytes(t *testing.T) {
	usbChan, received := newTestChannel()

	// Extended frame with 8 data bytes
	dataLen := byte(8)
	buf := []byte{
		0xAA,
		0xE0 | dataLen,
		0x83, 0x01, 0xF2, 0x09, // ID
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // 8 data bytes
		0x55,
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*received))
	f := (*received)[0]
	assert.Equal(t, uint32(0x09F20183), f.ID)
	assert.Equal(t, dataLen, f.Length)
	for i := 0; i < 8; i++ {
		assert.Equal(t, byte(i+1), f.Data[i])
	}
}

// TestParseFrames_CommandFrame verifies that 20-byte command frames (0xAA 0x55 + 18 bytes)
// are consumed from the buffer but do NOT produce any can.Frame output.
// Command frames are device responses/status messages, not CAN bus traffic.
func TestParseFrames_CommandFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Command frame: 0xAA, 0x55, followed by 18 more bytes (20 total)
	buf := make([]byte, 20)
	buf[0] = 0xAA
	buf[1] = 0x55
	for i := 2; i < 20; i++ {
		buf[i] = byte(i)
	}

	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received), "Command frames should not produce can.Frame outputs")
	assert.Equal(t, 0, len(buf), "Buffer should be consumed after command frame")
}

// TestParseFrames_ErrorRecovery_SkipToNextAA verifies that the parser can recover from
// garbage bytes at the start of the buffer by scanning forward to find the next 0xAA
// start-of-frame marker and successfully parsing the valid frame that follows.
func TestParseFrames_ErrorRecovery_SkipToNextAA(t *testing.T) {
	usbChan, received := newTestChannel()

	// Garbage bytes followed by a valid standard frame
	dataLen := byte(1)
	buf := []byte{
		0x00, 0x01, 0x02, // garbage (no 0xAA here)
		0xAA,
		0xC0 | dataLen, // standard frame, 1 byte of data
		0x10, 0x00,     // ID = 0x0010
		0xFF,           // 1 data byte
		0x55,           // end byte
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*received))
	assert.Equal(t, uint32(0x0010), (*received)[0].ID)
	assert.Equal(t, byte(0xFF), (*received)[0].Data[0])
}

// TestParseFrames_ErrorRecovery_NoAA verifies that when the buffer contains no 0xAA byte
// at all, the entire buffer is discarded. This handles completely corrupted data where
// there is no valid frame start marker to resynchronize on.
func TestParseFrames_ErrorRecovery_NoAA(t *testing.T) {
	usbChan, received := newTestChannel()

	// Buffer with no 0xAA at all - should clear the buffer
	buf := []byte{0x01, 0x02, 0x03, 0x04}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 0, len(buf), "Buffer should be cleared when no 0xAA found")
}

// TestParseFrames_BadEndByte verifies that a data frame with an incorrect end byte
// (anything other than 0x55) is rejected and the buffer is cleared. The parser cannot
// reliably determine frame boundaries after a bad end byte, so it discards everything.
func TestParseFrames_BadEndByte(t *testing.T) {
	usbChan, received := newTestChannel()

	// Standard data frame without proper 0x55 terminator
	dataLen := byte(2)
	buf := []byte{
		0xAA,
		0xC0 | dataLen,
		0x10, 0x00,     // ID
		0xAA, 0xBB,     // data
		0x99,           // bad end byte (should be 0x55)
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received), "Bad end byte should not produce a frame")
	assert.Equal(t, 0, len(buf), "Buffer should be cleared on bad end byte")
}

// TestParseFrames_IncompleteFrame_TooShort verifies that when only the start byte (0xAA)
// is in the buffer (not enough to determine frame type), the parser leaves the data in
// the buffer and returns without error, waiting for more bytes to arrive.
func TestParseFrames_IncompleteFrame_TooShort(t *testing.T) {
	usbChan, received := newTestChannel()

	// Only the start byte - needs more data
	buf := []byte{0xAA}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 1, len(buf), "Incomplete frame should remain in buffer")
}

// TestParseFrames_IncompleteStandardFrame verifies that a standard frame header with
// insufficient data bytes is left in the buffer for later completion. The parser
// knows how many bytes to expect from the info byte but won't consume until all arrive.
func TestParseFrames_IncompleteStandardFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Standard frame header but missing data bytes and end byte
	buf := []byte{
		0xAA,
		0xC0 | 4,   // standard frame, 4 data bytes expected
		0x10, 0x00, // ID
		// missing 4 data bytes and 0x55
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 4, len(buf), "Incomplete frame should remain in buffer")
}

// TestParseFrames_IncompleteExtendedFrame verifies the same incomplete-frame behavior for
// extended frames, which have a 4-byte ID field instead of 2. The parser should retain
// the partial data and wait for the rest of the frame to arrive.
func TestParseFrames_IncompleteExtendedFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Extended frame header but truncated
	buf := []byte{
		0xAA,
		0xE0 | 4,                   // extended frame, 4 data bytes
		0x83, 0x01, 0xF2, 0x09,     // ID
		// missing 4 data bytes and 0x55
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 6, len(buf), "Incomplete frame should remain in buffer")
}

// TestParseFrames_IncompleteCommandFrame verifies that an incomplete command frame
// (fewer than 20 bytes starting with 0xAA 0x55) is left in the buffer.
// Command frames are always exactly 20 bytes long.
func TestParseFrames_IncompleteCommandFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Command frame with only 10 of 20 bytes
	buf := make([]byte, 10)
	buf[0] = 0xAA
	buf[1] = 0x55
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 10, len(buf), "Incomplete command frame should remain in buffer")
}

// TestParseFrames_MultipleFrames verifies that when the buffer contains multiple
// complete frames back-to-back, all of them are parsed and dispatched in order.
// This simulates the common case where a single serial read returns multiple frames.
func TestParseFrames_MultipleFrames(t *testing.T) {
	usbChan, received := newTestChannel()

	// Two standard frames back to back
	buf := []byte{
		// Frame 1: ID=0x0001, 1 data byte (0x11)
		0xAA, 0xC0 | 1, 0x01, 0x00, 0x11, 0x55,
		// Frame 2: ID=0x0002, 2 data bytes (0x22, 0x33)
		0xAA, 0xC0 | 2, 0x02, 0x00, 0x22, 0x33, 0x55,
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(*received))
	assert.Equal(t, uint32(0x0001), (*received)[0].ID)
	assert.Equal(t, uint32(0x0002), (*received)[1].ID)
	assert.Equal(t, byte(0x11), (*received)[0].Data[0])
	assert.Equal(t, byte(0x22), (*received)[1].Data[0])
	assert.Equal(t, byte(0x33), (*received)[1].Data[1])
	assert.Equal(t, 0, len(buf))
}

// TestParseFrames_UnknownFrameType verifies that a frame with an unrecognized type
// (second byte doesn't match command 0x55 and bits[7:6] are not 0b11 for data)
// causes the entire buffer to be discarded. The parser cannot interpret the frame
// structure, so it clears the buffer to avoid infinite loops.
func TestParseFrames_UnknownFrameType(t *testing.T) {
	usbChan, received := newTestChannel()

	// A frame starting with 0xAA but second byte doesn't match any known type
	// (not 0x55 command, and top 2 bits of byte[1] are not 0b11)
	buf := []byte{0xAA, 0x00, 0x01, 0x02}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 0, len(buf), "Unknown frame type should clear buffer")
}

// TestParseFrames_ZeroLengthDataFrame verifies parsing a valid data frame with zero
// data bytes. While unusual, CAN frames with no data payload are valid -- the frame
// still has an ID and the end byte marker. This can occur with RTR (remote request) frames.
func TestParseFrames_ZeroLengthDataFrame(t *testing.T) {
	usbChan, received := newTestChannel()

	// Standard frame with 0 data bytes
	buf := []byte{
		0xAA,
		0xC0, // standard frame, 0 data bytes
		0x10, 0x00, // ID
		0x55, // end byte
	}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(*received))
	assert.Equal(t, uint32(0x0010), (*received)[0].ID)
	assert.Equal(t, byte(0), (*received)[0].Length)
	assert.Equal(t, 0, len(buf))
}
