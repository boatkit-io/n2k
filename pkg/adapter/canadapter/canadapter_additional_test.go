package canadapter

import (
	"testing"

	"github.com/brutella/can"
	"github.com/open-ships/n2k/pkg/pkt"
	"github.com/stretchr/testify/assert"
)

// mockHandler is a test double that captures all packets forwarded by a CANAdapter,
// allowing tests to inspect the output of the processing pipeline.
type mockHandler struct {
	// packets accumulates all packets received via HandlePacket, in order.
	packets []pkt.Packet
}

// HandlePacket records the packet for later test verification.
func (m *mockHandler) HandlePacket(p pkt.Packet) {
	m.packets = append(m.packets, p)
}

// TestNewCANAdapter verifies that NewCANAdapter returns a properly initialized adapter
// with a non-nil MultiBuilder ready for fast-packet assembly.
func TestNewCANAdapter(t *testing.T) {
	a := NewCANAdapter()
	assert.NotNil(t, a)
	assert.NotNil(t, a.multi)
}

// TestSetOutputAndHandleSingleFrameMessage verifies the complete flow for a single-frame
// (non-fast) PGN: setting the output handler, processing a CAN frame, and receiving
// a complete packet. PGN 127501 (Binary Switch Bank Status) is used as a single-frame
// test case. The test confirms:
//   - Exactly one packet is forwarded to the handler
//   - The packet is marked Complete (single-frame messages are immediately complete)
//   - PGN and source ID are correctly extracted from the CAN ID
func TestSetOutputAndHandleSingleFrameMessage(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// PGN 127501 is a single-frame (non-fast) PGN
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)

	a.HandleMessage(&f)

	assert.Equal(t, 1, len(h.packets), "Handler should have received exactly one packet")
	p := h.packets[0]
	assert.True(t, p.Complete, "Single-frame packet should be complete")
	assert.Equal(t, uint32(127501), p.Info.PGN)
	assert.Equal(t, uint8(224), p.Info.SourceId)
}

// TestHandleMessageWithNonCanFrame verifies that HandleMessage gracefully handles
// receiving a message type that is not *can.Frame. It should log a warning but not panic
// or forward anything to the handler. This tests the type-switch default case.
func TestHandleMessageWithNonCanFrame(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// Send a non-can.Frame message type; should just log a warning, no panic.
	type fakeMessage struct{}
	a.HandleMessage(fakeMessage{})

	assert.Equal(t, 0, len(h.packets), "Handler should not receive anything for non-can.Frame message")
}

// TestHandleMessageWithoutHandler verifies that processing a message when no handler is
// set does not panic. This tests the nil-guard in packetReady.
func TestHandleMessageWithoutHandler(t *testing.T) {
	a := NewCANAdapter()
	// No handler set - should not panic.
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)
	assert.NotPanics(t, func() {
		a.HandleMessage(&f)
	})
}

// TestHandleMessageMultipleNonFastPackets verifies that the adapter correctly handles
// multiple sequential single-frame packets, including packets from different sources.
// All three packets should be received complete and in order.
func TestHandleMessageMultipleNonFastPackets(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// Send multiple single-frame packets: two from source 224, one from source 100.
	raws := []string{
		"2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff",
		"2023-01-21T00:04:18Z,3,127501,224,0,8,00,04,c0,ff,ff,ff,ff,ff",
		"2023-01-21T00:04:19Z,3,127501,100,0,8,00,05,c0,ff,ff,ff,ff,ff",
	}
	for _, raw := range raws {
		f := CanFrameFromRaw(raw)
		a.HandleMessage(&f)
	}

	assert.Equal(t, 3, len(h.packets), "Handler should have received 3 packets")
	for _, p := range h.packets {
		assert.True(t, p.Complete)
	}
}

// TestHandleFastPacketMessage verifies multi-frame fast-packet assembly through the
// full CANAdapter pipeline. PGN 130820 is a fast-packet PGN. This test sends 5 frames
// with sequence ID 3 (0x60 = seqId 3, frame 0) that together carry a 32-byte payload.
//
// The test verifies:
//   - Only one complete packet is emitted (intermediate frames are buffered)
//   - The packet is marked Complete
//   - The assembled data is 32 bytes (matching the expected length in frame 0's byte 1)
//
// Frame data breakdown:
//   - Frame 0 (0x60): seqId=3, frameNum=0, expected=0x20 (32 bytes), 6 data bytes
//   - Frame 1 (0x61): seqId=3, frameNum=1, 7 data bytes
//   - Frame 2 (0x62): seqId=3, frameNum=2, 7 data bytes
//   - Frame 3 (0x63): seqId=3, frameNum=3, 7 data bytes
//   - Frame 4 (0x64): seqId=3, frameNum=4, 7 data bytes (5 used + 2 padding)
func TestHandleFastPacketMessage(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// PGN 130820 is a fast packet PGN. Build a multi-frame sequence.
	pInfo := NewPacketInfo(&can.Frame{ID: CanIdFromData(130820, 10, 1, 0), Length: 8})
	_ = pInfo

	// Use full multi-frame sequence from the existing test.
	frames := []struct {
		data [8]byte
	}{
		{[8]byte{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}}, // Frame 0: seqId=3, len=32
		{[8]byte{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}}, // Frame 1
		{[8]byte{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}}, // Frame 2
		{[8]byte{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}}, // Frame 3
		{[8]byte{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}}, // Frame 4 (final)
	}

	for _, fd := range frames {
		f := can.Frame{
			ID:     CanIdFromData(130820, 10, 1, 0),
			Length: 8,
			Data:   fd.data,
		}
		a.HandleMessage(&f)
	}

	// Only the last frame should complete the packet, so we get one packet.
	assert.Equal(t, 1, len(h.packets), "Handler should receive one completed fast packet")
	assert.True(t, h.packets[0].Complete)
	assert.Equal(t, 32, len(h.packets[0].Data))
}

// TestHandleFastPacketSingleFrame verifies that a fast-packet PGN whose entire payload
// fits within a single CAN frame is handled correctly. Frame 0 declares a payload length
// of 5 bytes, and all 5 fit in the 6 available data bytes of frame 0, so the packet is
// immediately complete without needing continuation frames.
//
// Data byte 0 = 0xa0: seqId=5 (bits 7-5), frameNum=0 (bits 4-0).
// Data byte 1 = 5: expected payload length (5 bytes).
// Data bytes 2-7: the 5 payload bytes plus 1 unused byte.
func TestHandleFastPacketSingleFrame(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// A fast packet that fits in a single frame.
	f := can.Frame{
		ID:     CanIdFromData(130820, 10, 1, 0),
		Length: 8,
		Data:   [8]byte{0xa0, 5, 163, 153, 32, 128, 1, 255},
	}
	a.HandleMessage(&f)

	assert.Equal(t, 1, len(h.packets))
	assert.True(t, h.packets[0].Complete)
}
