package actisenseendpoint

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.bug.st/serial"
)

type startupTestSerialPort struct {
	reads   [][]byte
	writes  [][]byte
	closed  bool
	reset   bool
	timeout time.Duration
}

func (p *startupTestSerialPort) Read(buffer []byte) (int, error) {
	if len(p.reads) == 0 {
		time.Sleep(100 * time.Microsecond)
		return 0, nil
	}
	read := p.reads[0]
	p.reads = p.reads[1:]
	return copy(buffer, read), nil
}

func (p *startupTestSerialPort) Write(data []byte) (int, error) {
	p.writes = append(p.writes, append([]byte(nil), data...))
	return len(data), nil
}

func (p *startupTestSerialPort) ResetInputBuffer() error {
	p.reset = true
	return nil
}

func (p *startupTestSerialPort) SetReadTimeout(timeout time.Duration) error {
	p.timeout = timeout
	return nil
}

func (p *startupTestSerialPort) Close() error {
	p.closed = true
	return nil
}

func commandResponse(t *testing.T, command byte, data []byte) []byte {
	t.Helper()
	payload := make([]byte, 12+len(data))
	payload[0] = command
	copy(payload[12:], data)
	encoded, err := encodeBDTP(bstNGTReceive, payload)
	require.NoError(t, err)
	return encoded
}

func decodedBDTPFrame(t *testing.T, encoded []byte) []byte {
	t.Helper()
	var frames [][]byte
	parser := bdtpParser{}
	parser.consume(encoded, func(frame []byte) {
		frames = append(frames, append([]byte(nil), frame...))
	})
	require.Len(t, frames, 1)
	return frames[0]
}

func requireBSTMessage(t *testing.T, encoded []byte, expectedID byte, expectedPayload []byte) {
	t.Helper()
	messageID, payload, err := decodeBSTEnvelope(decodedBDTPFrame(t, encoded))
	require.NoError(t, err)
	require.Equal(t, expectedID, messageID)
	require.Equal(t, expectedPayload, payload)
}

func startupResponses(t *testing.T, operatingMode uint16, address byte) []byte {
	t.Helper()
	modeData := make([]byte, 2)
	binary.LittleEndian.PutUint16(modeData, operatingMode)
	canData := make([]byte, 9)
	canData[8] = address
	return append(
		commandResponse(t, ngtOperatingMode, modeData),
		commandResponse(t, ngtCANConfig, canData)...,
	)
}

// ngtStartupResponsesWithBEMTrailer contains synthetic literal NGT-1 wire
// responses for OperatingMode and CANConfig. Each BEM frame has one trailing
// transport byte after the checksum, matching the response shape observed from
// physical NGT-1 hardware. Keep these bytes independent from encodeBDTP so the
// test covers compatibility with the device rather than our own encoder.
var ngtStartupResponsesWithBEMTrailer = []byte{
	dle, stx,
	bstNGTReceive, 0x0e,
	ngtOperatingMode, 0x01, 0x0e, 0x00, 0xda, 0x58, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00,
	0xfb, 0x00,
	dle, etx,
	dle, stx,
	bstNGTReceive, 0x19,
	ngtCANConfig, 0x01, 0x0e, 0x00, 0xda, 0x58, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x18, 0x17, 0x16, 0x15, 0x14, 0x13, 0x12, 0x11, 0x00, 0x00, 0x25, 0x01, 0x00,
	0xf7, 0x00,
	dle, etx,
}

func TestStartConfirmsReceiveAllModeBeforeOpening(t *testing.T) {
	staleMalformedFrame := []byte{dle, stx, bstNGTReceive, 2, 0, 0x5e, dle, etx}
	responses := append([]byte(nil), staleMalformedFrame...)
	responses = append(responses, startupResponses(t, ngtReceiveAll, 37)...)
	port := &startupTestSerialPort{reads: [][]byte{responses}}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.openPort = func(_ string, mode *serial.Mode) (serialPort, error) {
		require.Equal(t, defaultBaudRate, mode.BaudRate)
		return port, nil
	}

	require.NoError(t, ep.Start(context.Background()))
	require.True(t, port.reset)
	require.Equal(t, readTimeout, port.timeout)
	require.Len(t, port.writes, 3)
	require.Equal(t, endpoint.ExternalAddressState{Address: 37, Claimed: true}, ep.ExternalAddressState())
}

func TestStartAcceptsBEMResponseTrailerFromNGT1(t *testing.T) {
	port := &startupTestSerialPort{reads: [][]byte{ngtStartupResponsesWithBEMTrailer}}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.openPort = func(_ string, mode *serial.Mode) (serialPort, error) {
		require.Equal(t, defaultBaudRate, mode.BaudRate)
		return port, nil
	}

	require.NoError(t, ep.Start(context.Background()))
	require.Equal(t, endpoint.ExternalAddressState{Address: 37, Claimed: true}, ep.ExternalAddressState())
}

func TestStartAcceptsValidN2KTrafficWithoutBEMAcknowledgements(t *testing.T) {
	payload := []byte{
		3,
		0x11, 0xF8, 0x01,
		0xFF,
		0x22,
		0x78, 0x56, 0x34, 0x12,
		3,
		0x10, 0x20, 0x30,
	}
	traffic, err := encodeBDTP(bstN2KReceive, payload)
	require.NoError(t, err)
	port := &startupTestSerialPort{reads: [][]byte{traffic}}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.startupResponseTimeout = 2 * time.Millisecond
	ep.openPort = func(_ string, mode *serial.Mode) (serialPort, error) {
		require.Equal(t, defaultBaudRate, mode.BaudRate)
		return port, nil
	}

	require.NoError(t, ep.Start(context.Background()))
	require.False(t, port.closed)
	require.Equal(t, endpoint.ExternalAddressState{Address: 255}, ep.ExternalAddressState())
}

func TestDecodeBSTEnvelopeAcceptsN2KTransportTrailer(t *testing.T) {
	messageID, payload, err := decodeBSTEnvelope([]byte{bstN2KReceive, 0x00, 0x6d, 0x00})
	require.NoError(t, err)
	require.Equal(t, bstN2KReceive, messageID)
	require.Empty(t, payload)
}

func TestDecodeBSTEnvelopeRejectsIncompleteFrame(t *testing.T) {
	_, _, err := decodeBSTEnvelope([]byte{bstN2KReceive, 0x01, 0x00})
	require.ErrorContains(t, err, "actisense BST length is 1, frame contains 0")
}

func TestDecodeBSTEnvelopeRejectsChecksumFailureBeforeTrailer(t *testing.T) {
	_, _, err := decodeBSTEnvelope([]byte{bstN2KReceive, 0x00, 0x6c, 0x01})
	require.ErrorContains(t, err, "actisense BST checksum failed")
}

func TestStartRejectsFilteredOperatingMode(t *testing.T) {
	allPorts := []*startupTestSerialPort{
		{reads: [][]byte{startupResponses(t, 0, 37)}},
		{reads: [][]byte{startupResponses(t, 0, 37)}},
	}
	ports := append([]*startupTestSerialPort(nil), allPorts...)
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.startupResponseTimeout = 2 * time.Millisecond
	ep.openPort = func(_ string, _ *serial.Mode) (serialPort, error) {
		port := ports[0]
		ports = ports[1:]
		return port, nil
	}

	err := ep.Start(context.Background())
	require.ErrorContains(t, err, "gateway remained in filtered operating mode 0")
	for _, port := range allPorts {
		require.True(t, port.closed)
	}
}

func TestRefreshReceiveAllModeWritesSetAndQuery(t *testing.T) {
	port := &startupTestSerialPort{}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.port = port

	require.NoError(t, ep.refreshReceiveAllMode())
	require.Len(t, port.writes, 2)
	requireBSTMessage(t, port.writes[0], bstNGTSend,
		[]byte{ngtOperatingMode, byte(ngtReceiveAll), byte(ngtReceiveAll >> 8)})
	requireBSTMessage(t, port.writes[1], bstNGTSend, []byte{ngtOperatingMode})
}

func TestHandleFrameRestoresReceiveAllModeWhenGatewayReportsFiltered(t *testing.T) {
	port := &startupTestSerialPort{}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.port = port
	modeData := make([]byte, 2)
	binary.LittleEndian.PutUint16(modeData, 0)

	require.NoError(t, ep.handleFrame(decodedBDTPFrame(t,
		commandResponse(t, ngtOperatingMode, modeData))))
	require.Len(t, port.writes, 1)
	requireBSTMessage(t, port.writes[0], bstNGTSend,
		[]byte{ngtOperatingMode, byte(ngtReceiveAll), byte(ngtReceiveAll >> 8)})
}

func TestHandleFrameDoesNotLoopOnImmediateFilteredResponse(t *testing.T) {
	port := &startupTestSerialPort{}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.port = port
	require.NoError(t, ep.refreshReceiveAllMode())
	modeData := make([]byte, 2)
	binary.LittleEndian.PutUint16(modeData, 0)

	require.NoError(t, ep.handleFrame(decodedBDTPFrame(t,
		commandResponse(t, ngtOperatingMode, modeData))))
	require.Len(t, port.writes, 2)
}

func TestHandleFrameLeavesReceiveAllModeAlone(t *testing.T) {
	port := &startupTestSerialPort{}
	ep := New(logrus.New(), "/dev/test-ngt")
	ep.port = port
	modeData := make([]byte, 2)
	binary.LittleEndian.PutUint16(modeData, ngtReceiveAll)

	require.NoError(t, ep.handleFrame(decodedBDTPFrame(t,
		commandResponse(t, ngtOperatingMode, modeData))))
	require.Empty(t, port.writes)
}

func TestBDTPRoundTripEscapesDLE(t *testing.T) {
	encoded, err := encodeBDTP(bstN2KSend, []byte{2, 0x01, 0xF8, 0x01, 0xFF, 2, dle, 0x44})
	require.NoError(t, err)
	require.True(t, bytes.Contains(encoded, []byte{dle, dle}))

	var decoded [][]byte
	parser := bdtpParser{}
	parser.consume(encoded[:3], func(frame []byte) { decoded = append(decoded, frame) })
	parser.consume(encoded[3:], func(frame []byte) { decoded = append(decoded, frame) })
	require.Len(t, decoded, 1)
	require.Equal(t, bstN2KSend, decoded[0][0])
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
