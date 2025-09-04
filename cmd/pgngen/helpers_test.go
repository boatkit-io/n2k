package main

import (
	"testing"
)

// Test data for helper function validation
func TestGetReservedValueCount(t *testing.T) {
	tests := []struct {
		name      string
		field     PGNField
		expected  uint8
	}{
		{
			name: "Short field (3 bits) reserves 1 value",
			field: PGNField{
				BitLength: 3,
				FieldType: "NUMBER",
			},
			expected: 1,
		},
		{
			name: "Standard field (8 bits) reserves 2 values",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
			},
			expected: 2,
		},
		{
			name: "Large field (16 bits) reserves 2 values",
			field: PGNField{
				BitLength: 16,
				FieldType: "NUMBER",
			},
			expected: 2,
		},
		{
			name: "Non-numeric field reserves 0 values",
			field: PGNField{
				BitLength: 8,
				FieldType: "STRING_FIX",
			},
			expected: 0,
		},
		{
			name: "Zero bit length reserves 0 values",
			field: PGNField{
				BitLength: 0,
				FieldType: "NUMBER",
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getReservedValueCount(tt.field)
			if result != tt.expected {
				t.Errorf("getReservedValueCount() = %d, expected %d", result, tt.expected)
			}
		})
	}
}

func TestCalcMaxRawValue(t *testing.T) {
	tests := []struct {
		name     string
		field    PGNField
		expected uint64
	}{
		{
			name: "8-bit unsigned field",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
				Signed:    false,
			},
			expected: 0xFF, // 255
		},
		{
			name: "8-bit signed field",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
				Signed:    true,
			},
			expected: 0x7F, // 127 (max positive for signed)
		},
		{
			name: "16-bit unsigned field",
			field: PGNField{
				BitLength: 16,
				FieldType: "NUMBER",
				Signed:    false,
			},
			expected: 0xFFFF, // 65535
		},
		{
			name: "12-bit signed field",
			field: PGNField{
				BitLength: 12,
				FieldType: "NUMBER",
				Signed:    true,
			},
			expected: 0x7FF, // 2047 (max positive for 12-bit signed)
		},
		{
			name: "Non-numeric field returns 0",
			field: PGNField{
				BitLength: 8,
				FieldType: "STRING_FIX",
				Signed:    false,
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcMaxRawValue(tt.field)
			if result != tt.expected {
				t.Errorf("calcMaxRawValue() = 0x%X, expected 0x%X", result, tt.expected)
			}
		})
	}
}

func TestCalcMaxValidRawValue(t *testing.T) {
	tests := []struct {
		name     string
		field    PGNField
		expected uint64
	}{
		{
			name: "8-bit unsigned with 2 reserved values",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
				Signed:    false,
			},
			expected: 0xFD, // 255 - 2 = 253
		},
		{
			name: "3-bit unsigned with 1 reserved value",
			field: PGNField{
				BitLength: 3,
				FieldType: "NUMBER",
				Signed:    false,
			},
			expected: 0x6, // 7 - 1 = 6
		},
		{
			name: "16-bit signed with 2 reserved values",
			field: PGNField{
				BitLength: 16,
				FieldType: "NUMBER",
				Signed:    true,
			},
			expected: 0x7FFD, // 32767 - 2 = 32765
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcMaxValidRawValue(tt.field)
			if result != tt.expected {
				t.Errorf("calcMaxValidRawValue() = 0x%X, expected 0x%X", result, tt.expected)
			}
		})
	}
}

func TestCalcMissingValue(t *testing.T) {
	tests := []struct {
		name     string
		field    PGNField
		expected uint64
	}{
		{
			name: "8-bit unsigned field",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
				Signed:    false,
			},
			expected: 0xFF, // Missing = max value
		},
		{
			name: "8-bit signed field",
			field: PGNField{
				BitLength: 8,
				FieldType: "NUMBER",
				Signed:    true,
			},
			expected: 0x7F, // Missing = max positive value
		},
		{
			name: "Non-numeric field",
			field: PGNField{
				BitLength: 8,
				FieldType: "STRING_FIX",
				Signed:    false,
			},
			expected: 0, // No reserved values = 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcMissingValue(tt.field)
			if result != tt.expected {
				t.Errorf("calcMissingValue() = 0x%X, expected 0x%X", result, tt.expected)
			}
		})
	}
}

func TestNeedsScaling(t *testing.T) {
	resolution1 := float32(1.0)
	resolution025 := float32(0.25)
	
	tests := []struct {
		name     string
		field    PGNField
		expected bool
	}{
		{
			name: "Field with resolution != 1.0 needs scaling",
			field: PGNField{
				Resolution: &resolution025,
				Offset:     0,
			},
			expected: true,
		},
		{
			name: "Field with offset needs scaling",
			field: PGNField{
				Resolution: &resolution1,
				Offset:     100,
			},
			expected: true,
		},
		{
			name: "Field with resolution 1.0 and no offset doesn't need scaling",
			field: PGNField{
				Resolution: &resolution1,
				Offset:     0,
			},
			expected: false,
		},
		{
			name: "Field with nil resolution and no offset doesn't need scaling",
			field: PGNField{
				Resolution: nil,
				Offset:     0,
			},
			expected: false,
		},
		{
			name: "Field with both resolution and offset needs scaling",
			field: PGNField{
				Resolution: &resolution025,
				Offset:     273,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := needsScaling(tt.field)
			if result != tt.expected {
				t.Errorf("needsScaling() = %t, expected %t", result, tt.expected)
			}
		})
	}
}
