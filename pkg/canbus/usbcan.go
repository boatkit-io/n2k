package canbus

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/brutella/can"
	"go.bug.st/serial"
)

// USB-CAN Analyzer Protocol Documentation:
//
// This file implements the USB-CAN Analyzer protocol, a cheap USB-to-CAN dongle commonly
// sold under names like "USB-CAN Analyzer" or "Seeed USB CAN Analyzer". The device
// communicates over a virtual serial port using a proprietary binary framing protocol.
//
// Protocol references:
// * https://github.com/SeeedDocument/USB-CAN-Analyzer/blob/master/res/Document/USB%20(Serial%20port)%20to%20CAN%20protocol%20defines.pdf
// * https://github.com/SeeedDocument/USB-CAN-Analyzer?tab=readme-ov-file
// * https://github.com/kobolt/usb-can/blob/master/canusb.c
//
// Frame format overview:
//   - Every frame starts with 0xAA (start-of-frame marker)
//   - Every data/command frame ends with 0x55 (end-of-frame marker)
//   - There are two major frame categories:
//     1. Command frames: 0xAA 0x55 followed by 18 bytes of settings/response data (20 bytes total)
//     2. Data frames: 0xAA <info_byte> <id_bytes> <data_bytes> 0x55
//
// Data frame info byte layout (byte[1]):
//   - Bits [7:6] = 0b11 indicates a data frame
//   - Bit  [5]   = 1 for extended (29-bit) CAN ID, 0 for standard (11-bit) CAN ID
//   - Bit  [4]   = 1 for remote frame, 0 for data frame
//   - Bits [3:0] = data length (0-8 bytes of CAN payload)

// CANUSBMode is an enum for the CANUSB device's operating modes.
// These modes control how the CAN transceiver behaves on the bus.
type CANUSBMode byte

const (
	// ModeNormal is standard operating mode -- the device transmits and receives normally on the CAN bus.
	ModeNormal CANUSBMode = 0
	// ModeLoopback causes transmitted frames to be looped back as received frames (for testing without a bus).
	ModeLoopback CANUSBMode = 1
	// ModeSilent puts the device in listen-only mode -- it receives but never transmits ACKs or frames.
	ModeSilent CANUSBMode = 2
	// ModeLoopbackSilent combines loopback and silent modes (useful for self-test without bus interaction).
	ModeLoopbackSilent CANUSBMode = 3
)

// CANUSBFrame is an enum for the CAN frame type used in the settings frame to tell
// the device whether to operate in standard (11-bit ID) or extended (29-bit ID) mode.
type CANUSBFrame byte

const (
	// FrameStandard configures the device for standard 11-bit CAN IDs.
	FrameStandard CANUSBFrame = 1
	// FrameExtended configures the device for extended 29-bit CAN IDs (used by NMEA 2000).
	FrameExtended CANUSBFrame = 2
)

// USBCANChannelOptions contains the configuration required to open and operate a USB-CAN channel.
type USBCANChannelOptions struct {
	// SerialPortName is the OS path to the serial port device, e.g. "/dev/ttyUSB0" on Linux
	// or "/dev/cu.usbserial-1234" on macOS.
	SerialPortName string

	// SerialBaudRate is the baud rate for the serial (UART) connection between the host and
	// the USB-CAN dongle. This is NOT the CAN bus bitrate -- it controls only the USB serial link.
	// The USB-CAN Analyzer typically uses 2000000 (2 Mbaud).
	SerialBaudRate int

	// BitRate is the CAN bus bitrate in bits per second. Common values for NMEA 2000 are 250000.
	// This value is mapped to a protocol-specific byte code in the settings frame sent to the device.
	BitRate int

	// FrameHandler is the callback function invoked for each successfully parsed CAN frame.
	// The handler receives a can.Frame with the CAN ID and up to 8 bytes of payload data.
	FrameHandler can.HandlerFunc
}

// USBCANChannel represents a single USB-CAN-based canbus channel for sending/receiving CAN frames.
// It manages the serial port connection, sends the initial configuration frame, and continuously
// reads and parses the proprietary USB-CAN binary protocol from the serial stream.
type USBCANChannel struct {
	// options holds the user-provided configuration (serial port, bitrate, handler, etc.)
	options USBCANChannelOptions

	// port is the open serial port connection to the USB-CAN dongle.
	// It is nil until Run() is called.
	port serial.Port

	// log is the structured logger for debug/info messages about frame parsing and errors.
	log *slog.Logger
}

// NewUSBCANChannel creates and returns a new USBCANChannel configured with the given options.
// The channel is not opened until Run() is called.
//
// Parameters:
//   - log: structured logger for diagnostic output
//   - options: required configuration including serial port path, baud rate, CAN bitrate, and frame handler
//
// Returns an Interface so callers can use it polymorphically with SocketCANChannel.
func NewUSBCANChannel(log *slog.Logger, options USBCANChannelOptions) Interface {
	c := USBCANChannel{
		options: options,
		log:     log,
	}

	return &c
}

// Run opens the serial port, sends the initial configuration/settings frame to set the CAN bitrate,
// and then enters a blocking read loop that continuously reads bytes from the serial port,
// accumulates them in a buffer, and parses complete USB-CAN frames as they arrive.
//
// The read loop uses a 32-byte working buffer per read call. Since USB-CAN frames can span
// multiple reads (or multiple frames can arrive in a single read), the pending buffer accumulates
// partial data across iterations.
//
// This method blocks indefinitely and only returns on a read error or serial port failure.
func (c *USBCANChannel) Run(ctx context.Context) error {
	// Configure and open the serial port at the specified baud rate.
	// The USB-CAN Analyzer typically runs at 2 Mbaud on the serial link.
	mode := &serial.Mode{
		BaudRate: c.options.SerialBaudRate,
	}
	port, err := serial.Open(c.options.SerialPortName, mode)
	if err != nil {
		return err
	}

	c.port = port

	// Send the 20-byte settings frame to configure the device's CAN bitrate, frame type, and mode.
	// This must happen before we start reading, as the device won't produce data frames until configured.
	if err := c.sendSettingsFrame(); err != nil {
		return err
	}

	c.log.Info("Opened USBCAN and listening", "portName", c.options.SerialPortName)

	// pending accumulates bytes read from the serial port. Frames are parsed and consumed from the
	// front of this buffer. Any incomplete frame data remains in pending for the next iteration.
	pending := []byte{}
	for {
		// Read up to 32 bytes at a time from the serial port.
		// The actual number of bytes returned depends on what the OS has buffered.
		working := make([]byte, 32)
		readBytes, err := port.Read(working)
		if err != nil {
			return err
		}
		// Append only the bytes actually read (not the full 32-byte buffer) to pending.
		pending = append(pending, working[0:readBytes]...)

		// Attempt to parse as many complete frames as possible from the accumulated buffer.
		// parseFrames will update the pending slice to remove consumed bytes.
		if err := c.parseFrames(&pending); err != nil {
			return err
		}
	}
}

// parseFrames processes the accumulated receive buffer, extracting and dispatching complete USB-CAN
// frames. It modifies the buffer in-place via the pointer, removing consumed bytes from the front.
//
// The parser handles three frame types:
//  1. Command frames: start with 0xAA 0x55, are always 20 bytes long, and are logged but not dispatched.
//     These are typically responses to settings frames or device status messages.
//  2. Data frames: start with 0xAA followed by an info byte with bits[7:6]=0b11. The info byte encodes
//     whether this is a standard/extended frame, remote/data frame, and the data length.
//  3. Error recovery: if the buffer doesn't start with 0xAA, the parser scans forward to find the next
//     0xAA byte and discards the garbage bytes before it.
//
// If a frame is incomplete (not enough bytes in the buffer yet), the function returns nil and leaves
// the partial data in the buffer for the next call.
//
// Parameters:
//   - bufAddr: pointer to the byte slice buffer, modified in-place to remove consumed bytes
func (c *USBCANChannel) parseFrames(bufAddr *[]byte) error {
	for {
		buf := *bufAddr

		// If the buffer is empty, there's nothing to parse.
		if len(buf) == 0 {
			return nil
		}

		// Every valid USB-CAN frame must start with 0xAA (start-of-frame marker).
		// If the first byte is not 0xAA, we have garbage/corruption in the stream.
		if buf[0] != 0xaa {
			// Scan forward to find the next 0xAA byte to resynchronize.
			idx := slices.Index(buf, 0xaa)
			if idx == -1 {
				// No 0xAA found anywhere in the buffer -- discard everything.
				c.log.Debug("Error frame", "data", fmt.Sprintf("%+v", buf))
				*bufAddr = []byte{}
				return nil
			} else {
				// Found 0xAA at position idx -- skip the garbage bytes before it.
				c.log.Debug("Error frame, skipping bytes", "skip", idx, "data", fmt.Sprintf("%+v", buf))
			}

			*bufAddr = buf[idx:]
			continue
		}

		// We need at least 2 bytes to determine the frame type (0xAA + type byte).
		if len(buf) < 2 {
			return nil
		}

		// ----- Command Frame -----
		// Format: 0xAA 0x55 <18 bytes of command/response data> (20 bytes total)
		// These are sent by the device in response to settings frames or as status updates.
		if buf[1] == 0x55 {
			// Command frames are always exactly 20 bytes long.
			// If we don't have all 20 bytes yet, wait for more data.
			if len(buf) < 20 {
				return nil
			}

			c.log.Debug("Command frame", "data", fmt.Sprintf("%+v", buf[0:20]))

			// Consume the 20-byte command frame and continue parsing any remaining data.
			*bufAddr = buf[20:]
			continue
		}

		// ----- Data Frame -----
		// Check if bits [7:6] of the info byte are 0b11 (value 3), indicating a data frame.
		// Info byte layout: [7:6]=frame_type, [5]=extended, [4]=remote, [3:0]=data_length
		if (buf[1] >> 6) == 3 {
			// Start with 2 bytes consumed so far (0xAA + info byte)
			frameLen := byte(2)

			// Bit 5 of the info byte: 1 = extended frame (29-bit CAN ID, 4 ID bytes),
			//                          0 = standard frame (11-bit CAN ID, 2 ID bytes)
			extendedFrame := (buf[1] & 0x20) > 0

			// Bit 4 of the info byte: 1 = remote frame (RTR), 0 = data frame.
			// Remote frames are logged but handled the same way as data frames here.
			remoteFrame := (buf[1] & 0x10) > 0

			// Add the ID field length: 4 bytes for extended, 2 bytes for standard.
			if extendedFrame {
				frameLen += 4
			} else {
				frameLen += 2
			}

			// Bits [3:0] of the info byte encode the number of data bytes (0-8).
			dataLen := buf[1] & 0xf

			// Total frame length = header (2) + ID bytes (2 or 4) + data bytes + end byte (0x55)
			frameLen += dataLen + 1
			if len(buf) < int(frameLen) {
				// Not enough bytes yet for the complete frame -- wait for more data.
				return nil
			}

			// Extract the CAN ID from the frame. The ID is stored in little-endian byte order.
			var frameID uint32
			if extendedFrame {
				// Extended frame: 4-byte CAN ID in little-endian (bytes 2-5)
				// This produces a 29-bit extended CAN ID used by protocols like NMEA 2000.
				frameID = (uint32(buf[2])) | (uint32(buf[3]) << 8) | (uint32(buf[4]) << 16) | (uint32(buf[5]) << 24)
			} else {
				// Standard frame: 2-byte CAN ID in little-endian (bytes 2-3)
				// This produces an 11-bit standard CAN ID.
				frameID = (uint32(buf[2])) | (uint32(buf[3]) << 8)
			}

			// The last byte of the frame must be 0x55 (end-of-frame marker).
			endByte := buf[frameLen-1]

			// Extract the data payload bytes (located between the ID and the end byte).
			dataBytes := buf[frameLen-1-dataLen : frameLen-1]
			if endByte != 0x55 {
				// If the end byte is wrong, the frame is corrupt. Log it and discard the entire
				// buffer since we can't reliably determine where the next valid frame starts.
				c.log.Debug("Data frame with bad end byte",
					"extended", extendedFrame, "remote", remoteFrame,
					"frameID", fmt.Sprintf("%X", frameID),
					"data", fmt.Sprintf("%+v", dataBytes),
					"endByte", fmt.Sprintf("%X", endByte))
				*bufAddr = []byte{}
				return nil
			}

			// Build a can.Frame struct with the parsed data. The can.Frame.Data field is a
			// fixed [8]byte array, so we copy only the actual data bytes into it.
			fData := [8]byte{}
			for i := 0; i < int(dataLen); i++ {
				fData[i] = dataBytes[i]
			}
			fd := can.Frame{
				ID:     frameID,
				Length: dataLen,
				Data:   fData,
			}

			// Dispatch the parsed frame to the user-provided handler callback.
			c.options.FrameHandler(fd)

			// Consume this frame from the buffer and continue parsing.
			*bufAddr = buf[frameLen:]
			continue
		}

		// ----- Unknown Frame Type -----
		// If we get here, the byte after 0xAA is neither 0x55 (command) nor has bits[7:6]=0b11 (data).
		// This is an unrecognized frame type -- log it and discard the buffer.
		c.log.Debug("Unknown frame", "data", fmt.Sprintf("%+v", buf))
		*bufAddr = []byte{}
		return nil
	}
}

// Close shuts down the USB-CAN channel by closing the underlying serial port.
// It is safe to call Close() even if the port was never opened (port is nil).
func (c *USBCANChannel) Close() error {
	if c.port != nil {
		if err := c.port.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WriteFrame sends a CAN frame out through the USB-CAN dongle by encoding it into the
// USB-CAN binary protocol format and writing it to the serial port.
//
// Outgoing data frame format:
//   - Byte 0: 0xAA (start-of-frame marker)
//   - Byte 1: 0xC0 | dataLen (info byte: bits[7:6]=0b11 for data frame, bits[3:0]=length)
//     If the CAN ID requires more than 16 bits, bit 5 is set for extended frame.
//   - Bytes 2-3 (standard) or 2-5 (extended): CAN ID in little-endian byte order
//   - Next N bytes: data payload (N = frame.Length)
//   - Final byte: 0x55 (end-of-frame marker)
//
// Note: This currently defaults to standard frames and switches to extended only if the
// frame ID exceeds 0xFFFF. For NMEA 2000, which always uses 29-bit extended IDs, the
// frame ID will always exceed 0xFFFF, so extended mode is used automatically.
func (c *USBCANChannel) WriteFrame(frame can.Frame) error {
	// Build the frame header: start byte + info byte + 2-byte little-endian ID (standard).
	buf := []byte{
		0xaa,
		0xC0 | frame.Length, // 0xC0 = bits[7:6]=0b11 (data frame), OR'd with data length in bits[3:0]
		byte(frame.ID),      // CAN ID low byte
		byte(frame.ID >> 8), // CAN ID high byte (for standard frames)
	}
	// Not sure if this is the right way to do it, but for now give it a shot
	// (we're only sending standard frames over calex, so need to test this on n2k someday if we care...)
	if frame.ID > 0xffff {
		// The CAN ID needs more than 16 bits, so switch to extended frame format.
		// Set bit 5 of the info byte to indicate extended frame, and append the upper 2 ID bytes.
		buf[1] |= 0x20
		buf = append(buf, byte(frame.ID>>16), byte(frame.ID>>24))
	}
	// Append the data payload bytes and the end-of-frame marker.
	buf = append(buf, frame.Data[0:frame.Length]...)
	buf = append(buf, 0x55)

	o, err := c.port.Write(buf)
	if o != len(buf) {
		return fmt.Errorf("WriteFrame sent %d of %d bytes", o, len(buf))
	}
	if err != nil {
		return err
	}

	return nil
}

// sendSettingsFrame sends the initial 20-byte configuration frame to the USB-CAN device.
// This frame tells the device what CAN bitrate to use, what frame type to expect,
// and what operating mode to run in.
//
// Settings frame format (20 bytes):
//   - Byte 0:    0xAA (start-of-frame marker)
//   - Byte 1:    0x55 (identifies this as a command/settings frame)
//   - Byte 2:    0x12 (settings command identifier)
//   - Byte 3:    bitrate code (mapped from numeric bitrate via mapBitRate)
//   - Byte 4:    frame type (FrameStandard=1 or FrameExtended=2)
//   - Bytes 5-12: reserved/unused (all zeros)
//   - Byte 13:   operating mode (ModeNormal=0, ModeLoopback=1, etc.)
//   - Byte 14:   0x01 (unknown purpose, possibly an enable flag)
//   - Bytes 15-18: reserved/unused (all zeros)
//   - Byte 19:   checksum (sum of bytes 2 through 18, truncated to a single byte)
func (c *USBCANChannel) sendSettingsFrame() error {
	// Map the human-readable bitrate (e.g. 250000) to the protocol's byte code (e.g. 0x05).
	br, err := mapBitRate(c.options.BitRate)
	if err != nil {
		return err
	}

	buf := []byte{
		0xaa,                    // Byte 0: start-of-frame
		0x55,                    // Byte 1: command frame indicator
		0x12,                    // Byte 2: settings command ID
		br,                      // Byte 3: CAN bitrate code
		byte(FrameStandard),     // Byte 4: frame type (standard 11-bit IDs)
		0,                       // Byte 5: reserved
		0,                       // Byte 6: reserved
		0,                       // Byte 7: reserved
		0,                       // Byte 8: reserved
		0,                       // Byte 9: reserved
		0,                       // Byte 10: reserved
		0,                       // Byte 11: reserved
		0,                       // Byte 12: reserved
		byte(ModeNormal),        // Byte 13: operating mode (normal)
		0x01,                    // Byte 14: possibly an enable flag
		0,                       // Byte 15: reserved
		0,                       // Byte 16: reserved
		0,                       // Byte 17: reserved
		0,                       // Byte 18: reserved
		0,                       // Byte 19: placeholder for checksum
	}
	// Calculate the checksum over bytes 2-18 (17 bytes starting at index 2).
	// The checksum is a simple 8-bit sum (overflow wraps around naturally due to byte arithmetic).
	buf[19] = calcChecksum(buf, 2, 17)

	o, err := c.port.Write(buf)
	if o != len(buf) {
		return fmt.Errorf("sendSettingsFrame sent %d of %d bytes", o, len(buf))
	}
	if err != nil {
		return err
	}

	return nil
}

// mapBitRate converts a numeric CAN bus bitrate (in bits per second) to the corresponding
// protocol byte code expected by the USB-CAN Analyzer device.
//
// The USB-CAN protocol does not accept raw bitrate values -- instead, each supported bitrate
// is assigned a specific byte code that is sent in the settings frame.
//
// Parameters:
//   - bitRate: the desired CAN bus bitrate in bits per second (e.g. 250000 for NMEA 2000)
//
// Returns:
//   - The corresponding byte code, or an error if the bitrate is not supported by the device.
func mapBitRate(bitRate int) (byte, error) {
	switch bitRate {
	case 1000000:
		return 0x01, nil
	case 800000:
		return 0x02, nil
	case 500000:
		return 0x03, nil
	case 400000:
		return 0x04, nil
	case 250000:
		return 0x05, nil // NMEA 2000 standard bitrate
	case 200000:
		return 0x06, nil
	case 125000:
		return 0x07, nil
	case 100000:
		return 0x08, nil
	case 50000:
		return 0x09, nil
	case 20000:
		return 0x0a, nil
	case 10000:
		return 0x0b, nil
	case 5000:
		return 0x0c, nil
	default:
		return 0, fmt.Errorf("no matching bitrate setting for %d", bitRate)
	}
}

// calcChecksum computes a simple 8-bit additive checksum over a range of bytes in a buffer.
// This is the checksum algorithm used by the USB-CAN Analyzer protocol for settings frames.
//
// The checksum is calculated by summing all bytes in the range [start, start+len) and
// letting the result overflow naturally (since the return type is a single byte, the sum
// wraps around modulo 256).
//
// Parameters:
//   - buf: the byte buffer to checksum
//   - start: the starting index (inclusive) within buf
//   - len: the number of bytes to include in the checksum
//
// Returns the 8-bit checksum value.
func calcChecksum(buf []byte, start, len int) byte {
	cs := byte(0)
	for i := start; i < start+len; i++ {
		cs += buf[i]
	}
	return cs
}
