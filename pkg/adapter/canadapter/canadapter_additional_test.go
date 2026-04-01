package canadapter

import (
	"testing"

	"github.com/brutella/can"
	"github.com/open-ships/n2k/pkg/pkt"
	"github.com/stretchr/testify/assert"
)

// mockHandler records all packets it receives for test verification
type mockHandler struct {
	packets []pkt.Packet
}

func (m *mockHandler) HandlePacket(p pkt.Packet) {
	m.packets = append(m.packets, p)
}

func TestNewCANAdapter(t *testing.T) {
	a := NewCANAdapter()
	assert.NotNil(t, a)
	assert.NotNil(t, a.multi)
}

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

func TestHandleMessageWithNonCanFrame(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// Send a non-can.Frame message type; should just log a warning, no panic
	type fakeMessage struct{}
	a.HandleMessage(fakeMessage{})

	assert.Equal(t, 0, len(h.packets), "Handler should not receive anything for non-can.Frame message")
}

func TestHandleMessageWithoutHandler(t *testing.T) {
	a := NewCANAdapter()
	// No handler set - should not panic
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)
	assert.NotPanics(t, func() {
		a.HandleMessage(&f)
	})
}

func TestHandleMessageMultipleNonFastPackets(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// Send multiple single-frame packets
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

func TestHandleFastPacketMessage(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// PGN 130820 is a fast packet PGN. Build a multi-frame sequence.
	pInfo := NewPacketInfo(&can.Frame{ID: CanIdFromData(130820, 10, 1, 0), Length: 8})
	_ = pInfo

	// Use full multi-frame sequence from the existing test
	frames := []struct {
		data [8]byte
	}{
		{[8]byte{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}},
		{[8]byte{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}},
		{[8]byte{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}},
		{[8]byte{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}},
		{[8]byte{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}},
	}

	for _, fd := range frames {
		f := can.Frame{
			ID:     CanIdFromData(130820, 10, 1, 0),
			Length: 8,
			Data:   fd.data,
		}
		a.HandleMessage(&f)
	}

	// Only the last frame should complete the packet, so we get one packet
	assert.Equal(t, 1, len(h.packets), "Handler should receive one completed fast packet")
	assert.True(t, h.packets[0].Complete)
	assert.Equal(t, 32, len(h.packets[0].Data))
}

func TestHandleFastPacketSingleFrame(t *testing.T) {
	a := NewCANAdapter()
	h := &mockHandler{}
	a.SetOutput(h)

	// A fast packet that fits in a single frame
	f := can.Frame{
		ID:     CanIdFromData(130820, 10, 1, 0),
		Length: 8,
		Data:   [8]byte{0xa0, 5, 163, 153, 32, 128, 1, 255},
	}
	a.HandleMessage(&f)

	assert.Equal(t, 1, len(h.packets))
	assert.True(t, h.packets[0].Complete)
}
