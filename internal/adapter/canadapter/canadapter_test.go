package canadapter

import (
	"testing"

	"github.com/boatkit-io/n2k/internal/converter"
	"github.com/boatkit-io/n2k/internal/pgn"
	"github.com/boatkit-io/n2k/internal/pkt"

	"github.com/stretchr/testify/assert"
)

func TestPgn127501(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f, err := converter.CanFrameFromRaw(raw)
	assert.Nil(t, err)
	pInfo := ExtractMessageInfo(f[0])
	p := pkt.NewPacket(pInfo, f[0].Data[:])
	stream := pgn.NewDataStream(p.Data)
	decoder, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, pgn.BinarySwitchBankStatus{}, ret)
}

func TestPgn127501Write(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f, err := converter.CanFrameFromRaw(raw)
	assert.Nil(t, err)
	pInfo := ExtractMessageInfo(f[0])
	p := pkt.NewPacket(pInfo, f[0].Data[:])
	stream := pgn.NewDataStream(p.Data)
	decoder, err := pgn.FindDecoder(stream, p.Info.PGN)
	assert.Nil(t, err)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, pgn.BinarySwitchBankStatus{}, ret)
}

// TestRawToDataStream was removed as redundant to more comprehensive testing in tests/integration/pgn_serialization_test.go
