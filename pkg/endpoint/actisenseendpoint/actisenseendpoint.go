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
	baudProbeInterval   = 1500 * time.Millisecond
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
	maxBSTFrameLength = 258
)

// Endpoint is an Actisense NGT-1 serial endpoint.
type Endpoint struct {
	log        *logrus.Logger
	serialPort string

	startMu sync.Mutex
	writeMu sync.Mutex
	portMu  sync.RWMutex
	port    serial.Port

	handlerMu sync.RWMutex
	handler   endpoint.MessageHandler
	addressMu sync.RWMutex
	address   endpoint.ExternalAddressState
	addressFn func(endpoint.ExternalAddressState)
	txMu      sync.Mutex
	txPGNs    map[uint32]struct{}
	parser    bdtpParser
	baudIndex int
	closed    atomic.Bool
}

// New constructs an NGT-1 endpoint for serialPort.
func New(log *logrus.Logger, serialPort string) *Endpoint {
	return &Endpoint{
		log:        log,
		serialPort: serialPort,
		address:    endpoint.ExternalAddressState{Address: 255},
		txPGNs:     make(map[uint32]struct{}),
	}
}

// Start opens the serial port and places the NGT-1 in receive-all mode.
func (e *Endpoint) Start(context.Context) error {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	if e.closed.Load() {
		return errors.New("actisense endpoint is closed")
	}
	if e.currentPort() != nil {
		return nil
	}
	e.baudIndex = 0
	return e.openAtBaudLocked(actisenseBaudRates[e.baudIndex])
}

func (e *Endpoint) openAtBaudLocked(baudRate int) error {
	port, err := serial.Open(e.serialPort, &serial.Mode{
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
	e.portMu.Lock()
	e.port = port
	e.portMu.Unlock()

	command := []byte{ngtOperatingMode, byte(ngtReceiveAll), byte(ngtReceiveAll >> 8)}
	if err := e.writeBST(bstNGTSend, command); err != nil {
		e.portMu.Lock()
		e.port = nil
		e.portMu.Unlock()
		_ = port.Close()
		return fmt.Errorf("configure Actisense NGT-1 receive-all mode: %w", err)
	}
	if err := e.writeBST(bstNGTSend, []byte{ngtCANConfig}); err != nil {
		e.portMu.Lock()
		e.port = nil
		e.portMu.Unlock()
		_ = port.Close()
		return fmt.Errorf("query Actisense NGT-1 CAN address: %w", err)
	}
	e.log.Infof("Actisense NGT-1 serial link opened at %d baud", baudRate)
	return nil
}

// Run reads and decodes NGT-1 messages until cancellation or closure.
func (e *Endpoint) Run(ctx context.Context) error {
	if err := e.Start(ctx); err != nil {
		return err
	}
	buffer := make([]byte, 1024)
	probeStarted := time.Now()
	nextAddressPoll := time.Now().Add(addressPollInterval)
	baudConfirmed := false
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
				if e.handleFrame(frame) {
					baudConfirmed = true
				}
			})
		}
		if err != nil {
			if e.closed.Load() || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("read Actisense NGT-1 serial data: %w", err)
		}
		if !baudConfirmed && time.Since(probeStarted) >= baudProbeInterval {
			if switchErr := e.switchBaud(); switchErr != nil {
				return switchErr
			}
			probeStarted = time.Now()
			nextAddressPoll = time.Now().Add(addressPollInterval)
		}
		if baudConfirmed && !time.Now().Before(nextAddressPoll) {
			if err := e.writeBST(bstNGTSend, []byte{ngtCANConfig}); err != nil {
				return fmt.Errorf("refresh Actisense NGT-1 CAN address: %w", err)
			}
			nextAddressPoll = time.Now().Add(addressPollInterval)
		}
	}
}

func (e *Endpoint) switchBaud() error {
	e.startMu.Lock()
	defer e.startMu.Unlock()
	if e.closed.Load() {
		return nil
	}
	e.writeMu.Lock()
	e.portMu.Lock()
	port := e.port
	e.port = nil
	e.portMu.Unlock()
	if port != nil {
		_ = port.Close()
	}
	e.writeMu.Unlock()
	e.parser = bdtpParser{}
	e.publishExternalAddress(endpoint.ExternalAddressState{Address: 255})
	e.txMu.Lock()
	clear(e.txPGNs)
	e.txMu.Unlock()
	e.baudIndex = (e.baudIndex + 1) % len(actisenseBaudRates)
	if err := e.openAtBaudLocked(actisenseBaudRates[e.baudIndex]); err != nil {
		return fmt.Errorf("probe alternate Actisense NGT-1 baud rate: %w", err)
	}
	return nil
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

func (e *Endpoint) currentPort() serial.Port {
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
	if len(frame) < 3 {
		return nil, nil, errors.New("actisense BST frame is too short")
	}
	var checksum byte
	for _, value := range frame {
		checksum += value
	}
	if checksum != 0 {
		return nil, nil, fmt.Errorf("actisense BST checksum failed: 0x%02x", checksum)
	}
	if int(frame[1]) != len(frame)-3 {
		return nil, nil, fmt.Errorf("actisense BST length is %d, expected %d", frame[1], len(frame)-3)
	}
	if frame[0] == bstNGTReceive {
		state, err := decodeCANConfigResponse(frame[2 : len(frame)-1])
		return nil, state, err
	}
	if frame[0] != bstN2KReceive {
		return nil, nil, fmt.Errorf("unsupported Actisense BST message 0x%02x", frame[0])
	}
	payload := frame[2 : len(frame)-1]
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

func decodeCANConfigResponse(payload []byte) (*endpoint.ExternalAddressState, error) {
	if len(payload) == 0 || payload[0] != ngtCANConfig {
		return nil, nil
	}
	// BEM responses contain the command ID followed by sequence/model/serial
	// metadata and a four-byte error code. Command data begins at offset 12.
	if len(payload) < 21 {
		return nil, fmt.Errorf("actisense CAN config response is too short: %d", len(payload))
	}
	if errorCode := binary.LittleEndian.Uint32(payload[8:12]); errorCode != 0 {
		return nil, fmt.Errorf("actisense CAN config query failed: 0x%08x", errorCode)
	}
	data := payload[12:]
	state := endpoint.ExternalAddressState{Address: 255}
	// Current SDKs return NAME[8] + current source. Older NGT firmware may
	// append preferred/previous/current/claim fields; accept both shapes.
	if len(data) >= 12 && data[11] <= 1 && data[10] <= 252 {
		state.Address = data[10]
		state.Claimed = data[11] == 1 && state.Address <= 251
	} else {
		state.Address = data[8]
		state.Claimed = state.Address <= 251
	}
	return &state, nil
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
