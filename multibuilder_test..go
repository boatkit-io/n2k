package n2k

/*
import (
	"testing"

	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestFastPacket(t *testing.T) {
	tr := newFastPacketTracker(logrus.StandardLogger())

	// test fast packet that's actually complete in single-frame
	f := newFrame(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{160, 5, 163, 153, 32, 128, 1, 255}})
	d, e := tr.processFrame(f)
	assert.NoError(t, e)
	assert.True(t, d)
	r := decodeFrame(f)
	assert.IsType(t, FusionPowerState{}, r)

	// test failure case from framenum=1 with no pending framenum=0 for this seqid
	// SKIPPING this test. To allow for fast pgns that fit into a single packet,
	// processFrame returns this as complete, allowing later code to decode it
	// or fail if invalid
	//	f = newFrame(can.Frame{ID: canIdFromData(130820, 10, 1), Length: 8, Data: [8]uint8{161, 5, 163, 153, 32, 128, 1, 255}})
	//	_, e = tr.processFrame(f)
	//	assert.Error(t, e)

	// test misc multi frame packet that's in the data dictionary
	f = newFrame(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x60, 0x20, 0x00, 0x10, 0x13, 0x80, 0x0C, 0x70}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)

	f = newFrame(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x61, 0x86, 0x0A, 0x05, 0x80, 0x00, 0x58, 0xE8}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)

	f = newFrame(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x62, 0x55, 0x00, 0xFF, 0xFF, 0x00, 0x00, 0x7F}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)

	f = newFrame(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x63, 0x00, 0x00, 0x00, 0x00, 0x10, 0x7F, 0xFF}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)

	f = newFrame(can.Frame{ID: 0x09F20183, Length: 8, Data: [8]uint8{0x64, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0xFF}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.True(t, d)
	r = decodeFrame(f)
	assert.IsType(t, EngineParametersDynamic{}, r)

	// test misc multi frame packet that's not in the data dictionary
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{224, 60, 137, 152, 0, 1, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{225, 255, 127, 255, 255, 255, 127, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{226, 255, 255, 127, 255, 255, 255, 127}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{227, 0, 0, 0, 0, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{228, 255, 255, 255, 255, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{229, 255, 127, 255, 255, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{230, 255, 255, 255, 255, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{231, 255, 255, 255, 127, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.False(t, d)
	f = newFrame(can.Frame{ID: canIdFromData(130839, 41, 1), Length: 8, Data: [8]uint8{232, 127, 255, 255, 255, 255, 255, 255}})
	d, e = tr.processFrame(f)
	assert.NoError(t, e)
	assert.True(t, d)
	r = decodeFrame(f)
	assert.IsType(t, UnknownPGN{}, r)
}
*/
