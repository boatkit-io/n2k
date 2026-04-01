package pgn

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsProprietaryPGN_PDU1Addressed tests the proprietary single-frame addressed range
// (0x0EF00 - 0x0EFFF). Checks start, end, and middle of the range to ensure
// the boundary comparisons are inclusive on both ends.
func TestIsProprietaryPGN_PDU1Addressed(t *testing.T) {
	assert.True(t, IsProprietaryPGN(0x0EF00), "start of PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x0EFFF), "end of PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x0EF80), "middle of PDU1 addressed range")
}

// TestIsProprietaryPGN_PDU2NonAddressed tests the proprietary single-frame broadcast range
// (0x0FF00 - 0x0FFFF). These are non-addressed (destination = 255 broadcast).
func TestIsProprietaryPGN_PDU2NonAddressed(t *testing.T) {
	assert.True(t, IsProprietaryPGN(0x0FF00), "start of PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x0FFFF), "end of PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x0FF80), "middle of PDU2 non-addressed range")
}

// TestIsProprietaryPGN_FastPDU1Addressed tests the proprietary fast-packet addressed range
// (0x1EF00 - 0x1EFFF). Fast-packet PGNs can span multiple CAN frames.
func TestIsProprietaryPGN_FastPDU1Addressed(t *testing.T) {
	assert.True(t, IsProprietaryPGN(0x1EF00), "start of fast PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x1EFFF), "end of fast PDU1 addressed range")
	assert.True(t, IsProprietaryPGN(0x1EF80), "middle of fast PDU1 addressed range")
}

// TestIsProprietaryPGN_FastPDU2NonAddressed tests the proprietary fast-packet broadcast range
// (0x1FF00 - 0x1FFFF).
func TestIsProprietaryPGN_FastPDU2NonAddressed(t *testing.T) {
	assert.True(t, IsProprietaryPGN(0x1FF00), "start of fast PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x1FFFF), "end of fast PDU2 non-addressed range")
	assert.True(t, IsProprietaryPGN(0x1FF80), "middle of fast PDU2 non-addressed range")
}

// TestIsProprietaryPGN_BoundaryValues verifies that PGN values just outside each
// proprietary range are correctly classified as non-proprietary. This catches
// off-by-one errors in the range comparisons.
func TestIsProprietaryPGN_BoundaryValues(t *testing.T) {
	assert.False(t, IsProprietaryPGN(0x0EEFF), "one below PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x0F000), "one above PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x0FEFF), "one below PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x10000), "one above PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x1EEFF), "one below fast PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x1F000), "one above fast PDU1 addressed range")
	assert.False(t, IsProprietaryPGN(0x1FEFF), "one below fast PDU2 non-addressed range")
	assert.False(t, IsProprietaryPGN(0x20000), "one above fast PDU2 non-addressed range")
}

// TestIsProprietaryPGN_NonProprietary verifies that well-known standard PGNs
// (Vessel Heading, Binary Switch Bank, ISO Request) and PGN 0 are not classified
// as proprietary.
func TestIsProprietaryPGN_NonProprietary(t *testing.T) {
	assert.False(t, IsProprietaryPGN(0))
	assert.False(t, IsProprietaryPGN(127250)) // Vessel Heading
	assert.False(t, IsProprietaryPGN(127501)) // Binary Switch Bank Status
	assert.False(t, IsProprietaryPGN(59904))  // ISO Request
}

// TestGetProprietaryInfo verifies extraction of manufacturer and industry codes from
// a proprietary PGN payload. Constructs a 2-byte header with manufacturer code 381 (B&G)
// and industry code 4 (Marine), matching the wire format:
//   - Bits 0-10: manufacturer code (11 bits)
//   - Bits 11-12: reserved
//   - Bits 13-15: industry code (3 bits)
func TestGetProprietaryInfo(t *testing.T) {
	manCode := uint16(381)
	industryCode := uint8(4)
	// Low byte contains the bottom 8 bits of the 11-bit manufacturer code.
	lowByte := uint8(manCode & 0xFF)
	// High byte packs: bits 0-2 = top 3 bits of manufacturer code,
	// bits 3-4 = reserved (0), bits 5-7 = industry code.
	highByte := uint8((manCode>>8)&0x07) | uint8(industryCode<<5)

	man, ind, err := GetProprietaryInfo([]uint8{lowByte, highByte, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, ManufacturerCodeConst(381), man)
	assert.Equal(t, IndustryCodeConst(4), ind)
}

// TestGetProprietaryInfo_DifferentManufacturer repeats the proprietary info extraction
// with a different manufacturer code (419) to verify the bit packing is not
// coincidentally correct for only one value.
func TestGetProprietaryInfo_DifferentManufacturer(t *testing.T) {
	manCode := uint16(419)
	industryCode := uint8(4)
	lowByte := uint8(manCode & 0xFF)
	highByte := uint8((manCode>>8)&0x07) | uint8(industryCode<<5)

	man, ind, err := GetProprietaryInfo([]uint8{lowByte, highByte, 0, 0})
	assert.NoError(t, err)
	assert.Equal(t, ManufacturerCodeConst(419), man)
	assert.Equal(t, IndustryCodeConst(4), ind)
}

// TestGetFieldDescriptor_KnownPGN verifies that field descriptors can be retrieved for
// a well-known PGN (127250 Vessel Heading). Checks both field 1 (SID, 8-bit) and
// field 2 (Heading) to ensure the field map is correctly populated by the generated code.
func TestGetFieldDescriptor_KnownPGN(t *testing.T) {
	fd, err := GetFieldDescriptor(127250, 0, 1)
	assert.NoError(t, err)
	assert.NotNil(t, fd)
	assert.Equal(t, "SID", fd.Name)
	assert.Equal(t, uint16(8), fd.BitLength)

	fd2, err := GetFieldDescriptor(127250, 0, 2)
	assert.NoError(t, err)
	assert.NotNil(t, fd2)
	assert.Equal(t, "Heading", fd2.Name)
}

// TestGetFieldDescriptor_UnknownPGN verifies that requesting a field descriptor for a
// PGN number that does not exist in the database returns a "PGN not found" error.
func TestGetFieldDescriptor_UnknownPGN(t *testing.T) {
	fd, err := GetFieldDescriptor(999999, 0, 1)
	assert.Error(t, err)
	assert.Nil(t, fd)
	assert.Contains(t, err.Error(), "PGN not found")
}

// TestGetFieldDescriptor_MissingFieldIndex verifies that requesting a field index
// that does not exist within an otherwise valid PGN returns a "not found" error.
func TestGetFieldDescriptor_MissingFieldIndex(t *testing.T) {
	fd, err := GetFieldDescriptor(127250, 0, 99)
	assert.Error(t, err)
	assert.Nil(t, fd)
	assert.Contains(t, err.Error(), "not found")
}

// TestSearchUnseenList_KnownUnseen verifies that PGN 127490 (which is in the canboat
// database but has no sample data) is found in the unseen list.
func TestSearchUnseenList_KnownUnseen(t *testing.T) {
	assert.True(t, SearchUnseenList(127490))
}

// TestSearchUnseenList_KnownSeen verifies that PGN 127250 (Vessel Heading), which has
// sample data and a tested decoder, is NOT in the unseen list.
func TestSearchUnseenList_KnownSeen(t *testing.T) {
	assert.False(t, SearchUnseenList(127250))
}

// TestSearchUnseenList_CompletelyUnknown verifies that a fabricated PGN number not in
// the canboat database at all is not found in the unseen list (it's not unseen, it's
// simply nonexistent).
func TestSearchUnseenList_CompletelyUnknown(t *testing.T) {
	assert.False(t, SearchUnseenList(999999))
}

// TestPgnInfoLookup_Populated verifies that the init() function correctly populates
// the PgnInfoLookup map with known PGNs from the generated pgnList.
func TestPgnInfoLookup_Populated(t *testing.T) {
	assert.NotNil(t, PgnInfoLookup[127250], "Vessel Heading should be in lookup")
	assert.NotNil(t, PgnInfoLookup[127501], "Binary Switch Bank Status should be in lookup")
	assert.Greater(t, len(PgnInfoLookup[127250]), 0)
	assert.Greater(t, len(PgnInfoLookup[127501]), 0)
}

// TestPgnInfoLookup_PGNDetails verifies that a specific PgnInfo entry has the expected
// metadata: correct PGN number, description, fast-packet flag, non-nil decoder, and
// a valid Self pointer. Uses Vessel Heading (127250) as a well-known reference PGN.
func TestPgnInfoLookup_PGNDetails(t *testing.T) {
	infos := PgnInfoLookup[127250]
	assert.Equal(t, 1, len(infos))
	assert.Equal(t, uint32(127250), infos[0].PGN)
	assert.Equal(t, "Vessel Heading", infos[0].Description)
	assert.False(t, infos[0].Fast)
	assert.NotNil(t, infos[0].Decoder)
	assert.NotNil(t, infos[0].Self)
}

// TestPgnInfoLookup_UnknownPGN verifies that looking up a nonexistent PGN returns nil
// (not an empty slice or a panic).
func TestPgnInfoLookup_UnknownPGN(t *testing.T) {
	assert.Nil(t, PgnInfoLookup[999999])
}
