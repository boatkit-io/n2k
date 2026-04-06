# Virtual CAN (vcan) Interface Support

## Status

vcan interfaces are now supported with the modified tugboat package. The `dumpcan` command and other SocketCAN endpoints work correctly with vcan interfaces.

## Usage

```bash
# Dump CAN messages from vcan0
bin/dumpcan -iface vcan0

# Generate test PGN data on vcan0
bin/spewpgns -iface vcan0
```

## Implementation

The tugboat package has been modified to handle vcan interfaces properly by:

- Accepting "vcan" as a valid link type
- Handling vcan interfaces as GenericLink instead of Can links
- Skipping bitrate configuration for vcan interfaces (not applicable)
- Using the brutella/can library for actual CAN communication

## Solutions

### 1. Use Real CAN Hardware

The most reliable solution is to use actual CAN hardware:

- USB-CAN adapters
- PCIe CAN cards
- Embedded CAN controllers

### 2. Use Alternative Virtual CAN Implementations

Instead of vcan, consider:

- **slcan**: Serial line CAN interface
- **can-utils**: Provides various CAN tools
- **candump/cansend**: For testing and development

### 3. Use Replay Functionality

For testing and development, use the replay functionality with recorded CAN data:

```bash
# Replay from a raw log file
bin/replay -rawReplayFile /path/to/can_data.log

# Replay from an n2k log file  
bin/replay -replayFile /path/to/n2k_data.log
```

### 4. Use spewpgns for Test Data Generation

The `spewpgns` command can generate test PGN data for development:

```bash
# Generate test data on a real CAN interface
bin/spewpgns -iface can0
```

### 5. Create Test Data Files

Create test data files and use the replay functionality:

```bash
# Create a simple test file
echo "12345678 1 0 0 0 0 0 0 0" > test_data.raw

# Replay the test data
bin/replay -rawReplayFile test_data.raw
```

## Testing CAN Interface Support

Use the test program to check if your CAN interface is supported:

```bash
go run cmd/testvcan/main.go -iface <interface_name>
```

## Workarounds for Development

1. **Use Real Hardware**: Connect actual CAN hardware for full testing
2. **Record and Replay**: Record data from real hardware, then replay for testing
3. **Mock Data**: Create test data files with known PGN structures
4. **Alternative Libraries**: Consider using different CAN libraries that support vcan

## Future Improvements

- Consider updating the `tugboat` package when vcan support is added
- Implement a custom CAN interface wrapper that supports vcan
- Add better error messages and fallback options in the codebase
