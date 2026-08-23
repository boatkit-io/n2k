package actisenseendpoint

import (
	"bytes"
	"encoding/binary"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/stretchr/testify/require"
)

func TestBDTPRoundTripEscapesDLE(t *testing.T) {
	encoded, err := encodeBDTP(bstN2KSend, []byte{2, 0x01, 0xF8, 0x01, 0xFF, 2, dle, 0x44})
	require.NoError(t, err)
	require.True(t, bytes.Contains(encoded, []byte{dle, dle}))

	var decoded [][]byte
	parser := bdtpParser{}
	parser.consume(encoded[:3], func(frame []byte) { decoded = append(decoded, frame) })
	parser.consume(encoded[3:], func(frame []byte) { decoded = append(decoded, frame) })
	require.Len(t, decoded, 1)
	require.Equal(t, byte(bstN2KSend), decoded[0][0])
	require.Equal(t, byte(8), decoded[0][1])
	var checksum byte
	for _, value := range decoded[0] {
		checksum += value
	}
	require.Zero(t, checksum)
}

func TestDecodeBST93CompletePGN(t *testing.T) {
	payload := []byte{
		3,
		0x11, 0xF8, 0x01,
		0xFF,
		0x22,
		0x78, 0x56, 0x34, 0x12,
		3,
		0x10, 0x20, 0x30,
	}
	encoded, err := encodeBDTP(bstN2KReceive, payload)
	require.NoError(t, err)
	var body []byte
	parser := bdtpParser{}
	parser.consume(encoded, func(frame []byte) { body = frame })

	now := time.Unix(123, 456)
	message, err := decodeBST93(body, now)
	require.NoError(t, err)
	require.Equal(t, now, message.Timestamp)
	require.Equal(t, uint8(3), message.Priority)
	require.Equal(t, uint32(129041), message.PGN)
	require.Equal(t, uint8(0xFF), message.Destination)
	require.Equal(t, uint8(0x22), message.Source)
	require.Equal(t, []byte{0x10, 0x20, 0x30}, message.Data)
}

func TestDecodeBSTRejectsChecksumFailure(t *testing.T) {
	frame := []byte{bstN2KReceive, 0, 0}
	_, err := decodeBST93(frame, time.Now())
	require.ErrorContains(t, err, "checksum")
}

func TestDecodeCANConfigResponseUsesGatewayAddress(t *testing.T) {
	payload := make([]byte, 21)
	payload[0] = ngtCANConfig
	copy(payload[12:20], []byte{0x18, 0x17, 0x16, 0x15, 0x14, 0x13, 0x12, 0x11})
	payload[20] = 37
	encoded, err := encodeBDTP(bstNGTReceive, payload)
	require.NoError(t, err)
	var body []byte
	parser := bdtpParser{}
	parser.consume(encoded, func(frame []byte) { body = frame })

	message, state, err := decodeBST(body, time.Now())
	require.NoError(t, err)
	require.Nil(t, message)
	require.Equal(t, uint8(37), state.Address)
	require.True(t, state.Claimed)
}

func TestDecodeCANConfigResponseUsesExtendedAddressState(t *testing.T) {
	payload := make([]byte, 24)
	payload[0] = ngtCANConfig
	payload[22] = 41
	payload[23] = 1
	encoded, err := encodeBDTP(bstNGTReceive, payload)
	require.NoError(t, err)
	var body []byte
	parser := bdtpParser{}
	parser.consume(encoded, func(frame []byte) { body = frame })

	_, state, err := decodeBST(body, time.Now())
	require.NoError(t, err)
	require.Equal(t, &endpoint.ExternalAddressState{Address: 41, Claimed: true}, state)
}

func TestBST94PayloadOmitsSourceAndCarriesDestination(t *testing.T) {
	message := endpoint.PGNMessage{
		Priority:    3,
		PGN:         127250,
		Source:      99,
		Destination: 42,
		Data:        []byte{1, 2},
	}
	require.Equal(t, []byte{3, 0x12, 0xF1, 0x01, 42, 2, 1, 2}, bst94Payload(message))
	require.Equal(t, []byte{ngtEnableTxPGN, 0x12, 0xF1, 0x01, 0, 1}, enableTransmitPGNCommand(message.PGN))
}

func TestDecodeCANConfigResponseRejectsDeviceError(t *testing.T) {
	payload := make([]byte, 21)
	payload[0] = ngtCANConfig
	binary.LittleEndian.PutUint32(payload[8:12], 0xfffffb7a)
	encoded, err := encodeBDTP(bstNGTReceive, payload)
	require.NoError(t, err)
	var body []byte
	parser := bdtpParser{}
	parser.consume(encoded, func(frame []byte) { body = frame })

	_, _, err = decodeBST(body, time.Now())
	require.ErrorContains(t, err, "CAN config query failed")
}
