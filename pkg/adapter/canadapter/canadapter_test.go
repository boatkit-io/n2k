package canadapter

import (
	"testing"

	"github.com/open-ships/n2k/pkg/pgn"
	"github.com/open-ships/n2k/pkg/pkt"
	"github.com/stretchr/testify/assert"
)

// TestPgn127501 verifies end-to-end decoding of PGN 127501 (Binary Switch Bank Status),
// a single-frame non-fast PGN. The test parses a raw CAN log line, creates a packet,
// filters decoders, and then runs the matching decoder to produce a BinarySwitchBankStatus
// struct. This validates the full pipeline from raw CAN data to typed Go struct.
func TestPgn127501(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := CanFrameFromRaw(raw)
	pInfo := NewPacketInfo(&f)
	p := pkt.NewPacket(pInfo, f.Data[:])
	assert.NotEmpty(t, p.Candidates)
	p.AddDecoders()
	assert.Equal(t, len(p.Decoders), 1)
	// Run the decoder and verify it produces the expected struct type.
	decoder := p.Decoders[0]
	stream := pgn.NewPgnDataStream(p.Data)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, pgn.BinarySwitchBankStatus{}, ret)
}
