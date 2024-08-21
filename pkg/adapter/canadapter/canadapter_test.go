package canadapter

import (
	"testing"

	"github.com/boatkit-io/n2k/pkg/converter"
	"github.com/boatkit-io/n2k/pkg/pgn"
	"github.com/boatkit-io/n2k/pkg/pkt"

	//	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestPgn127501(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := converter.CanFrameFromRaw(raw)
	pInfo := NewPacketInfo(f)
	p := pkt.NewPacket(pInfo, f.Data[:])
	assert.NotEmpty(t, p.Candidates)
	p.AddDecoders()
	assert.Equal(t, len(p.Decoders), 1)
	decoder := p.Decoders[0]
	stream := pgn.NewDataStream(p.Data)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, pgn.BinarySwitchBankStatus{}, ret)
}

func TestPgn127501Write(t *testing.T) {
	raw := "2023-01-21T00:04:17Z,3,127501,224,0,8,00,03,c0,ff,ff,ff,ff,ff"
	f := converter.CanFrameFromRaw(raw)
	pInfo := NewPacketInfo(f)
	p := pkt.NewPacket(pInfo, f.Data[:])
	assert.NotEmpty(t, p.Candidates)
	p.AddDecoders()
	assert.Equal(t, len(p.Decoders), 1)
	decoder := p.Decoders[0]
	stream := pgn.NewDataStream(p.Data)
	ret, err := decoder(p.Info, stream)
	assert.Nil(t, err)
	assert.IsType(t, pgn.BinarySwitchBankStatus{}, ret)
}
