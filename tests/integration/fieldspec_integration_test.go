package integration

import (
	"testing"

	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFieldSpecIntegration validates that the new FieldSpec approach works correctly
// by testing key scenarios that were problematic with the old approach
func TestFieldSpecIntegration(t *testing.T) {
	tests := []struct {
		name        string
		pgn         uint32
		fieldId     string
		testData    []byte
		expectNil   bool
		expectValue interface{}
		description string
	}{
		{
			name:        "High precision latitude",
			pgn:         127233, // Man Overboard Notification
			fieldId:     "Latitude",
			testData:    []byte{0x01, 0x02, 0x03, 0x04}, // Mock data
			expectNil:   false,
			description: "Test high-precision field that should use float64",
		},
		{
			name:        "Standard resolution frequency",
			pgn:         65001, // Bus #1 Phase C Basic AC Quantities
			fieldId:     "AcFrequency",
			testData:    []byte{0x10, 0x20}, // Mock data for 16-bit field
			expectNil:   false,
			description: "Test standard precision field that should use float32",
		},
		{
			name:        "Missing value with reserved count",
			pgn:         127233,
			fieldId:     "Latitude",
			testData:    []byte{0xFF, 0xFF, 0xFF, 0x7F}, // Missing value pattern
			expectNil:   true,
			description: "Test that missing values are properly handled",
		},
		{
			name:        "Non-scaled integer field",
			pgn:         126992, // System Time
			fieldId:     "Date",
			testData:    []byte{0x10, 0x20}, // Mock data
			expectNil:   false,
			description: "Test non-scaled field that should use ReadRaw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get PGN info
			pgnInfo := pgn.GetPgnInfo(tt.pgn)
			require.NotNil(t, pgnInfo, "PGN %d should exist", tt.pgn)

			// Get FieldSpec
			fieldSpec, exists := pgnInfo.FieldSpecs[tt.fieldId]
			require.True(t, exists, "Field %s should exist in PGN %d", tt.fieldId, tt.pgn)
			require.NotNil(t, fieldSpec, "FieldSpec should not be nil")

			// Validate FieldSpec has expected properties
			assert.True(t, fieldSpec.BitLength > 0, "BitLength should be positive")
			assert.True(t, fieldSpec.Resolution >= 0, "Resolution should be non-negative")

			// Create a data stream
			stream := pgn.NewDataStream(tt.testData)

			// Test the appropriate read function based on field characteristics
			if fieldSpec.IsScaled() {
				t.Logf("Testing scaled field: Resolution=%.10f, Offset=%d", fieldSpec.Resolution, fieldSpec.Offset)

				// Determine expected type based on resolution threshold
				if fieldSpec.Resolution != 0 && fieldSpec.Resolution <= 0.0000001 {
					// High precision - should use float64
					result, err := pgn.ReadScaled[float64](stream, fieldSpec)
					assert.NoError(t, err, "ReadScaled[float64] should not error")

					if tt.expectNil {
						assert.Nil(t, result, "Expected nil result for missing value")
					} else {
						assert.NotNil(t, result, "Expected non-nil result")
					}
				} else {
					// Standard precision - should use float32
					result, err := pgn.ReadScaled[float32](stream, fieldSpec)
					assert.NoError(t, err, "ReadScaled[float32] should not error")

					if tt.expectNil {
						assert.Nil(t, result, "Expected nil result for missing value")
					} else {
						assert.NotNil(t, result, "Expected non-nil result")
					}
				}
			} else {
				t.Logf("Testing non-scaled field: BitLength=%d, IsSigned=%t", fieldSpec.BitLength, fieldSpec.IsSigned)

				// Non-scaled field - use appropriate integer type
				if fieldSpec.IsSigned {
					if fieldSpec.BitLength <= 32 {
						result, err := pgn.ReadRaw[int32](stream, fieldSpec)
						assert.NoError(t, err, "ReadRaw[int32] should not error")

						if tt.expectNil {
							assert.Nil(t, result, "Expected nil result for missing value")
						} else {
							assert.NotNil(t, result, "Expected non-nil result")
						}
					} else {
						result, err := pgn.ReadRaw[int64](stream, fieldSpec)
						assert.NoError(t, err, "ReadRaw[int64] should not error")

						if tt.expectNil {
							assert.Nil(t, result, "Expected nil result for missing value")
						} else {
							assert.NotNil(t, result, "Expected non-nil result")
						}
					}
				} else {
					if fieldSpec.BitLength <= 32 {
						result, err := pgn.ReadRaw[uint32](stream, fieldSpec)
						assert.NoError(t, err, "ReadRaw[uint32] should not error")

						if tt.expectNil {
							assert.Nil(t, result, "Expected nil result for missing value")
						} else {
							assert.NotNil(t, result, "Expected non-nil result")
						}
					} else {
						result, err := pgn.ReadRaw[uint64](stream, fieldSpec)
						assert.NoError(t, err, "ReadRaw[uint64] should not error")

						if tt.expectNil {
							assert.Nil(t, result, "Expected nil result for missing value")
						} else {
							assert.NotNil(t, result, "Expected non-nil result")
						}
					}
				}
			}
		})
	}
}

// TestFieldSpecConsistency validates that FieldSpecs are consistent across PGNs
func TestFieldSpecConsistency(t *testing.T) {
	for _, pgnInfo := range pgn.PgnList {
		t.Run(pgnInfo.Id, func(t *testing.T) {
			// Validate that all FieldSpecs have valid values
			for fieldId, fieldSpec := range pgnInfo.FieldSpecs {
				assert.True(t, fieldSpec.BitLength > 0, "Field %s should have positive BitLength", fieldId)
				assert.True(t, fieldSpec.Resolution >= 0, "Field %s should have non-negative Resolution", fieldId)

				// Validate MaxRawValue makes sense for the bit length
				// Note: IEEE FLOAT fields (32-bit) don't use the standard reserved value logic
				// and may have MaxRawValue=0, which is expected
				if fieldSpec.BitLength != 32 || fieldSpec.ReservedCount > 0 {
					expectedMaxRaw := uint64(1<<fieldSpec.BitLength) - 1
					if fieldSpec.IsSigned {
						expectedMaxRaw = uint64(1<<(fieldSpec.BitLength-1)) - 1
					}
					if fieldSpec.ReservedCount > 0 {
						expectedMaxRaw -= uint64(fieldSpec.ReservedCount)
					}

					assert.Equal(t, expectedMaxRaw, fieldSpec.MaxRawValue,
						"Field %s MaxRawValue should be calculated correctly", fieldId)
				}

				// Validate MissingValue is only set when there are reserved values
				if fieldSpec.ReservedCount == 0 {
					assert.Equal(t, uint64(0), fieldSpec.MissingValue,
						"Field %s with no reserved values should have MissingValue=0", fieldId)
				} else {
					assert.True(t, fieldSpec.MissingValue > fieldSpec.MaxRawValue,
						"Field %s MissingValue should be greater than MaxRawValue", fieldId)
				}
			}
		})
	}
}

// TestFieldSpecPerformance benchmarks the new FieldSpec approach
func TestFieldSpecPerformance(t *testing.T) {
	// Get a PGN with various field types for testing
	pgnInfo := pgn.GetPgnInfo(127233) // Man Overboard Notification - has high-precision fields
	require.NotNil(t, pgnInfo)

	// Test data
	testData := make([]byte, 32)
	for i := range testData {
		testData[i] = byte(i + 1) // Non-zero test pattern
	}

	// Benchmark scaled field reads
	if latSpec, exists := pgnInfo.FieldSpecs["Latitude"]; exists {
		b := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				stream := pgn.NewDataStream(testData)
				_, _ = pgn.ReadScaled[float64](stream, latSpec)
			}
		})

		t.Logf("ReadScaled[float64] performance: %s", b.String())
	}

	// Benchmark raw field reads
	pgnInfo2 := pgn.GetPgnInfo(126992) // System Time - has integer fields
	if pgnInfo2 != nil {
		if dateSpec, exists := pgnInfo2.FieldSpecs["Date"]; exists {
			b := testing.Benchmark(func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					stream := pgn.NewDataStream(testData)
					_, _ = pgn.ReadRaw[uint16](stream, dateSpec)
				}
			})

			t.Logf("ReadRaw[uint16] performance: %s", b.String())
		}
	}
}

// TestFieldSpecEdgeCases tests edge cases and boundary conditions
func TestFieldSpecEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Zero resolution handling",
			testFunc: func(t *testing.T) {
				// Create a mock FieldSpec with zero resolution
				spec := &pgn.FieldSpec{
					BitLength:     16,
					MaxRawValue:   0xFFFF,
					MissingValue:  0,
					Resolution:    0, // Zero resolution
					Offset:        0,
					IsSigned:      false,
					ReservedCount: 0,
				}

				testData := []byte{0x10, 0x20}
				stream := pgn.NewDataStream(testData)

				// Should handle zero resolution gracefully
				result, err := pgn.ReadScaled[float32](stream, spec)
				assert.NoError(t, err)
				assert.NotNil(t, result)
			},
		},
		{
			name: "Maximum bit length handling",
			testFunc: func(t *testing.T) {
				// Create a mock FieldSpec with 64-bit length
				spec := &pgn.FieldSpec{
					BitLength:     64,
					MaxRawValue:   0xFFFFFFFFFFFFFFFE, // Max - 1 for reserved
					MissingValue:  0xFFFFFFFFFFFFFFFF,
					Resolution:    1.0,
					Offset:        0,
					IsSigned:      false,
					ReservedCount: 1,
				}

				testData := make([]byte, 8)
				for i := range testData {
					testData[i] = 0xFF
				}
				stream := pgn.NewDataStream(testData)

				// Should handle 64-bit fields
				result, err := pgn.ReadRaw[uint64](stream, spec)
				assert.NoError(t, err)
				// Should return nil for missing value pattern
				assert.Nil(t, result)
			},
		},
		{
			name: "Domain constraints",
			testFunc: func(t *testing.T) {
				// Create a mock FieldSpec with domain constraints
				minVal := 10.0
				maxVal := 100.0
				spec := &pgn.FieldSpec{
					BitLength:     16,
					MaxRawValue:   0xFFFF,
					MissingValue:  0,
					Resolution:    0.1,
					Offset:        0,
					IsSigned:      false,
					ReservedCount: 0,
					DomainMin:     &minVal,
					DomainMax:     &maxVal,
				}

				// Test data that would result in value outside domain
				testData := []byte{0xFF, 0xFF} // Very large value
				stream := pgn.NewDataStream(testData)

				result, err := pgn.ReadScaled[float32](stream, spec)
				assert.NoError(t, err)
				assert.NotNil(t, result)

				// Should be clamped to domain max
				assert.True(t, *result <= float32(maxVal))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.testFunc)
	}
}
