package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

func TestDebugDumpPGN_MessageInfoEmbedded(t *testing.T) {
	v := VesselHeading{
		Info: MessageInfo{PGN: 127250, SourceId: 7, Priority: 3},
	}
	result := DebugDumpPGN(v)
	// MessageInfo fields should be flattened into the output (not nested)
	assert.Contains(t, result, "PGN=")
	assert.Contains(t, result, "SourceId=")
	assert.Contains(t, result, "Priority")
}
