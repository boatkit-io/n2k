// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.

// Package actisenseendpoint implements the Actisense NGT-1 BDTP/BST serial
// protocol. The NGT transports complete NMEA 2000 messages rather than raw CAN
// frames and owns its NMEA 2000 source address on behalf of the host software.
package actisenseendpoint

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/boatkit-io/n2k/pkg/endpoint"
	"github.com/brutella/can"
	"github.com/sirupsen/logrus"
	"go.bug.st/serial"
)

const (
	defaultBaudRate     = 115200
	alternateBaudRate   = 230400
	startupTimeout      = 2 * time.Second
	addressPollInterval = 5 * time.Second
	readTimeout         = 250 * time.Millisecond

	dle = byte(0x10)
	stx = byte(0x02)
	etx = byte(0x03)

	bstN2KReceive = byte(0x93)
	bstN2KSend    = byte(0x94)
	bstNGTReceive = byte(0xA0)
	bstNGTSend    = byte(0xA1)

	ngtOperatingMode  = byte(0x11)
	ngtReceiveAll     = uint16(2)
	ngtCANConfig      = byte(0x42)
	ngtEnableTxPGN    = byte(0x47)
	ngtActivatePGNs   = byte(0x4B)
	maxPGNDataLength  = 223
	maxBSTFrameLength = 259 // Command, length, 255-byte payload, checksum, transport trailer.
)

type serialPort interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	ResetInputBuffer() error
	SetReadTimeout(time.Duration) error
	Close() error
}

type serialPortOpener func(string, *serial.Mode) (serialPort, error)

// Endpoint is an Actisense NGT-1 serial endpoint.
type Endpoint struct {
	log        *logrus.Logger
	serialPort string

	startMu                sync.Mutex
	writeMu                sync.Mutex
	portMu                 sync.RWMutex
	port                   serialPort
	openPort               serialPortOpener
	startupResponseTimeout time.Duration

	handlerMu sync.RWMutex
	handler   endpoint.MessageHandler
	addressMu sync.RWMutex
	address   endpoint.ExternalAddressState
	addressFn func(endpoint.ExternalAddressState)
	txMu      sync.Mutex
	txPGNs    map[uint32]struct{}
	parser    bdtpParser
	closed    atomic.Bool
}

// New constructs an NGT-1 endpoint for serialPortName.
func New(log *logrus.Logger, serialPortName string) *Endpoint {
	return &Endpoint{
		log:        log,
		serialPort: serialPortName,
		address:    endpoint.ExternalAddressState{Address: 255},
		txPGNs:     make(map[uint32]struct{}),
		openPort: func(name string, mode *serial.Mode) (serialPort, error) {
			return serial.Open(name, mode)
		},
		startupResponseTimeout: startupTimeout,
	}
}

// Start opens the serial port and places the NGT-1 in receive-all mode.
func (e *Endpoint) Start(ctx context.Context) error {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	if e.closed.Load() {
		return errors.New("actisense endpoint is closed")
	}
	if e.currentPort() != nil {
		return nil
	}
	var startupErrors []error
	for _, baudRate := range actisenseBaudRates {
		err := e.openAtBaudLocked(ctx, baudRate)
		if err == nil {
			return nil
		}
		startupErrors = append(startupErrors, err)
		if err := ctx.Err(); err != nil {
			return errors.Join(err, errors.Join(startupErrors...))
		}
	}
	return fmt.Errorf("start Actisense NGT-1: %w", errors.Join(startupErrors...))
}

func (e *Endpoint) openAtBaudLocked(ctx context.Context, baudRate int) error {
	port, err := e.openPort(e.serialPort, &serial.Mode{
		BaudRate: baudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		return fmt.Errorf("open Actisense NGT-1 serial port %s: %w", e.serialPort, err)
	}
	if err := port.SetReadTimeout(readTimeout); err != nil {
		_ = port.Close()
		return fmt.Errorf("set Actisense NGT-1 read timeout: %w", err)
	}
	if err := port.ResetInputBuffer(); err != nil {
		_ = port.Close()
		return fmt.Errorf("clear stale Actisense NGT-1 serial input: %w", err)
	}
	e.portMu.Lock()
	e.port = port
	e.portMu.Unlock()
	e.parser = bdtpParser{}

	command := []byte{ngtOperatingMode, byte(ngtReceiveAll), byte(ngtReceiveAll >> 8)}
	if err := e.writeBST(bstNGTSend, command); err != nil {
		e.closePort(port)
		return fmt.Errorf("configure Actisense NGT-1 receive-all mode: %w", err)
	}
	if err := e.writeBST(bstNGTSend, []byte{ngtOperatingMode}); err != nil {
		e.closePort(port)
		return fmt.Errorf("query Actisense NGT-1 operating mode: %w", err)
	}
	if err := e.writeBST(bstNGTSend, []byte{ngtCANConfig}); err != nil {
		e.closePort(port)
		return fmt.Errorf("query Actisense NGT-1 CAN address: %w", err)
	}
	if err := e.confirmStartup(ctx, port); err != nil {
		e.closePort(port)
		return fmt.Errorf("confirm Actisense NGT-1 startup at %d baud: %w", baudRate, err)
	}
	e.log.Infof("Actisense NGT-1 serial link opened at %d baud", baudRate)
	return nil
}

func (e *Endpoint) closePort(port serialPort) {
	e.portMu.Lock()
	e.port = nil
	e.portMu.Unlock()
	_ = port.Close()
	e.storeExternalAddress(endpoint.ExternalAddressState{Address: 255})
	e.txMu.Lock()
	clear(e.txPGNs)
	e.txMu.Unlock()
}

type startupConfirmation struct {
	operatingMode      *uint16
	operatingModeErr   error
	canConfigErr       error
	canConfigReceived  bool
	n2kTrafficReceived bool
}

func (e *Endpoint) observeStartupFrame(frame []byte, confirmation *startupConfirmation) {
	messageID, payload, err := decodeBSTEnvelope(frame)
	if err != nil {
		e.log.WithError(err).Warn("Discarding invalid Actisense NGT-1 startup message")
		return
	}
	if messageID == bstN2KReceive {
		if message, _, messageErr := decodeBST(frame, time.Now()); messageErr == nil && message != nil {
			confirmation.n2kTrafficReceived = true
		}
		return
	}
	if messageID != bstNGTReceive {
		return
	}
	if mode, matched, modeErr := decodeOperatingModeResponse(payload); matched {
		if modeErr != nil {
			confirmation.operatingModeErr = modeErr
			return
		}
		confirmation.operatingMode = &mode
		confirmation.operatingModeErr = nil
		return
	}
	if len(payload) == 0 || payload[0] != ngtCANConfig {
		return
	}
	state, stateErr := decodeCANConfigResponse(payload)
	if stateErr != nil {
		confirmation.canConfigErr = stateErr
		return
	}
	if state != nil {
		e.storeExternalAddress(*state)
		confirmation.canConfigReceived = true
		confirmation.canConfigErr = nil
	}
}

func (e *Endpoint) confirmStartup(ctx context.Context, port serialPort) error {
	timeout := e.startupResponseTimeout
	if timeout <= 0 {
		timeout = startupTimeout
	}
	deadline := time.Now().Add(timeout)
	buffer := make([]byte, 1024)
	confirmation := startupConfirmation{}
	for time.Now().Before(deadline) {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, err := port.Read(buffer)
		if read > 0 {
			e.parser.consume(buffer[:read], func(frame []byte) {
				e.observeStartupFrame(frame, &confirmation)
			})
		}
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if e.closed.Load() {
				return errors.New("actisense endpoint closed during startup")
			}
			return fmt.Errorf("read Actisense NGT-1 startup response: %w", err)
		}
		if confirmation.operatingMode != nil && *confirmation.operatingMode == ngtReceiveAll && confirmation.canConfigReceived {
			return nil
		}
	}
	if confirmation.operatingModeErr != nil {
		return confirmation.operatingModeErr
	}
	if confirmation.operatingMode != nil && *confirmation.operatingMode != ngtReceiveAll {
		return fmt.Errorf("gateway remained in filtered operating mode %d", *confirmation.operatingMode)
	}
	if confirmation.canConfigErr != nil {
		return confirmation.canConfigErr
	}
	if confirmation.n2kTrafficReceived {
		e.log.Warn("Actisense NGT-1 did not acknowledge startup queries; continuing after receiving valid NMEA 2000 traffic")
		return nil
	}
	if confirmation.operatingMode == nil {
		return errors.New("receive-all operating mode was not acknowledged")
	}
	return errors.New("CAN address response was not received")
}

// Run reads and decodes NGT-1 messages until cancellation or closure.
func (e *Endpoint) Run(ctx context.Context) error {
	if err := e.Start(ctx); err != nil {
		return err
	}
	buffer := make([]byte, 1024)
	nextAddressPoll := time.Now().Add(addressPollInterval)
	e.notifyExternalAddress()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		port := e.currentPort()
		if port == nil {
			return nil
		}
		read, err := port.Read(buffer)
		if read > 0 {
			e.parser.consume(buffer[:read], func(frame []byte) {
				e.handleFrame(frame)
			})
		}
		if err != nil {
			if e.closed.Load() || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read Actisense NGT-1 serial data: %w", err)
		}
		if !time.Now().Before(nextAddressPoll) {
			if err := e.writeBST(bstNGTSend, []byte{ngtCANConfig}); err != nil {
				return fmt.Errorf("refresh Actisense NGT-1 CAN address: %w", err)
			}
			nextAddressPoll = time.Now().Add(addressPollInterval)
		}
	}
}

// Close stops serial I/O.
func (e *Endpoint) Close() error {
	e.closed.Store(true)
	e.startMu.Lock()
	defer e.startMu.Unlock()
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	e.portMu.Lock()
	port := e.port
	e.port = nil
	e.portMu.Unlock()
	if port != nil {
		err := port.Close()
		e.publishExternalAddress(endpoint.ExternalAddressState{Address: 255})
		return err
	}
	e.publishExternalAddress(endpoint.ExternalAddressState{Address: 255})
	return nil
}

// SetOutput installs the decoded-message consumer.
func (e *Endpoint) SetOutput(handler endpoint.MessageHandler) {
	e.handlerMu.Lock()
	e.handler = handler
	e.handlerMu.Unlock()
}

// ExternalAddressState reports the source address currently claimed by the
// NGT-1 for host-originated traffic.
func (e *Endpoint) ExternalAddressState() endpoint.ExternalAddressState {
	e.addressMu.RLock()
	defer e.addressMu.RUnlock()
	return e.address
}

// SetExternalAddressHandler installs the address-state consumer.
func (e *Endpoint) SetExternalAddressHandler(handler func(endpoint.ExternalAddressState)) {
	e.addressMu.Lock()
	e.addressFn = handler
	e.addressMu.Unlock()
}

func (e *Endpoint) publishExternalAddress(state endpoint.ExternalAddressState) {
	e.addressMu.Lock()
	if e.address == state {
		e.addressMu.Unlock()
		return
	}
	e.address = state
	handler := e.addressFn
	e.addressMu.Unlock()
	if handler != nil {
		handler(state)
	}
}

func (e *Endpoint) storeExternalAddress(state endpoint.ExternalAddressState) {
	e.addressMu.Lock()
	e.address = state
	e.addressMu.Unlock()
}

func (e *Endpoint) notifyExternalAddress() {
	e.addressMu.RLock()
	state := e.address
	handler := e.addressFn
	e.addressMu.RUnlock()
	if handler != nil {
		handler(state)
	}
}

// WriteFrame is retained for the generic endpoint contract. NGT-1 traffic must
// be sent as complete PGNs through WritePGN.
func (e *Endpoint) WriteFrame(can.Frame) {
	e.log.Error("Actisense NGT-1 cannot transmit an individual raw CAN frame")
}

// WritePGN sends one complete NMEA 2000 message using BST-94. The NGT-1 owns
// the source address, so message.Source is intentionally not serialized.
func (e *Endpoint) WritePGN(message endpoint.PGNMessage) error {
	if message.Priority > 7 {
		return fmt.Errorf("invalid NMEA 2000 priority %d", message.Priority)
	}
	if message.PGN == 0 || message.PGN > 0x3FFFF {
		return fmt.Errorf("invalid NMEA 2000 PGN %d", message.PGN)
	}
	if len(message.Data) == 0 || len(message.Data) > maxPGNDataLength {
		return fmt.Errorf("invalid NMEA 2000 data length %d", len(message.Data))
	}
	state := e.ExternalAddressState()
	if !state.Claimed {
		return errors.New("actisense NGT-1 has not claimed an NMEA 2000 address")
	}
	e.txMu.Lock()
	defer e.txMu.Unlock()
	if _, enabled := e.txPGNs[message.PGN]; !enabled {
		command := enableTransmitPGNCommand(message.PGN)
		if err := e.writeBST(bstNGTSend, command); err != nil {
			return fmt.Errorf("enable Actisense NGT-1 transmit PGN %d: %w", message.PGN, err)
		}
		if err := e.writeBST(bstNGTSend, []byte{ngtActivatePGNs}); err != nil {
			return fmt.Errorf("activate Actisense NGT-1 transmit PGN %d: %w", message.PGN, err)
		}
		e.txPGNs[message.PGN] = struct{}{}
	}
	return e.writeBST(bstN2KSend, bst94Payload(message))
}

func enableTransmitPGNCommand(pgn uint32) []byte {
	command := make([]byte, 6)
	command[0] = ngtEnableTxPGN
	binary.LittleEndian.PutUint32(command[1:5], pgn)
	command[5] = 1
	return command
}

func bst94Payload(message endpoint.PGNMessage) []byte {
	payload := make([]byte, 6+len(message.Data))
	payload[0] = message.Priority
	pgnBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(pgnBytes, message.PGN)
	copy(payload[1:4], pgnBytes[:3])
	payload[4] = message.Destination
	payload[5] = byte(len(message.Data)) //nolint:gosec // WritePGN limits data to 223 bytes.
	copy(payload[6:], message.Data)
	return payload
}

func (e *Endpoint) currentPort() serialPort {
	e.portMu.RLock()
	defer e.portMu.RUnlock()
	return e.port
}

func (e *Endpoint) writeBST(messageID byte, payload []byte) error {
	frame, err := encodeBDTP(messageID, payload)
	if err != nil {
		return err
	}
	e.writeMu.Lock()
	defer e.writeMu.Unlock()
	port := e.currentPort()
	if port == nil || e.closed.Load() {
		return errors.New("actisense NGT-1 serial port is not open")
	}
	for len(frame) > 0 {
		written, writeErr := port.Write(frame)
		if writeErr != nil {
			return fmt.Errorf("write Actisense NGT-1 serial data: %w", writeErr)
		}
		if written <= 0 {
			return errors.New("write Actisense NGT-1 serial data: no progress")
		}
		frame = frame[written:]
	}
	return nil
}

func (e *Endpoint) handleFrame(frame []byte) bool {
	message, address, err := decodeBST(frame, time.Now())
	if err != nil {
		e.log.WithError(err).Warn("Discarding invalid Actisense NGT-1 message")
		return false
	}
	if address != nil {
		e.publishExternalAddress(*address)
	}
	if message == nil {
		return true
	}
	e.handlerMu.RLock()
	handler := e.handler
	e.handlerMu.RUnlock()
	if handler != nil {
		handler.HandleMessage(message)
	}
	return true
}

func encodeBDTP(messageID byte, payload []byte) ([]byte, error) {
	if len(payload) > 255 {
		return nil, fmt.Errorf("actisense BST payload is too long: %d", len(payload))
	}
	body := make([]byte, 0, len(payload)+3)
	body = append(body, messageID, byte(len(payload))) //nolint:gosec // The payload length is checked above.
	body = append(body, payload...)
	var checksum byte
	for _, value := range body {
		checksum += value
	}
	body = append(body, 0-checksum)

	frame := []byte{dle, stx}
	for _, value := range body {
		frame = append(frame, value)
		if value == dle {
			frame = append(frame, dle)
		}
	}
	return append(frame, dle, etx), nil
}

func decodeBST93(frame []byte, timestamp time.Time) (*endpoint.PGNMessage, error) {
	message, _, err := decodeBST(frame, timestamp)
	return message, err
}

func decodeBST(frame []byte, timestamp time.Time) (*endpoint.PGNMessage, *endpoint.ExternalAddressState, error) {
	messageID, payload, err := decodeBSTEnvelope(frame)
	if err != nil {
		return nil, nil, err
	}
	if messageID == bstNGTReceive {
		state, err := decodeCANConfigResponse(payload)
		return nil, state, err
	}
	if messageID != bstN2KReceive {
		return nil, nil, fmt.Errorf("unsupported Actisense BST message 0x%02x", messageID)
	}
	if len(payload) < 11 {
		return nil, nil, fmt.Errorf("actisense BST-93 payload is too short: %d", len(payload))
	}
	dataLength := int(payload[10])
	if len(payload) != 11+dataLength {
		return nil, nil, fmt.Errorf("actisense BST-93 data length is %d, payload contains %d", dataLength, len(payload)-11)
	}
	return &endpoint.PGNMessage{
		Timestamp:   timestamp,
		Priority:    payload[0] & 0x07,
		PGN:         uint32(payload[1]) | uint32(payload[2])<<8 | uint32(payload[3])<<16,
		Destination: payload[4],
		Source:      payload[5],
		Data:        append([]byte(nil), payload[11:]...),
	}, nil, nil
}

func decodeBSTEnvelope(frame []byte) (messageID byte, payload []byte, err error) {
	if len(frame) < 3 {
		return 0, nil, errors.New("actisense BST frame is too short")
	}
	declaredPayloadLength := int(frame[1])
	messageLength := declaredPayloadLength + 3 // Command, length, payload, checksum.
	if len(frame) < messageLength {
		return 0, nil, fmt.Errorf("actisense BST length is %d, frame contains %d", frame[1], len(frame)-3)
	}
	var checksum byte
	for _, value := range frame[:messageLength] {
		checksum += value
	}
	if checksum != 0 {
		return 0, nil, fmt.Errorf("actisense BST checksum failed: 0x%02x", checksum)
	}
	// Some NGT-1 firmware appends transport bytes after the checksum. The BST
	// store length identifies the checksum boundary; Actisense's SDK validates
	// that declared message and ignores any remaining bytes in the BDTP frame.
	return frame[0], frame[2 : messageLength-1], nil
}

func decodeCANConfigResponse(payload []byte) (*endpoint.ExternalAddressState, error) {
	data, matched, err := decodeCommandResponse(payload, ngtCANConfig)
	if err != nil {
		return nil, fmt.Errorf("actisense CAN config query failed: %w", err)
	}
	if !matched {
		return nil, err
	}
	state := endpoint.ExternalAddressState{Address: 255}
	// Current SDKs return NAME[8] + current source. Older NGT firmware may
	// append preferred/previous/current/claim fields; accept both shapes.
	if len(data) < 9 {
		return nil, fmt.Errorf("actisense CAN config response is too short: %d", len(payload))
	}
	if len(data) >= 12 && data[11] <= 1 && data[10] <= 252 {
		state.Address = data[10]
		state.Claimed = data[11] == 1 && state.Address <= 251
	} else {
		state.Address = data[8]
		state.Claimed = state.Address <= 251
	}
	return &state, nil
}

func decodeOperatingModeResponse(payload []byte) (mode uint16, matched bool, err error) {
	data, matched, err := decodeCommandResponse(payload, ngtOperatingMode)
	if err != nil || !matched {
		return 0, matched, err
	}
	if len(data) < 2 {
		return 0, true, fmt.Errorf("actisense operating mode response is too short: %d", len(payload))
	}
	return binary.LittleEndian.Uint16(data[:2]), true, nil
}

func decodeCommandResponse(payload []byte, expectedCommand byte) (data []byte, matched bool, err error) {
	if len(payload) == 0 || payload[0] != expectedCommand {
		return nil, false, nil
	}
	// BEM responses contain the command ID followed by sequence/model/serial
	// metadata and a four-byte error code. Command data begins at offset 12.
	if len(payload) < 12 {
		return nil, true, fmt.Errorf("actisense command 0x%02x response is too short: %d", expectedCommand, len(payload))
	}
	if errorCode := binary.LittleEndian.Uint32(payload[8:12]); errorCode != 0 {
		return nil, true, fmt.Errorf("actisense command 0x%02x failed: 0x%08x", expectedCommand, errorCode)
	}
	return payload[12:], true, nil
}

type bdtpParser struct {
	buffer  []byte
	escaped bool
	inFrame bool
}

func (p *bdtpParser) consume(data []byte, handler func([]byte)) {
	for _, value := range data {
		if !p.inFrame {
			if p.escaped && value == stx {
				p.inFrame = true
				p.buffer = p.buffer[:0]
			}
			p.escaped = value == dle
			continue
		}
		if p.escaped {
			switch value {
			case dle:
				if len(p.buffer) >= maxBSTFrameLength {
					p.inFrame = false
					p.buffer = p.buffer[:0]
					p.escaped = false
					continue
				}
				p.buffer = append(p.buffer, dle)
			case etx:
				handler(append([]byte(nil), p.buffer...))
				p.inFrame = false
				p.buffer = p.buffer[:0]
			case stx:
				p.buffer = p.buffer[:0]
			default:
				p.inFrame = false
				p.buffer = p.buffer[:0]
			}
			p.escaped = false
			continue
		}
		if value == dle {
			p.escaped = true
		} else {
			if len(p.buffer) >= maxBSTFrameLength {
				p.inFrame = false
				p.buffer = p.buffer[:0]
				continue
			}
			p.buffer = append(p.buffer, value)
		}
	}
}

var _ endpoint.Endpoint = (*Endpoint)(nil)
var _ endpoint.PGNWriter = (*Endpoint)(nil)
var _ endpoint.ExternalAddressProvider = (*Endpoint)(nil)

var actisenseBaudRates = [...]int{defaultBaudRate, alternateBaudRate}
