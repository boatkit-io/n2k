package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDebugDumpPGN_SimpleStruct verifies that DebugDumpPGN produces output containing
// the struct type name and all populated field values. Uses a VesselHeading with
// non-nil Sid and Heading fields to ensure pointer dereferencing works correctly.
func TestDebugDumpPGN_SimpleStruct(t *testing.T) {
	heading := float32(1.5)
	sid := uint8(3)
	v := VesselHeading{
		Info:    MessageInfo{PGN: 127250, SourceId: 1, Priority: 2},
		Sid:     &sid,
		Heading: &heading,
	}
	result := DebugDumpPGN(v)
	assert.Contains(t, result, "VesselHeading:")
	assert.Contains(t, result, "PGN=")
	assert.Contains(t, result, "127250")
	assert.Contains(t, result, "SourceId=")
	assert.Contains(t, result, "Sid=")
	assert.Contains(t, result, "Heading=")
}

// TestDebugDumpPGN_NilPointers verifies that nil pointer fields (representing NMEA 2000
// "data not available") are rendered as "nil" in the debug output rather than causing
// a nil dereference panic. All optional fields on VesselHeading are left nil.
func TestDebugDumpPGN_NilPointers(t *testing.T) {
	v := VesselHeading{
		Info: MessageInfo{PGN: 127250, SourceId: 5},
		Sid:  nil,
	}
	result := DebugDumpPGN(v)
	assert.Contains(t, result, "VesselHeading:")
	assert.Contains(t, result, "Sid=nil")
	assert.Contains(t, result, "Heading=nil")
	assert.Contains(t, result, "Deviation=nil")
	assert.Contains(t, result, "Variation=nil")
}

// TestDebugDumpPGN_NonNilPointers verifies that non-nil pointer fields are dereferenced
// and their values printed (not the pointer address). Uses RateOfTurn with both Sid and
// Rate populated, and checks that neither appears as "nil" in the output.
func TestDebugDumpPGN_NonNilPointers(t *testing.T) {
	rate := float64(-0.5)
	sid := uint8(1)
	v := RateOfTurn{
		Info: MessageInfo{PGN: 127251, SourceId: 2},
		Sid:  &sid,
		Rate: &rate,
	}
	result := DebugDumpPGN(v)
	assert.Contains(t, result, "RateOfTurn:")
	assert.Contains(t, result, "Sid=")
	assert.NotContains(t, result, "Sid=nil")
	assert.Contains(t, result, "Rate=")
	assert.NotContains(t, result, "Rate=nil")
}

// TestDebugDumpPGN_MessageInfoEmbedded verifies that the embedded MessageInfo struct's
// fields (PGN, SourceId, Priority) are flattened into the top-level output rather than
// being nested inside an "Info={...}" wrapper. This makes the debug output more readable
// since every PGN struct embeds MessageInfo.
func TestDebugDumpPGN_MessageInfoEmbedded(t *testing.T) {
	v := VesselHeading{
		Info: MessageInfo{PGN: 127250, SourceId: 7, Priority: 3},
	}
	result := DebugDumpPGN(v)
	assert.Contains(t, result, "PGN=")
	assert.Contains(t, result, "SourceId=")
	assert.Contains(t, result, "Priority")
}
