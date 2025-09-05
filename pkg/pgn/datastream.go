package pgn

import (
	"fmt"
	"math"

	"golang.org/x/exp/constraints"
)

// FieldSpec contains pre-calculated metadata for efficient field read/write operations.
// This eliminates runtime calculations and provides a lightweight alternative to FieldDescriptor.
type FieldSpec struct {
	BitLength     uint16  // Field bit length
	MaxRawValue   uint64  // Pre-calculated maximum valid raw value (accounting for reserved values)
	MissingValue  uint64  // Pre-calculated sentinel value for nil/missing data
	Resolution    float64 // Scaling factor (1.0 = no scaling)
	Offset        int64   // Applied after scaling: scaledValue = (rawValue * resolution) + offset
	IsSigned      bool    // Signed vs unsigned interpretation
	ReservedCount uint8   // Number of reserved values at top of range (0-2)

	// Domain constraints (meaningful value range) - optional validation/clamping
	DomainMin *float64 // Minimum meaningful value (nil = no constraint)
	DomainMax *float64 // Maximum meaningful value (nil = no constraint)
}

// IsScaled returns true if this field requires resolution/offset processing
func (fs FieldSpec) IsScaled() bool {
	return fs.Resolution != 0 && fs.Resolution != 1.0 || fs.Offset != 0
}

// HasDomainConstraints returns true if domain min/max constraints are specified
func (fs FieldSpec) HasDomainConstraints() bool {
	return fs.DomainMin != nil || fs.DomainMax != nil
}

// DataStream instances provide methods to read/write data types to/from a stream.
// byteOffset and bitOffset combine to act as the "read/write cursor".
// The low level read/write functions update the cursor.
type DataStream struct {
	data []uint8

	byteOffset uint16
	bitOffset  uint8
}

// GetData returns the DataStream's current contents
func (s *DataStream) GetData() []uint8 {
	return s.data[:s.byteOffset]
}

// NewDataStream returns a new DataStream. Call it with the data from a complete Packet.
func NewDataStream(data []uint8) *DataStream {
	return &DataStream{
		data:       data,
		byteOffset: 0,
		bitOffset:  0,
	}
}

// getBitOffset method returns the cursor in bits.
func (s *DataStream) getBitOffset() uint32 {
	return uint32(s.byteOffset)*8 + uint32(s.bitOffset)
}

// resetToStart method resets the stream. Used for testing.
func (s *DataStream) resetToStart() {
	s.byteOffset = 0
	s.bitOffset = 0
}

// remainingLength returns the number of bits remaining in the stream
func (s *DataStream) remainingLength() uint16 {
	totalBits := len(s.data)*8 - (int(s.byteOffset)*8 + int(s.bitOffset))
	return uint16(totalBits)
}

// calcMaxPositiveValue calculates the maximum value that can be represented
// with a given length of signed or unsigned contents.
// reservedValuesCount is the number of reserved values for the field. Valid values are 0-2.
func calcMaxPositiveValue(bitLength uint16, signed bool, reservedValuesCount int) uint64 {
	// calculate maximum valid value
	maxVal := uint64(0xFFFFFFFFFFFFFFFF)

	maxVal >>= 64 - bitLength // the largest value representable in length of field
	if signed {               // high bit set means it's negative, so maximum positive value is 1 bit shorter
		maxVal >>= 1 // we know it's a positive value, so safe for us to check.
	}

	if reservedValuesCount > 0 {
		if reservedValuesCount > 2 {
			reservedValuesCount = 2
		}
		maxVal -= uint64(reservedValuesCount)
	}

	return maxVal
}

// missingValue calculates the value representing a missing (nil) wire value
func missingValue(bitLength uint16, signed bool, reservedValuesCount int) uint64 {
	if reservedValuesCount == 0 {
		// No reserved values means we can't represent missing - return 0 as a safe default
		return 0
	}

	missing := uint64(0xFFFFFFFFFFFFFFFF)
	missing >>= 64 - bitLength // the largest value representable in length of field if unsigned
	if signed {                // high bit set means it's negative, so maximum positive value is 1 bit shorter
		missing >>= 1 // missing flag is max positive value; negative value has high bit set
	}
	return missing
}

// calcPrecision calculates the resulting precision of applying a given resolution to a given value
func calcPrecision(resolution float64) uint8 {
	precision := resolution
	digits := uint8(0)
	for precision >= 0 && precision < 1.0 {
		precision *= 10
		digits++
	}
	return digits
}

// roundFloat rounds a float64 to the specified precision
func roundFloat(val float64, precision uint8) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

// roundFloat32 rounds a float32 to the specified precision
func roundFloat32(val float32, precision uint8) float32 {
	ratio := math.Pow(10, float64(precision))
	return float32(math.Round(float64(val)*ratio) / ratio)
}

// ReadRaw reads a non-scaled integer field using pre-calculated FieldSpec metadata
func ReadRaw[T constraints.Integer](s *DataStream, spec *FieldSpec) (*T, error) {
	if spec == nil {
		return nil, fmt.Errorf("FieldSpec is nil")
	}
	if spec.BitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength exceeds 64 bits", spec.BitLength)
	}

	var rawValue uint64
	var err error

	if spec.ReservedCount == 0 {
		// No reserved values - all bit patterns are valid, never return nil
		rawValue, err = s.getNumberRaw(spec.BitLength)
		if err != nil {
			return nil, err
		}
	} else {
		// Has reserved values - use nullable logic
		v, err := s.getNullableNumberRaw(spec.BitLength, spec.IsSigned, int(spec.ReservedCount))
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, nil
		}
		rawValue = *v
	}

	// Apply offset for non-scaled fields
	if spec.Offset != 0 {
		if spec.IsSigned {
			signedVal := int64(rawValue)
			signedVal += spec.Offset
			rawValue = uint64(signedVal)
		} else {
			rawValue = uint64(int64(rawValue) + spec.Offset)
		}
	}

	// Handle signed conversion if needed
	if spec.IsSigned && spec.BitLength < 64 {
		if (rawValue & (1 << (spec.BitLength - 1))) != 0 {
			// Negative number - sign extend
			mask := uint64(0xFFFFFFFFFFFFFFFF) << spec.BitLength
			rawValue |= mask
		}
	}

	result := T(rawValue)
	return &result, nil
}

// ReadScaled reads a scaled float field using pre-calculated FieldSpec metadata
func ReadScaled[T constraints.Float](s *DataStream, spec *FieldSpec) (*T, error) {
	if spec == nil {
		return nil, fmt.Errorf("FieldSpec is nil")
	}
	if spec.BitLength > 64 {
		return nil, fmt.Errorf("requested %d bitLength exceeds 64 bits", spec.BitLength)
	}

	var val float64

	if spec.ReservedCount == 0 {
		// No reserved values - read raw value
		v, err := s.getNumberRaw(spec.BitLength)
		if err != nil {
			return nil, err
		}

		if spec.IsSigned {
			// Handle signed conversion
			if spec.BitLength < 64 && (v&(1<<(spec.BitLength-1))) != 0 {
				mask := uint64(0xFFFFFFFFFFFFFFFF) << spec.BitLength
				v |= mask
			}
			val = float64(int64(v))
		} else {
			// Unsigned value
			val = float64(v)
		}
	} else {
		// Has reserved values - use nullable logic
		if spec.IsSigned {
			rawValue, err := s.getSignedNullableNumber(spec.BitLength, int(spec.ReservedCount))
			if err != nil {
				return nil, err
			}
			if rawValue == nil {
				return nil, nil
			}
			val = float64(*rawValue)
		} else {
			rawValue, err := s.getUnsignedNullableNumber(spec.BitLength, int(spec.ReservedCount))
			if err != nil {
				return nil, err
			}
			if rawValue == nil {
				return nil, nil
			}
			val = float64(*rawValue)
		}
	}

	// Apply resolution scaling first
	if spec.Resolution != 0 && spec.Resolution != 1.0 {
		prec := calcPrecision(spec.Resolution)
		val = roundFloat(val*spec.Resolution, prec)
	}

	// Then add offset
	val += float64(spec.Offset)

	// Apply domain constraints if specified
	if spec.DomainMax != nil && val > *spec.DomainMax {
		val = *spec.DomainMax
	}
	if spec.DomainMin != nil && val < *spec.DomainMin {
		val = *spec.DomainMin
	}

	result := T(val)
	return &result, nil
}
