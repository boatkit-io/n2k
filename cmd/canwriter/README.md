# CAN Writer Tool

A command-line tool for writing streams of CAN frames to a CAN bus interface using the `github.com/brutella/can` library.

## Usage

```bash
./canwriter [options]
```

## Options

- `-iface string`: CAN interface name (default "can0")
- `-count int`: Number of frames to send (default 10)
- `-interval duration`: Interval between frames (default 100ms)
- `-verbose`: Enable verbose logging
- `-baseid uint`: Base CAN ID in decimal or hex format (default 0x18F00400)
- `-pattern string`: Data pattern - increment, random, fixed, nmea (default "increment")
- `-source uint`: NMEA 2000 source address 0-255 (default 100)
- `-pgn uint`: NMEA 2000 PGN for nmea pattern (default 61184)

## Examples

### Basic Usage - Send 10 frames with incrementing data
```bash
./bin/canwriter
```

### Send to specific interface with custom timing
```bash
./bin/canwriter -iface can1 -count 20 -interval 50ms
```

### Send NMEA 2000 compatible frames
```bash
./bin/canwriter -pattern nmea -pgn 127488 -source 110 -count 5
```

### Send with custom base ID (hex format)
```bash
./bin/canwriter -baseid 0x1CFFFFFF -pattern fixed -count 3
```

### Random data pattern with verbose logging
```bash
./bin/canwriter -pattern random -verbose -interval 200ms
```

### High-frequency burst test
```bash
./bin/canwriter -count 100 -interval 10ms -pattern increment
```

## Data Patterns

### increment (default)
- Frame counter, test patterns, timestamp, and calculated values
- Useful for testing frame ordering and timing

### random
- Pseudo-random data based on time and index
- Good for stress testing and bandwidth verification

### fixed
- Fixed test pattern (0xDEADBEEF, 0xCAFEBABE)
- Useful for protocol testing and debugging

### nmea
- NMEA 2000 compatible frame format
- Proper PGN-based frame IDs with realistic marine data
- Simulates engine RPM, temperature, and other sensor data

## API Implementation

The tool uses the correct `github.com/brutella/can` API pattern for sender applications:

1. Create bus: `can.NewBusForInterfaceWithName("can0")`
2. Send frames directly: `bus.Publish(frame)`
3. Cleanup: `bus.Disconnect()`

**Note**: `ConnectAndPublish()` is designed for applications that need to continuously read from the CAN bus (listeners/servers) and runs an infinite loop. For sender applications like canwriter, we only need to create the bus and use `Publish()` directly.

## Prerequisites

- A CAN interface must be available (e.g., can0, vcan0)
- Appropriate permissions to access the CAN interface
- The interface must be brought up with proper bitrate:

```bash
# For physical CAN interface
sudo ip link set can0 type can bitrate 250000
sudo ip link set can0 up

# For virtual CAN interface (testing)
sudo modprobe vcan
sudo ip link add dev vcan0 type vcan
sudo ip link set vcan0 up
```

## Building

```bash
mise run build
```

The binary will be created as `bin/canwriter`.

## Example Output

```
$ ./bin/canwriter -count 3 -pattern nmea -verbose
INFO[0000] Opened CAN interface can0
INFO[0000] Sending 3 frames with 100ms interval
INFO[0000] Base ID: 0x18F00400, Pattern: nmea
INFO[0000] NMEA 2000 mode: PGN=61184, Source=100
Sent frame 1: ID=0x18EF0064, Data=[00006400FF00FFFF]
Sent frame 2: ID=0x18EF0064, Data=[0164000001FF0002FFFF]
Sent frame 3: ID=0x18EF0064, Data=[025802FF04FFFFFF]
INFO[0000] Successfully sent 3/3 frames
``` 