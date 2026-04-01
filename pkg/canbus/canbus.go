// Package canbus is built around the Channel structure, which represents a single canbus channel for sending/receiving CAN frames.
//
// CAN bus (Controller Area Network) is a communication protocol widely used in marine NMEA 2000 networks
// and automotive systems. This package provides two implementations:
//   - USBCANChannel: for USB-to-CAN adapter dongles that communicate via serial port
//   - SocketCANChannel: for Linux SocketCAN interfaces (e.g., MCP2515 SPI-based CAN controllers)
//
// Both implementations satisfy the Interface type and handle the low-level framing,
// bitrate configuration, and frame parsing required to send and receive raw CAN frames.
package canbus
