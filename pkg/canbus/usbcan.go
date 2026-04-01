package canbus

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/brutella/can"
	"go.bug.st/serial"
)

// Docs for this whole crappy thing:
// * https://github.com/SeeedDocument/USB-CAN-Analyzer/blob/master/res/Document/USB%20(Serial%20port)%20to%20CAN%20protocol%20defines.pdf
// * https://github.com/SeeedDocument/USB-CAN-Analyzer?tab=readme-ov-file
// * https://github.com/kobolt/usb-can/blob/master/canusb.c

// CANUSBMode is an enum for the CANUSB device's modes, which we don't really understand
type CANUSBMode byte

const (
	ModeNormal         CANUSBMode = 0
	ModeLoopback       CANUSBMode = 1
	ModeSilent         CANUSBMode = 2
	ModeLoopbackSilent CANUSBMode = 3
)

// CANUSBFrame is an enum for the frame type
type CANUSBFrame byte

const (
	FrameStandard CANUSBFrame = 1
	FrameExtended CANUSBFrame = 2
)

// USBCANChannelOptions is a type that contains required options on a SocketCANChannel.
type USBCANChannelOptions struct {
	SerialPortName string
	SerialBaudRate int
	BitRate        int
	FrameHandler   can.HandlerFunc
}

// USBCANChannel represents a single USB-CAN-based canbus channel for sending/receiving CAN frames
type USBCANChannel struct {
	options USBCANChannelOptions

	port serial.Port

	log *slog.Logger
}

// NewUSBCANChannel returns a Channel object based on USBCAN and the given options.  ChannelOptions are required settings.
func NewUSBCANChannel(log *slog.Logger, options USBCANChannelOptions) Interface {
	c := USBCANChannel{
		options: options,
		log:     log,
	}

	return &c
}

// Run opens the serial interface and starts listening.
func (c *USBCANChannel) Run(ctx context.Context) error {
	mode := &serial.Mode{
		BaudRate: c.options.SerialBaudRate,
	}
	port, err := serial.Open(c.options.SerialPortName, mode)
	if err != nil {
		return err
	}

	c.port = port

	if err := c.sendSettingsFrame(); err != nil {
		return err
	}

	c.log.Info("Opened USBCAN and listening", "portName", c.options.SerialPortName)

	pending := []byte{}
	for {
		working := make([]byte, 32)
		readBytes, err := port.Read(working)
		if err != nil {
			return err
		}
		pending = append(pending, working[0:readBytes]...)

		if err := c.parseFrames(&pending); err != nil {
			return err
		}
	}
}

// parseFrames is a helper to parse any waiting frames from the recv buffer, and update the recv buffer to keep going
func (c *USBCANChannel) parseFrames(bufAddr *[]byte) error {
	for {
		buf := *bufAddr

		if len(buf) == 0 {
			return nil
		}

		if buf[0] != 0xaa {
			idx := slices.Index(buf, 0xaa)
			if idx == -1 {
				c.log.Debug("Error frame", "data", fmt.Sprintf("%+v", buf))
				*bufAddr = []byte{}
				return nil
			} else {
				c.log.Debug("Error frame, skipping bytes", "skip", idx, "data", fmt.Sprintf("%+v", buf))
			}

			*bufAddr = buf[idx:]
			continue
		}

		if len(buf) < 2 {
			return nil
		}

		if buf[1] == 0x55 {
			// command frame
			if len(buf) < 20 {
				return nil
			}

			c.log.Debug("Command frame", "data", fmt.Sprintf("%+v", buf[0:20]))

			*bufAddr = buf[20:]
			continue
		}

		if (buf[1] >> 6) == 3 {
			// data frame
			frameLen := byte(2)
			extendedFrame := (buf[1] & 0x20) > 0
			remoteFrame := (buf[1] & 0x10) > 0
			if extendedFrame {
				frameLen += 4
			} else {
				frameLen += 2
			}

			dataLen := buf[1] & 0xf
			frameLen += dataLen + 1
			if len(buf) < int(frameLen) {
				return nil
			}

			var frameID uint32
			if extendedFrame {
				frameID = (uint32(buf[2])) | (uint32(buf[3]) << 8) | (uint32(buf[4]) << 16) | (uint32(buf[5]) << 24)
			} else {
				frameID = (uint32(buf[2])) | (uint32(buf[3]) << 8)
			}

			endByte := buf[frameLen-1]

			dataBytes := buf[frameLen-1-dataLen : frameLen-1]
			if endByte != 0x55 {
				c.log.Debug("Data frame with bad end byte",
					"extended", extendedFrame, "remote", remoteFrame,
					"frameID", fmt.Sprintf("%X", frameID),
					"data", fmt.Sprintf("%+v", dataBytes),
					"endByte", fmt.Sprintf("%X", endByte))
				*bufAddr = []byte{}
				return nil
			}

			fData := [8]byte{}
			for i := 0; i < int(dataLen); i++ {
				fData[i] = dataBytes[i]
			}
			fd := can.Frame{
				ID:     frameID,
				Length: dataLen,
				Data:   fData,
			}
			c.options.FrameHandler(fd)

			*bufAddr = buf[frameLen:]
			continue
		}

		c.log.Debug("Unknown frame", "data", fmt.Sprintf("%+v", buf))
		*bufAddr = []byte{}
		return nil
	}
}

// Close shuts down the channel
func (c *USBCANChannel) Close() error {
	if c.port != nil {
		if err := c.port.Close(); err != nil {
			return err
		}
	}
	return nil
}

// WriteFrame will send a CAN frame to the channel
func (c *USBCANChannel) WriteFrame(frame can.Frame) error {
	buf := []byte{
		0xaa,
		0xC0 | frame.Length,
		byte(frame.ID),
		byte(frame.ID >> 8),
	}
	// Not sure if this is the right way to do it, but for now give it a shot
	// (we're only sending standard frames over calex, so need to test this on n2k someday if we care...)
	if frame.ID > 0xffff {
		// switch to extended frame
		buf[1] |= 0x20
		buf = append(buf, byte(frame.ID>>16), byte(frame.ID>>24))
	}
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

// sendSettingsFrame is a helper to send the startup settings frame to set the bitrate appropriately
func (c *USBCANChannel) sendSettingsFrame() error {
	br, err := mapBitRate(c.options.BitRate)
	if err != nil {
		return err
	}

	buf := []byte{
		0xaa,
		0x55,
		0x12,
		br,
		byte(FrameStandard),
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		0,
		byte(ModeNormal),
		0x01,
		0,
		0,
		0,
		0,
		0,
	}
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

// mapBitRate is a helper to map numeric bitrates to their byte values
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
		return 0x05, nil
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

// calcChecksum is a crappy function that calcs checksum the way they do
func calcChecksum(buf []byte, start, len int) byte {
	cs := byte(0)
	for i := start; i < start+len; i++ {
		cs += buf[i]
	}
	return cs
}
