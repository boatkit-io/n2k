# NMEA 2000 Node Design Document

## Overview

The `Node` package provides a generic NMEA 2000 device implementation that handles the standard behaviors required for any N2K device on the network. It sits above the existing pipeline architecture and provides a high-level abstraction for implementing NMEA 2000 devices.

## Architecture Integration

```
Application Layer (Device Implementation)
    ↓
Node (Generic N2K Device)
    ↓ (reads via)          ↓ (writes via)
Subscribe Manager ←--→ Publisher
    ↓                      ↓
Converter ←--→ Converter
    ↓                      ↓
Adapter ←--→ Adapter
    ↓                      ↓
Endpoint
```

The Node integrates with the existing pipeline by:
- Using `subscribe.Manager` to receive PGNs from the network
- Using `pgn.Publisher` to send PGNs to the network
- Working with PGN structs (not raw bytes) for type safety

## Core Interface

```go
type Node interface {
    // Lifecycle - explicit control for dynamic reconfiguration
    Start() error
    Stop() error
    
    // Address management
    ClaimAddress(preferredAddress uint8) error
    GetNetworkAddress() uint8
    IsAddressClaimed() bool
    
    // Writing PGNs (idiomatic Go naming)
    Write(pgnStruct any) error
    WriteTo(pgnStruct any, destination uint8) error
    
    // Configuration
    SetDeviceInfo(info DeviceInfo) error  // Returns error for invalid NAME
    SetProductInfo(info ProductInfo)
    SetSupportedPGNs(transmit, receive []uint32)
    
    // Customizable handlers
    RegisterPGNHandler(pgn uint32, handler PGNHandler)
    
    // Heartbeat control
    SetHeartbeatInterval(interval time.Duration)
    EnableHeartbeat(enable bool)
}

type PGNHandler interface {
    HandlePGN(pgnStruct any, sourceAddress uint8) error
}
```

## Data Structures

### Node Structure
```go
type Node struct {
    // Dependencies
    subscriber *subscribe.Manager
    publisher  *pgn.Publisher  // Use existing Publisher
    
    // Device identity
    deviceInfo  DeviceInfo
    productInfo ProductInfo
    name        uint64  // Computed NAME field (ISO 11783-5)
    
    // Network state
    networkAddress   uint8
    preferredAddress uint8
    addressClaimed   bool
    
    // Standard PGN support
    transmitPGNs []uint32
    receivePGNs  []uint32
    
    // Custom handlers
    pgnHandlers map[uint32]PGNHandler
    
    // Heartbeat management
    heartbeatEnabled  bool
    heartbeatInterval time.Duration
    heartbeatSeq      uint8
    heartbeatTicker   *time.Ticker
    
    // Lifecycle
    started bool
    ctx     context.Context
    cancel  context.CancelFunc
    wg      sync.WaitGroup
    
    // Subscriptions (for cleanup)
    subscriptions []subscribe.Subscription
}
```

### Device Information
```go
type DeviceInfo struct {
    UniqueNumber         uint32                     // 21 bits max
    ManufacturerCode     pgn.ManufacturerCodeConst  // 11 bits max
    DeviceFunction       pgn.DeviceFunctionConst    // 8 bits
    DeviceClass          pgn.DeviceClassConst       // 7 bits
    DeviceInstanceLower  uint8                      // 3 bits
    DeviceInstanceUpper  uint8                      // 5 bits
    SystemInstance       uint8                      // 4 bits
    IndustryGroup        pgn.IndustryCodeConst      // 3 bits
    ArbitraryAddressCapable bool                    // 1 bit
}

type ProductInfo struct {
    NMEA2000Version      float32
    ProductCode          uint16
    ModelID              string
    SoftwareVersionCode  string
    ModelVersion         string
    ModelSerialCode      string
    CertificationLevel   uint8
    LoadEquivalency      uint8
}
```

## Key Behaviors

### 1. Address Claiming Process
- Follows ISO 11783-5 address claiming procedure
- Lower address numbers have higher priority
- Monitors for address conflicts and responds appropriately
- Gracefully handles write-only or replay endpoints

**Algorithm:**
1. Send `IsoAddressClaim` PGN with device NAME
2. Monitor network for conflicts (same address)
3. Compare NAMEs if conflict detected (lower NAME wins)
4. If we lose, find new address and re-claim
5. Periodically re-send claim to maintain address

### 2. Standard PGN Handling

The Node automatically subscribes to and handles these standard PGNs:

- **ISO Address Claim (60928)**: Monitor conflicts, respond to challenges
- **ISO Commanded Address (65240)**: Handle address commands from network manager
- **ISO Request (59904)**: Route to registered handlers or return appropriate response
- **Heartbeat (126993)**: Optional periodic transmission

### 3. Built-in Responses

**Product Information (126996):**
- Responds with configured product details
- Automatically triggered by ISO Request

**PGN List (126464):**
- Responds with supported transmit/receive PGN lists
- Automatically triggered by ISO Request

**Default ISO Request Response:**
- For unhandled PGNs, sends `IsoAcknowledgement` with "Not Available" control byte

### 4. NAME Field Validation

The `SetDeviceInfo()` method validates that all fields fit within their bit limits per ISO 11783-5:
```go
func (d DeviceInfo) ComputeName() (uint64, error) {
    // Validate field ranges
    if d.UniqueNumber > 0x1FFFFF {
        return 0, fmt.Errorf("unique number exceeds 21-bit limit")
    }
    // ... other validations
    
    // Pack into 64-bit NAME field per ISO 11783-5
    name := uint64(d.UniqueNumber) |
           (uint64(d.ManufacturerCode) << 21) |
           (uint64(d.DeviceInstanceLower) << 32) |
           // ... continue bit packing per spec
    
    return name, nil
}
```

## Usage Examples

### Basic Device Setup
```go
// Create node with existing pipeline components
node := NewNode(subscriberManager, publisher)

// Configure device identity
deviceInfo := DeviceInfo{
    UniqueNumber:     12345,
    ManufacturerCode: pgn.YourManufacturerCode,
    DeviceFunction:   pgn.EngineFunction,
    DeviceClass:      pgn.PropulsionClass,
    DeviceInstanceLower: 0,
    DeviceInstanceUpper: 0,
    SystemInstance:   0,
    IndustryGroup:    pgn.Marine,
    ArbitraryAddressCapable: true,
}
err := node.SetDeviceInfo(deviceInfo)  // Validates NAME
if err != nil {
    log.Fatal("Invalid device info:", err)
}

// Set product information
node.SetProductInfo(ProductInfo{
    NMEA2000Version: 2.300,
    ProductCode:     1001,
    ModelID:         "MyEngine v1.0",
    SoftwareVersionCode: "1.0.0",
    ModelVersion:    "Rev A",
    ModelSerialCode: "SN123456",
    CertificationLevel: 1,
    LoadEquivalency: 1,
})

// Configure supported PGNs
node.SetSupportedPGNs(
    []uint32{127488, 127489}, // Transmit: Engine parameters
    []uint32{126208},         // Receive: NMEA Request Group Function
)

// Start and claim address
err = node.ClaimAddress(50)  // Preferred address
if err != nil {
    log.Fatal("Address claim failed:", err)
}
err = node.Start()
if err != nil {
    log.Fatal("Node start failed:", err)
}

// Enable heartbeat
node.SetHeartbeatInterval(60 * time.Second)
node.EnableHeartbeat(true)
```

### Custom PGN Handler
```go
// Register custom handler for engine data requests
node.RegisterPGNHandler(127488, &MyEngineHandler{})

type MyEngineHandler struct {
    engine *Engine
}

func (h *MyEngineHandler) HandlePGN(pgnStruct any, sourceAddress uint8) error {
    // Handle ISO Request for engine data
    request := pgnStruct.(*pgn.IsoRequest)
    
    // Create engine data response
    engineData := &pgn.EngineParametersRapid{
        Info: pgn.MessageInfo{
            PGN:      127488,
            SourceId: node.GetNetworkAddress(),
            TargetId: sourceAddress,
        },
        EngineInstance: &[]uint8{0}[0],
        EngineSpeed:    &h.engine.RPM,
        // ... other fields
    }
    
    return node.WriteTo(engineData, sourceAddress)
}
```

### Multiple Device Functions
```go
// Different device functions in same application
engineNode := NewNode(subscriber, publisher)
engineNode.SetDeviceInfo(DeviceInfo{
    DeviceFunction: pgn.EngineFunction,
    // ... other fields
})

navigationNode := NewNode(subscriber, publisher)  
navigationNode.SetDeviceInfo(DeviceInfo{
    DeviceFunction: pgn.NavigationFunction,
    // ... other fields
})

// Each gets its own address
engineNode.ClaimAddress(50)
navigationNode.ClaimAddress(51)

engineNode.Start()
navigationNode.Start()
```

### Sending Data
```go
// Send engine data periodically
ticker := time.NewTicker(1 * time.Second)
go func() {
    for range ticker.C {
        engineData := &pgn.EngineParametersRapid{
            Info: pgn.MessageInfo{
                PGN:      127488,
                SourceId: node.GetNetworkAddress(),
            },
            EngineInstance: &[]uint8{0}[0],
            EngineSpeed:    &engine.GetRPM(),
            // ... other fields
        }
        
        err := node.Write(engineData)  // Broadcast
        if err != nil {
            log.Printf("Failed to send engine data: %v", err)
        }
    }
}()
```

## Implementation Considerations

### 1. Lifecycle Management
- **Explicit Start/Stop**: Supports dynamic reconfiguration by allowing restart
- **Address Revalidation**: On restart, address claiming process runs again
- **Graceful Shutdown**: Stop() cancels goroutines and cleans up subscriptions

### 2. Error Handling
- **Silent Write Failures**: If endpoint doesn't support writing, operations succeed but nothing happens
- **Address Conflicts**: Automatically handled per ISO 11783-5
- **Invalid Configurations**: `SetDeviceInfo()` validates and returns errors

### 3. Thread Safety
- All public methods are thread-safe
- Internal state protected by mutexes where needed
- Goroutines properly managed with context cancellation

### 4. Memory Management
- Subscriptions tracked and cleaned up on Stop()
- Timers properly stopped to prevent leaks
- No persistent connections or resources

### 5. Testing Considerations
- Interface-based design supports easy mocking
- Dependencies injected for testability
- State can be inspected for validation

## Future Enhancements

### Deferred Features
- **Address Contention**: `ForceAddress()` method to actively contend for higher priority addresses
- **ISO Transport Protocol**: Multi-frame message support for large PGNs
- **Network Device List**: Track other devices on the network
- **Configuration Persistence**: Save/load device configuration

### Potential Optimizations
- **PGN Filtering**: Only subscribe to PGNs we actually handle
- **Batched Writes**: Combine multiple PGNs into single transmission
- **Priority-based Transmission**: Queue management for different PGN priorities

## Dependencies

### Required Packages
- `github.com/boatkit-io/n2k/pkg/subscribe` - For receiving PGNs
- `github.com/boatkit-io/n2k/pkg/pgn` - For PGN encoding/decoding and Publisher
- `context` - For lifecycle management
- `sync` - For thread safety
- `time` - For heartbeat and timers

### Integration Points
- Must work with existing `subscribe.Manager`
- Must use existing `pgn.Publisher`
- Must work with generated PGN structs from `pgninfo_generated.go`
- Should integrate with existing endpoint abstraction 