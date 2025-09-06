package pgn

import (
	"testing"
)

func TestFieldSpecIsScaled(t *testing.T) {
	tests := []struct {
		name     string
		spec     FieldSpec
		expected bool
	}{
		{
			name: "Resolution != 1.0 is scaled",
			spec: FieldSpec{
				Resolution: 0.25,
				Offset:     0,
			},
			expected: true,
		},
		{
			name: "Offset != 0 is scaled",
			spec: FieldSpec{
				Resolution: 1.0,
				Offset:     100,
			},
			expected: true,
		},
		{
			name: "Resolution 1.0 and Offset 0 is not scaled",
			spec: FieldSpec{
				Resolution: 1.0,
				Offset:     0,
			},
			expected: false,
		},
		{
			name: "Both resolution and offset makes it scaled",
			spec: FieldSpec{
				Resolution: 0.01,
				Offset:     273,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spec.IsScaled()
			if result != tt.expected {
				t.Errorf("IsScaled() = %t, expected %t", result, tt.expected)
			}
		})
	}
}

func TestFieldSpecHasDomainConstraints(t *testing.T) {
	min := 10.0
	max := 100.0

	tests := []struct {
		name     string
		spec     FieldSpec
		expected bool
	}{
		{
			name: "Has domain min",
			spec: FieldSpec{
				DomainMin: &min,
				DomainMax: nil,
			},
			expected: true,
		},
		{
			name: "Has domain max",
			spec: FieldSpec{
				DomainMin: nil,
				DomainMax: &max,
			},
			expected: true,
		},
		{
			name: "Has both domain constraints",
			spec: FieldSpec{
				DomainMin: &min,
				DomainMax: &max,
			},
			expected: true,
		},
		{
			name: "Has no domain constraints",
			spec: FieldSpec{
				DomainMin: nil,
				DomainMax: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.spec.HasDomainConstraints()
			if result != tt.expected {
				t.Errorf("HasDomainConstraints() = %t, expected %t", result, tt.expected)
			}
		})
	}
}

func TestReadRawUint16(t *testing.T) {
	// Test data: 0x1234 in little-endian (0x34, 0x12)
	data := []uint8{0x34, 0x12}
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     16,
		MaxRawValue:   0xFFFD, // 65533 (accounting for 2 reserved values)
		MissingValue:  0xFFFF, // 65535
		Resolution:    1.0,
		Offset:        0,
		IsSigned:      false,
		ReservedCount: 2,
	}

	result, err := ReadRaw[uint16](stream, spec)
	if err != nil {
		t.Fatalf("ReadRaw failed: %v", err)
	}
	if result == nil {
		t.Fatal("ReadRaw returned nil")
	}
	if *result != 0x1234 {
		t.Errorf("ReadRaw() = 0x%X, expected 0x1234", *result)
	}
}

func TestReadRawWithOffset(t *testing.T) {
	// Test data: value 10
	data := []uint8{10}
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253 (accounting for 2 reserved values)
		MissingValue:  0xFF, // 255
		Resolution:    1.0,
		Offset:        1000, // Add offset of 1000
		IsSigned:      false,
		ReservedCount: 2,
	}

	result, err := ReadRaw[uint16](stream, spec)
	if err != nil {
		t.Fatalf("ReadRaw failed: %v", err)
	}
	if result == nil {
		t.Fatal("ReadRaw returned nil")
	}
	expected := uint16(10 + 1000) // raw value + offset
	if *result != expected {
		t.Errorf("ReadRaw() = %d, expected %d", *result, expected)
	}
}

func TestReadRawMissingValue(t *testing.T) {
	// Test data: missing value (0xFF for 8-bit field with 2 reserved values)
	data := []uint8{0xFF}
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253 (accounting for 2 reserved values)
		MissingValue:  0xFF, // 255
		Resolution:    1.0,
		Offset:        0,
		IsSigned:      false,
		ReservedCount: 2,
	}

	result, err := ReadRaw[uint8](stream, spec)
	if err != nil {
		t.Fatalf("ReadRaw failed: %v", err)
	}
	if result != nil {
		t.Errorf("ReadRaw() = %v, expected nil for missing value", result)
	}
}

func TestReadScaledFloat32(t *testing.T) {
	// Test data: raw value 100
	data := []uint8{100}
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253 (accounting for 2 reserved values)
		MissingValue:  0xFF, // 255
		Resolution:    0.1,  // 0.1 units per bit
		Offset:        20,   // Add 20 after scaling
		IsSigned:      false,
		ReservedCount: 2,
	}

	result, err := ReadScaled[float32](stream, spec)
	if err != nil {
		t.Fatalf("ReadScaled failed: %v", err)
	}
	if result == nil {
		t.Fatal("ReadScaled returned nil")
	}

	expected := float32(100*0.1 + 20) // (rawValue * resolution) + offset = 10 + 20 = 30
	if *result != expected {
		t.Errorf("ReadScaled() = %f, expected %f", *result, expected)
	}
}

func TestReadScaledWithDomainConstraints(t *testing.T) {
	// Test data: raw value that would scale to 200, but domain max is 150
	data := []uint8{200}
	stream := NewDataStream(data)

	domainMax := 150.0
	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253
		MissingValue:  0xFF, // 255
		Resolution:    1.0,  // 1:1 scaling
		Offset:        0,
		IsSigned:      false,
		ReservedCount: 2,
		DomainMax:     &domainMax,
	}

	result, err := ReadScaled[float32](stream, spec)
	if err != nil {
		t.Fatalf("ReadScaled failed: %v", err)
	}
	if result == nil {
		t.Fatal("ReadScaled returned nil")
	}

	// Should be clamped to domain max
	if *result != 150.0 {
		t.Errorf("ReadScaled() = %f, expected 150.0 (clamped to domain max)", *result)
	}
}

func TestWriteRawUint16(t *testing.T) {
	data := make([]uint8, 2)
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     16,
		MaxRawValue:   0xFFFD, // 65533
		MissingValue:  0xFFFF, // 65535
		Resolution:    1.0,
		Offset:        0,
		IsSigned:      false,
		ReservedCount: 2,
	}

	value := uint16(0x1234)
	err := WriteRaw(stream, &value, spec)
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Verify the data was written correctly (little-endian)
	expected := []uint8{0x34, 0x12}
	result := stream.GetData()
	if len(result) != 2 {
		t.Fatalf("Expected 2 bytes written, got %d", len(result))
	}
	for i, b := range expected {
		if result[i] != b {
			t.Errorf("Byte %d: got 0x%02X, expected 0x%02X", i, result[i], b)
		}
	}
}

func TestWriteRawNilValue(t *testing.T) {
	data := make([]uint8, 1)
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253
		MissingValue:  0xFF, // 255
		Resolution:    1.0,
		Offset:        0,
		IsSigned:      false,
		ReservedCount: 2,
	}

	err := WriteRaw[uint8](stream, nil, spec)
	if err != nil {
		t.Fatalf("WriteRaw failed: %v", err)
	}

	// Should write the missing value
	result := stream.GetData()
	if len(result) != 1 || result[0] != 0xFF {
		t.Errorf("Expected missing value 0xFF, got 0x%02X", result[0])
	}
}

func TestWriteScaledFloat32(t *testing.T) {
	data := make([]uint8, 1)
	stream := NewDataStream(data)

	spec := &FieldSpec{
		BitLength:     8,
		MaxRawValue:   0xFD, // 253
		MissingValue:  0xFF, // 255
		Resolution:    0.1,  // 0.1 units per bit
		Offset:        20,   // Subtract 20 before scaling
		IsSigned:      false,
		ReservedCount: 2,
	}

	value := float32(30.0) // Should become: (30 - 20) / 0.1 = 100 raw
	err := WriteScaled(stream, &value, spec)
	if err != nil {
		t.Fatalf("WriteScaled failed: %v", err)
	}

	result := stream.GetData()
	if len(result) != 1 || result[0] != 100 {
		t.Errorf("Expected raw value 100, got %d", result[0])
	}
}
