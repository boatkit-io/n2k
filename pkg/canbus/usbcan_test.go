package canbus

import (
	"log/slog"
	"testing"

	"github.com/brutella/can"
	"github.com/stretchr/testify/assert"
)

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

func TestMapBitRate_InvalidRate(t *testing.T) {
	invalidRates := []int{0, 1, 9999, 300000, 999999, -1}
	for _, rate := range invalidRates {
		_, err := mapBitRate(rate)
		assert.Error(t, err, "bitrate %d should return error", rate)
	}
}

func TestCalcChecksum(t *testing.T) {
	// Simple known sum
	buf := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	cs := calcChecksum(buf, 1, 4) // sum of 0x01+0x02+0x03+0x04 = 0x0A
	assert.Equal(t, byte(0x0A), cs)

	// All zeros
	buf = []byte{0x00, 0x00, 0x00, 0x00}
	cs = calcChecksum(buf, 0, 4)
	assert.Equal(t, byte(0x00), cs)

	// Overflow wraps around (byte arithmetic)
	buf = []byte{0xFF, 0x01}
	cs = calcChecksum(buf, 0, 2) // 0xFF + 0x01 = 0x00 (overflow)
	assert.Equal(t, byte(0x00), cs)

	// Single byte
	buf = []byte{0x42}
	cs = calcChecksum(buf, 0, 1)
	assert.Equal(t, byte(0x42), cs)
}

func newTestChannel() (*USBCANChannel, *[]can.Frame) {
	received := &[]can.Frame{}
	ch := NewUSBCANChannel(slog.Default(), USBCANChannelOptions{
		FrameHandler: func(f can.Frame) {
			*received = append(*received, f)
		},
	})
	return ch.(*USBCANChannel), received
}

func TestParseFrames_EmptyBuffer(t *testing.T) {
	usbChan, received := newTestChannel()
	buf := []byte{}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 0, len(buf))
}

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

func TestParseFrames_ErrorRecovery_SkipToNextAA(t *testing.T) {
	usbChan, received := newTestChannel()

	// Garbage bytes followed by a valid standard frame
	dataLen := byte(1)
	buf := []byte{
		0x00, 0x01, 0x02, // garbage
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

func TestParseFrames_ErrorRecovery_NoAA(t *testing.T) {
	usbChan, received := newTestChannel()

	// Buffer with no 0xAA at all - should clear the buffer
	buf := []byte{0x01, 0x02, 0x03, 0x04}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 0, len(buf), "Buffer should be cleared when no 0xAA found")
}

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

func TestParseFrames_IncompleteFrame_TooShort(t *testing.T) {
	usbChan, received := newTestChannel()

	// Only the start byte - needs more data
	buf := []byte{0xAA}
	err := usbChan.parseFrames(&buf)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(*received))
	assert.Equal(t, 1, len(buf), "Incomplete frame should remain in buffer")
}

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

func TestParseFrames_MultipleFrames(t *testing.T) {
	usbChan, received := newTestChannel()

	// Two standard frames back to back
	buf := []byte{
		// Frame 1: ID=0x0001, 1 data byte
		0xAA, 0xC0 | 1, 0x01, 0x00, 0x11, 0x55,
		// Frame 2: ID=0x0002, 2 data bytes
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
