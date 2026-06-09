# Debugging Guide for canwriter

This guide explains how to debug the `canwriter` tool using VS Code debugging configurations and tasks.

## Prerequisites

1. **VS Code with Go extension** installed
2. **Virtual CAN utilities** (for testing without real hardware):
   ```bash
   sudo apt-get install can-utils
   ```
3. **Delve debugger** (usually installed with Go extension):
   ```bash
   go install github.com/go-delve/delve/cmd/dlv@latest
   ```

## Debug Configurations

The project includes several pre-configured debug configurations in `.vscode/launch.json`:

### 1. Debug canwriter (basic)

- **Purpose**: Basic debugging with default increment pattern
- **Interface**: `vcan0` (virtual CAN)
- **Args**: `-iface=vcan0 -count=5 -interval=200ms -verbose`
- **Use case**: General debugging and testing basic functionality

### 2. Debug canwriter (NMEA mode)

- **Purpose**: Debug NMEA 2000 compatible frame generation
- **Interface**: `vcan0` (virtual CAN)
- **Args**: `-iface=vcan0 -pattern=nmea -pgn=127488 -source=110 -count=3 -interval=500ms -verbose`
- **Use case**: Testing NMEA 2000 frame ID generation and data patterns

### 3. Debug canwriter (random pattern)

- **Purpose**: Debug random data generation
- **Interface**: `vcan0` (virtual CAN)
- **Args**: `-iface=vcan0 -pattern=random -count=10 -interval=100ms -baseid=0x1CFFFFFF -verbose`
- **Use case**: Testing random data generation and custom base IDs

### 4. Debug canwriter (fixed pattern)

- **Purpose**: Debug fixed test patterns
- **Interface**: `vcan0` (virtual CAN)
- **Args**: `-iface=vcan0 -pattern=fixed -count=5 -interval=1s -verbose`
- **Use case**: Testing deterministic patterns for protocol debugging

### 5. Debug canwriter (real CAN interface)

- **Purpose**: Debug with real CAN hardware
- **Interface**: `can0` (real CAN interface)
- **Args**: `-iface=can0 -pattern=nmea -pgn=126993 -source=100 -count=3 -interval=1s -verbose`
- **Use case**: Testing with actual CAN hardware

## VS Code Tasks

Several tasks are available in `.vscode/tasks.json` to help with testing and debugging:

### Setup Tasks

#### Setup vcan0 interface

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Setup vcan0 interface"
- **Purpose**: Creates and brings up virtual CAN interface for testing
- **Requires**: `sudo` privileges

#### Remove vcan0 interface

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Remove vcan0 interface"
- **Purpose**: Removes the virtual CAN interface when done testing

### Monitoring Tasks

#### Monitor vcan0 traffic

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Monitor vcan0 traffic"
- **Purpose**: Opens `candump` to monitor CAN traffic in real-time
- **Output**: Shows all CAN frames being sent/received on vcan0

#### Check CAN interfaces

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Check CAN interfaces"
- **Purpose**: Lists all available CAN interfaces on the system

### Build and Test Tasks

#### Build canwriter

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Build canwriter"
- **Purpose**: Builds the canwriter binary using `mise run build`

#### Test canwriter with vcan0

- **Command**: `Ctrl+Shift+P` → "Tasks: Run Task" → "Test canwriter with vcan0"
- **Purpose**: Runs canwriter with test configuration
- **Dependencies**: Automatically builds canwriter first

## Debugging Workflow

### 1. Setup Virtual CAN Interface

```bash
# Method 1: Use VS Code task
Ctrl+Shift+P → "Tasks: Run Task" → "Setup vcan0 interface"

# Method 2: Manual command
sudo modprobe vcan
sudo ip link add dev vcan0 type vcan
sudo ip link set vcan0 up
```

### 2. Start Traffic Monitoring (Optional)

```bash
# Method 1: Use VS Code task
Ctrl+Shift+P → "Tasks: Run Task" → "Monitor vcan0 traffic"

# Method 2: Manual command
candump vcan0
```

### 3. Start Debugging

1. Open VS Code
2. Go to **Run and Debug** panel (`Ctrl+Shift+D`)
3. Select desired debug configuration from dropdown
4. Set breakpoints in the code as needed
5. Press **F5** or click **Start Debugging**

### 4. Debug Key Areas

#### Frame Generation

Set breakpoints in:

- `generateFrame()` - Main frame generation dispatcher
- `generateNMEAFrame()` - NMEA 2000 frame creation
- `generateIncrementFrame()` - Increment pattern
- `generateRandomFrame()` - Random pattern
- `generateFixedFrame()` - Fixed pattern

#### CAN Bus Communication

Set breakpoints in:

- `can.NewBusForInterfaceWithName()` - Bus creation
- `bus.Publish(frame)` - Frame sending
- Error handling around CAN operations

**Note**: `ConnectAndPublish()` is not used in canwriter as it's designed for listeners that need to continuously read from the CAN bus and runs an infinite loop.

#### Command Line Processing

Set breakpoints in:

- `flag.Parse()` - Argument parsing
- Parameter validation logic
- Configuration logging

## Common Debugging Scenarios

### 1. Frame ID Issues

- **Debug config**: "Debug canwriter (NMEA mode)"
- **Focus**: `generateNMEAFrame()` function
- **Check**: Priority, PGN, and Source ID encoding

### 2. Timing Issues

- **Debug config**: "Debug canwriter (basic)"
- **Focus**: Main loop with `time.Sleep(*interval)`
- **Check**: Interval calculation and frame sending timing

### 3. Data Pattern Problems

- **Debug config**: "Debug canwriter (random pattern)" or "Debug canwriter (fixed pattern)"
- **Focus**: Specific pattern generation functions
- **Check**: Data array filling and pattern logic

### 4. CAN Interface Problems

- **Debug config**: "Debug canwriter (real CAN interface)"
- **Focus**: `can.NewBusForInterfaceWithName()` and `bus.Publish()`
- **Check**: Interface availability and permissions

## Troubleshooting

### Virtual CAN Interface Issues

```bash
# Check if vcan module is loaded
lsmod | grep vcan

# Check interface status
ip link show type can

# Check interface traffic
candump vcan0
```

### Permission Issues

```bash
# Add user to appropriate groups (may require logout/login)
sudo usermod -a -G dialout $USER

# Or run with sudo for testing
sudo ./bin/canwriter -iface=can0 -count=3 -verbose
```

### Debugging Not Working

1. Ensure Go extension is installed and updated
2. Check that `dlv` debugger is in PATH
3. Verify workspace folder is correctly opened
4. Check that `mise run build` completes successfully

## Tips

1. **Use verbose logging**: Always include `-verbose` flag during debugging
2. **Start with virtual CAN**: Test with `vcan0` before using real hardware
3. **Monitor traffic**: Use `candump` to verify frames are being sent
4. **Small counts**: Use small `-count` values during debugging to avoid long waits
5. **Longer intervals**: Use longer `-interval` values to step through code more easily
