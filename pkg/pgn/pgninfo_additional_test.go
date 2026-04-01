package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsProprietaryPGN_PDU1Addressed(t *testing.T) {
	// Range 0x0EF00 to 0x0EFFF (61184 - 61439)
	assert.True(t, IsProprietaryPGN(0x0EF00), "start of PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x0EFFF), "end of PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x0EF80), "middle of PDU1 addressed range")
}

func TestIsProprietaryPGN_PDU2NonAddressed(t *testing.T) {
	// Range 0x0FF00 to 0x0FFFF (65280 - 65535)
	assert.True(t, IsProprietaryPGN(0x0FF00), "start of PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x0FFFF), "end of PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x0FF80), "middle of PDU2 non-addressed range")
}

func TestIsProprietaryPGN_FastPDU1Addressed(t *testing.T) {
	// Range 0x1EF00 to 0x1EFFF (126720 - 126975)
	assert.True(t, IsProprietaryPGN(0x1EF00), "start of fast PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x1EFFF), "end of fast PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x1EF80), "middle of fast PDU1 addressed range")
}

func TestIsProprietaryPGN_FastPDU2NonAddressed(t *testing.T) {
	// Range 0x1FF00 to 0x1FFFF (130816 - 131071)
	assert.True(t, IsProprietaryPGN(0x1FF00), "start of fast PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x1FFFF), "end of fast PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x1FF80), "middle of fast PDU2 non-addressed range")
}

func TestIsProprietaryPGN_BoundaryValues(t *testing.T) {
	// Just outside each range
	assert.False(t, IsProprietaryPGN(0x0EEFF), "one below PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x0F000), "one above PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x0FEFF), "one below PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x10000), "one above PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x1EEFF), "one below fast PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x1F000), "one above fast PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x1FEFF), "one below fast PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x20000), "one above fast PDU2 non-addressed range")
}

func TestIsProprietaryPGN_NonProprietary(t *testing.T) {
	assert.False(t, IsProprietaryPGN(0))
	assert.False(t, IsProprietaryPGN(127250)) // Vessel Heading
	assert.False(t, IsProprietaryPGN(127501)) // Binary Switch Bank Status
	assert.False(t, IsProprietaryPGN(59904))  // ISO Request
}

func TestGetProprietaryInfo(t *testing.T) {
	// Encode manufacturer code 381 (B&G) = 0x17D in 11 bits
	// Byte layout: bits 0-7 = low 8 bits of man code, bits 8-10 = high 3 bits of man code,
	// bits 11-12 = reserved, bits 13-15 = industry code
	// 381 = 0b101_1111_1101
	// low byte: 0x7D (bits 0-7)
	// high byte: bits 8-10 = 0b001 (man code high), bits 11-12 = reserved (0), bits 13-15 = industry code
	// With industry code = 4 (Marine): 4 << 5 = 0x80, man high bits = 0x01
	// So high byte = 0x01 | 0x80 = 0x81
	manCode := uint16(381)
	industryCode := uint8(4)
	lowByte := uint8(manCode & 0xFF)
	highByte := uint8((manCode>>8)&0x07) | uint8(industryCode<<5)

	man, ind, err := GetProprietaryInfo([]uint8{lowByte, highByte, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, ManufacturerCodeConst(381), man)
	assert.Equal(t, IndustryCodeConst(4), ind)
}

func TestGetProprietaryInfo_DifferentManufacturer(t *testing.T) {
	// Manufacturer code 419 = 0x1A3
	// Industry code 4 (Marine)
	manCode := uint16(419)
	industryCode := uint8(4)
	lowByte := uint8(manCode & 0xFF)
	highByte := uint8((manCode>>8)&0x07) | uint8(industryCode<<5)

	man, ind, err := GetProprietaryInfo([]uint8{lowByte, highByte, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, ManufacturerCodeConst(419), man)
	assert.Equal(t, IndustryCodeConst(4), ind)
}

func TestGetFieldDescriptor_KnownPGN(t *testing.T) {
	// PGN 127250 (Vessel Heading) - field 1 is SID
	fd, err := GetFieldDescriptor(127250, 0, 1)
	assert.NoError(t, err)
	assert.NotNil(t, fd)
	assert.Equal(t, "SID", fd.Name)
	assert.Equal(t, uint16(8), fd.BitLength)

	// field 2 is Heading
	fd2, err := GetFieldDescriptor(127250, 0, 2)
	assert.NoError(t, err)
	assert.NotNil(t, fd2)
	assert.Equal(t, "Heading", fd2.Name)
}

func TestGetFieldDescriptor_UnknownPGN(t *testing.T) {
	fd, err := GetFieldDescriptor(999999, 0, 1)
	assert.Error(t, err)
	assert.Nil(t, fd)
	assert.Contains(t, err.Error(), "PGN not found")
}

func TestGetFieldDescriptor_MissingFieldIndex(t *testing.T) {
	// PGN 127250 exists but field index 99 does not
	fd, err := GetFieldDescriptor(127250, 0, 99)
	assert.Error(t, err)
	assert.Nil(t, fd)
	assert.Contains(t, err.Error(), "not found")
}

func TestSearchUnseenList_KnownUnseen(t *testing.T) {
	// 127490 is in the unseen list per the generated code
	assert.True(t, SearchUnseenList(127490))
}

func TestSearchUnseenList_KnownSeen(t *testing.T) {
	// 127250 (Vessel Heading) has sample data and is NOT in the unseen list
	assert.False(t, SearchUnseenList(127250))
}

func TestSearchUnseenList_CompletelyUnknown(t *testing.T) {
	// A completely unknown PGN should not be in the unseen list either
	assert.False(t, SearchUnseenList(999999))
}

func TestPgnInfoLookup_Populated(t *testing.T) {
	// Verify lookup is populated with known PGNs
	assert.NotNil(t, PgnInfoLookup[127250], "Vessel Heading should be in lookup")
	assert.NotNil(t, PgnInfoLookup[127501], "Binary Switch Bank Status should be in lookup")
	assert.Greater(t, len(PgnInfoLookup[127250]), 0)
	assert.Greater(t, len(PgnInfoLookup[127501]), 0)
}

func TestPgnInfoLookup_PGNDetails(t *testing.T) {
	infos := PgnInfoLookup[127250]
	assert.Equal(t, 1, len(infos))
	assert.Equal(t, uint32(127250), infos[0].PGN)
	assert.Equal(t, "Vessel Heading", infos[0].Description)
	assert.False(t, infos[0].Fast)
	assert.NotNil(t, infos[0].Decoder)
	assert.NotNil(t, infos[0].Self)
}

func TestPgnInfoLookup_UnknownPGN(t *testing.T) {
	assert.Nil(t, PgnInfoLookup[999999])
}
