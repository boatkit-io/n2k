# Node Testing Framework Design Document

## Overview

This document outlines a comprehensive testing framework for the NMEA 2000 Node implementation, leveraging the existing n2k pipeline infrastructure. The approach uses scenario-based testing with dynamic response injection and comprehensive output capture to validate Node behavior in controlled network conditions.

The framework operates entirely within the n2k package ecosystem, using proper PGN structs and the existing pipeline architecture for realistic testing without requiring live network hardware.

## Architecture Integration

```
Test Harness (Long-Running)
    ↓ (loads scenarios)
Test Scenario → TestOrchestrator
    ↓ (configures)         ↓ (coordinates)
Node Under Test ←→ Shared n2k Pipeline ←→ ResponseInjector
    ↓ (sends PGNs)       (Publisher/Subscriber)    ↑ (sends responses)
    ↓                           ↓                   ↑
    ↓                    OutputCapture              ↑
    ↓                    (via Subscriber)           ↑
    ↓                           ↓                   ↑
    └→ receives responses ←─────────────────────────┘
                  ↓
            Test Assertions
```

### Key Design Principles

1. **Shared Pipeline**: Both Node Under Test and ResponseInjector use the same n2k pipeline infrastructure
2. **PGN Struct Based**: All communication uses proper PGN structs, not raw bytes
3. **Long-Running Harness**: Test tool runs continuously, accepting scenario files for execution
4. **Pure Software**: No dependency on live network hardware or SocketCAN interfaces

## Key Components

### 1. Enhanced `convertcandumps` Command

**Current Capability:**
- Sorts raw files by PGN
- Handles multi-format conversions (raw, ydr, can, n2k)

**New Enhancement:**
Add `--consolidate-fast` flag to write fast format PGNs as single lines with extended data.

```bash
# Current multi-frame format (multiple lines per fast PGN)
2024-01-01T10:00:01Z,6,127489,50,255,8,00,8F,01,FF,FF,7F,FF,FF
2024-01-01T10:00:01Z,6,127489,50,255,8,01,FF,FF,FF,FF,FF,FF,FF
2024-01-01T10:00:01Z,6,127489,50,255,8,02,FF,FF,FF,FF,FF,FF,FF

# New consolidated format (single line with all data)
2024-01-01T10:00:01Z,6,127489,50,255,23,01,FF,FF,7F,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF,FF
```

**Benefits:**
- Easier test data creation and editing
- Clearer test scenarios (one line per logical message)
- Simplified pattern matching in tests
- `rawendpoint` handles frame sequencing automatically

### 2. Test Harness Framework

#### Core Structure
```go
type TestHarness struct {
    // Long-running test orchestration
    orchestrator *TestOrchestrator
    
    // Shared n2k pipeline components
    publisher   *pgn.Publisher
    subscriber  *subscribe.Manager
    
    // Test components
    responseInjector *ResponseInjector
    outputCapture   *OutputCapture
    
    // Currently loaded scenario
    currentScenario *TestScenario
    
    // Control
    running bool
    scenarioQueue chan string // File paths of scenarios to run
}

type TestOrchestrator struct {
    // Node under test
    testNode Node
    
    // Test environment coordination
    responseInjector *ResponseInjector
    outputCapture   *OutputCapture
    assertionRunner *AssertionRunner
    
    // Test execution
    scenario *TestScenario
    results  *TestResults
    
    // Timing control
    startTime time.Time
    phases    []TestPhase
}
```

#### Response Injection System
```go
type ResponseInjector struct {
    // n2k pipeline integration
    publisher *pgn.Publisher  // Sends real PGN structs
    
    // Dynamic response generation
    triggers []ResponseTrigger
    
    // State management
    triggerStates map[string]*TriggerState
    responseQueue chan *TimedResponse
}

type ResponseTrigger struct {
    Name        string
    Description string
    
    // Trigger conditions
    MatchPGN     uint32
    MatchSource  uint8
    MatchTarget  uint8
    MatchData    []byte     // Optional data pattern
    
    // Response generation (creates real PGN structs)
    ResponsePGN     uint32
    ResponseSource  uint8
    ResponseTarget  uint8
    ResponseData    []byte  // Used to construct proper PGN structs
    ResponseDelay   time.Duration
    
    // Trigger behavior
    TriggerOnce    bool
    MaxTriggers    int
    ActiveWindow   time.Duration  // Time window when trigger is active
}
```

### 3. Test Scenario Definition

#### Scenario Structure
```go
type TestScenario struct {
    Name        string
    Description string
    Duration    time.Duration
    
    // Node configuration
    NodeConfig NodeConfiguration
    
    // Network environment
    NetworkState NetworkState
    
    // Test phases
    Phases []TestPhase
    
    // Success criteria
    Assertions []TestAssertion
}

type NodeConfiguration struct {
    DeviceInfo         DeviceInfo
    ProductInfo        ProductInfo
    PreferredAddress   uint8
    SupportedPGNs      PGNSupport
    HeartbeatInterval  time.Duration
    CustomHandlers     map[uint32]string  // PGN -> handler name
}

type NetworkState struct {
    // Pre-existing devices on network
    ExistingDevices []SimulatedDevice
    
    // Static response files
    ResponseFiles []string
    
    // Dynamic response triggers
    ResponseTriggers []ResponseTrigger
}

type TestPhase struct {
    Name        string
    StartTime   time.Duration  // Relative to test start
    Duration    time.Duration
    
    // Actions to perform
    Actions []TestAction
    
    // Expected outcomes
    Expectations []TestExpectation
}
```

#### Test Actions
```go
type TestAction interface {
    Execute(sim *NodeSimulator) error
}

// Example actions
type StartNodeAction struct {
    PreferredAddress uint8
}

type InjectMessageAction struct {
    PGN         uint32
    Source      uint8
    Target      uint8
    Data        []byte
    Delay       time.Duration
}

type TriggerAddressConflictAction struct {
    ConflictingAddress uint8
    ConflictingNAME    uint64
}

type SendIsoRequestAction struct {
    RequestedPGN  uint32
    RequestSource uint8
}
```

### 4. Test Data Management

#### Test Data Structure
```
testdata/
├── scenarios/
│   ├── address_claiming/
│   │   ├── basic_claim.yaml
│   │   ├── address_conflict.yaml
│   │   ├── higher_priority.yaml
│   │   └── address_exhaustion.yaml
│   ├── iso_protocol/
│   │   ├── pgn_list_request.yaml
│   │   ├── product_info_request.yaml
│   │   └── unknown_pgn_request.yaml
│   └── heartbeat/
│       ├── periodic_heartbeat.yaml
│       └── heartbeat_control.yaml
├── network_captures/
│   ├── address_claims.raw
│   ├── iso_requests.raw
│   └── heartbeat_samples.raw
├── response_templates/
│   ├── address_claim_responses.raw
│   ├── iso_response_templates.raw
│   └── standard_responses.raw
└── device_profiles/
    ├── engine_device.yaml
    ├── navigation_device.yaml
    └── display_device.yaml
```

#### Scenario Definition Format (YAML)
```yaml
name: "Address Claim Conflict Resolution"
description: "Test node behavior when another device claims the same address"
duration: "30s"

node_config:
  device_info:
    unique_number: 12345
    manufacturer_code: 1851  # Hypothetical manufacturer
    device_function: 130     # Engine function
    device_class: 50         # Propulsion class
    device_instance_lower: 0
    device_instance_upper: 0
    system_instance: 0
    industry_group: 4        # Marine
    arbitrary_address_capable: true
  preferred_address: 50
  supported_pgns:
    transmit: [127488, 127489]  # Engine parameters
    receive: [59904]            # ISO Request
  heartbeat_interval: "60s"

network_state:
  response_files:
    - "testdata/response_templates/address_claim_responses.raw"
  
  response_triggers:
    - name: "address_conflict"
      description: "Another device claims same address"
      match_pgn: 60928        # ISO Address Claim
      match_target: 255       # Broadcast
      response_pgn: 60928
      response_source: 75     # Conflicting device
      response_target: 255
      response_data: [0x15, 0x00, 0x00, 0x00, 0x80, 0x8D, 0xC0, 0x09]  # Lower priority NAME
      response_delay: "100ms"
      trigger_once: true

phases:
  - name: "startup"
    start_time: "0s"
    duration: "5s"
    actions:
      - type: "start_node"
        preferred_address: 50
    expectations:
      - type: "message_sent"
        pgn: 60928
        source: 50
        target: 255
        timeout: "2s"
        description: "Node should send address claim"

  - name: "conflict_injection"
    start_time: "5s" 
    duration: "10s"
    actions:
      - type: "inject_message"
        pgn: 60928
        source: 75
        target: 255
        data: [0x15, 0x00, 0x00, 0x00, 0x80, 0x8D, 0xC0, 0x09]
        delay: "1s"
    expectations:
      - type: "address_changed"
        old_address: 50
        new_address: 51
        timeout: "5s"
        description: "Node should claim new address due to conflict"

assertions:
  - type: "final_address"
    expected_address: 51
    description: "Node should end up with address 51"
  - type: "message_count"
    pgn: 60928
    min_count: 2
    max_count: 3
    description: "Should send 2-3 address claims (initial + conflict resolution)"
```

### 5. Command Line Interface

#### Test Harness Commands
```bash
# Start long-running test harness
./nodetest --daemon --port 8080

# Run single scenario (daemon mode)
./nodetest --run-scenario testdata/scenarios/address_claiming/basic_claim.yaml

# Run single scenario (standalone mode)
./nodetest --scenario basic_claim.yaml --output results.json

# Interactive mode for debugging
./nodetest --scenario basic_claim.yaml --interactive --verbose

# Queue multiple scenarios (daemon mode)
./nodetest --queue-scenarios testdata/scenarios/address_claiming/*.yaml

# Monitor running tests
./nodetest --status
./nodetest --logs --follow

# Batch testing (standalone)
./nodetest --test-suite testdata/scenarios/address_claiming/ \
    --output-format junit \
    --parallel 4

# Configuration override
./nodetest --scenario basic_claim.yaml \
    --node-config preferred_address=60,manufacturer_code=1851
```

#### API Interface (Daemon Mode)
```bash
# HTTP API for scenario management
curl -X POST http://localhost:8080/scenarios \
    -H "Content-Type: application/json" \
    -d '{"file": "basic_claim.yaml"}'

# Get test results
curl http://localhost:8080/results/latest

# List running tests
curl http://localhost:8080/status
```

#### Enhanced convertcandumps Command
```bash
# New consolidate-fast flag
./convertcandumps --input network_capture.raw \
    --output test_data.raw \
    --consolidate-fast \
    --sort-by-pgn

# Create test data for specific PGNs
./convertcandumps --input large_capture.raw \
    --output address_claims.raw \
    --filter-pgn 60928 \
    --consolidate-fast

# Extract request/response pairs
./convertcandumps --input network_capture.raw \
    --output iso_requests.raw \
    --filter-pgn 59904,126464,126996 \
    --consolidate-fast \
    --group-by-sequence
```

### 6. Test Data Harvesting

#### Response Pattern Extraction
```go
type ResponseHarvester struct {
    inputFile     string
    patterns      []RequestResponsePattern
    outputDir     string
}

type RequestResponsePattern struct {
    RequestPGN     uint32
    ResponsePGNs   []uint32
    TimeWindow     time.Duration  // Max time between request and response
    SourceMatching bool           // Whether response source must match request target
}

// Usage example
harvester := ResponseHarvester{
    inputFile: "large_network_capture.raw",
    patterns: []RequestResponsePattern{
        {
            RequestPGN:   59904,  // ISO Request
            ResponsePGNs: []uint32{126464, 126996, 126998}, // PGN List, Product Info, Config Info
            TimeWindow:   time.Second * 5,
            SourceMatching: true,
        },
    },
    outputDir: "testdata/response_templates/",
}
```

### 7. Validation and Assertions

#### Test Assertion Types
```go
type TestAssertion interface {
    Validate(results *TestResults) error
}

// Message-based assertions
type MessageSentAssertion struct {
    PGN         uint32
    Source      uint8
    Target      uint8
    DataPattern []byte
    MinCount    int
    MaxCount    int
    TimeWindow  time.Duration
}

type MessageSequenceAssertion struct {
    Sequence []MessagePattern
    MaxGap   time.Duration
}

// State-based assertions
type AddressClaimedAssertion struct {
    ExpectedAddress uint8
    TimeoutToClaim  time.Duration
}

type HandlerCalledAssertion struct {
    HandlerName string
    PGN         uint32
    MinCalls    int
    MaxCalls    int
}

// Timing assertions
type HeartbeatIntervalAssertion struct {
    ExpectedInterval time.Duration
    Tolerance        time.Duration
    MinBeats         int
}
```

### 8. Implementation Phases

#### Phase 1: Infrastructure (Week 1-2) ✅ COMPLETE
1. ✅ Enhance `convertcandumps` with `--consolidate-fast` flag  
2. ✅ Create `ResponseInjector` with dynamic response generation
3. ✅ Implement `OutputCapture` with message validation
4. ✅ Create basic assertion framework

#### Phase 2: Test Harness (Week 3-4) 🎯 CURRENT
1. Create `TestOrchestrator` for coordinating test execution
2. Implement `TestHarness` with shared pipeline architecture
3. Build YAML scenario parser and configuration system
4. Add basic `nodetest` command with scenario execution

#### Phase 3: Long-Running Harness (Week 5-6)
1. Add daemon mode with HTTP API
2. Implement scenario queuing and management
3. Create real-time monitoring and logging
4. Build interactive debugging interface

#### Phase 4: Test Scenarios (Week 7-8)
1. Create comprehensive address claiming test scenarios
2. Implement ISO protocol test scenarios  
3. Add heartbeat and custom PGN test scenarios
4. Build CI integration and batch testing capabilities

## Test Scenarios

### Address Claiming Tests

#### 1. Basic Address Claim
- **Objective**: Verify node successfully claims preferred address
- **Setup**: Empty network, no conflicts
- **Expected**: Node claims address, sends ISO Address Claim
- **Assertions**: Address claimed within timeout, correct NAME field

#### 2. Address Conflict Resolution
- **Objective**: Test behavior when another device claims same address
- **Setup**: Node claims address, inject conflicting claim with lower priority NAME
- **Expected**: Node yields address, claims new available address
- **Assertions**: Address changes, new claim sent, no duplicate addresses

#### 3. Higher Priority Address Claim
- **Objective**: Verify node wins address conflict with higher priority
- **Setup**: Node claims address, inject conflicting claim with higher priority NAME
- **Expected**: Node retains address, sends repeat claim
- **Assertions**: Address unchanged, conflict response sent

#### 4. Address Exhaustion
- **Objective**: Test behavior when all preferred addresses are taken
- **Setup**: Pre-populate network with devices on addresses 128-247
- **Expected**: Node finds available address outside preferred range
- **Assertions**: Node claims valid address, address within allowed range

### ISO Protocol Tests

#### 5. PGN List Request Response
- **Objective**: Verify node responds to PGN list requests
- **Setup**: Node operational, inject ISO Request for PGN 126464
- **Expected**: Node responds with supported PGN list
- **Assertions**: Response sent to requester, correct PGN list format

#### 6. Product Information Request
- **Objective**: Test product information response
- **Setup**: Node configured with product info, inject ISO Request for PGN 126996
- **Expected**: Node responds with configured product information
- **Assertions**: Correct product data sent, proper message format

#### 7. Unknown PGN Request
- **Objective**: Verify handling of unsupported PGN requests
- **Setup**: Node operational, inject ISO Request for unhandled PGN
- **Expected**: Node sends ISO Acknowledgment with "Not Available"
- **Assertions**: NACK response sent, correct error code

### Heartbeat Tests

#### 8. Periodic Heartbeat Transmission
- **Objective**: Verify heartbeat sent at configured intervals
- **Setup**: Node with heartbeat enabled, 10-second interval
- **Expected**: Heartbeat PGN sent every 10 seconds
- **Assertions**: Timing within tolerance, sequence counter increments

#### 9. Heartbeat Control
- **Objective**: Test enabling/disabling heartbeat functionality
- **Setup**: Node operational, toggle heartbeat enable/disable
- **Expected**: Heartbeat starts/stops based on configuration
- **Assertions**: No heartbeats when disabled, resumes when enabled

### Custom Handler Tests

#### 10. Custom PGN Handler Registration
- **Objective**: Verify custom handlers receive and process PGNs
- **Setup**: Node with registered handler for specific PGN
- **Expected**: Handler called when PGN received, can send responses
- **Assertions**: Handler invoked, response data correct

## Benefits

### 1. Realistic Testing
- Uses actual network captures as basis for test data
- Simulates real device interactions and timing
- Tests complex multi-device scenarios

### 2. Comprehensive Coverage
- Address claiming edge cases (conflicts, priority, exhaustion)
- ISO protocol compliance (requests, responses, unknown PGNs)
- Custom PGN handler validation
- Heartbeat timing and control
- Multi-device network scenarios

### 3. Maintainable Test Suite
- YAML-based scenario definitions
- Reusable response templates
- Parameterized test configurations
- Clear separation of test data and test logic

### 4. Developer Experience
- Interactive debugging mode
- Clear test output and failure reporting
- Easy scenario creation and modification
- Integration with existing CI/CD pipeline

### 5. Scalable Architecture
- Parallel test execution
- Extensible assertion framework
- Pluggable response generation
- Support for custom device profiles

## Integration with Existing Pipeline

The testing framework leverages the existing n2k pipeline architecture with a shared Publisher/Subscriber system:

```go
// Test environment setup using shared n2k pipeline
func createTestEnvironment() (*TestHarness, error) {
    log := logrus.StandardLogger()
    
    // Create shared pipeline components
    adapter := canadapter.NewCANAdapter(log)
    packetStruct := pkt.NewPacketStruct()
    subscriber := subscribe.New()
    publisher := pgn.NewPublisher(adapter)
    
    // Wire the pipeline
    packetStruct.SetOutput(subscriber)
    adapter.SetOutput(packetStruct)
    
    // Create output capture (subscribes to outgoing messages)
    outputCapture := NewOutputCapture(subscriber, log)
    
    // Create response injector (publishes responses via publisher)
    responseInjector := NewResponseInjector(publisher, subscriber, log)
    
    // Create test orchestrator
    orchestrator := &TestOrchestrator{
        responseInjector: responseInjector,
        outputCapture:   outputCapture,
    }
    
    // Create Node Under Test with shared publisher/subscriber
    testNode := NewNode(subscriber, publisher)
    orchestrator.testNode = testNode
    
    return &TestHarness{
        orchestrator:     orchestrator,
        publisher:       publisher,  
        subscriber:      subscriber,
        responseInjector: responseInjector,
        outputCapture:   outputCapture,
    }, nil
}

// Example test execution flow
func (h *TestHarness) runAddressClaimTest() error {
    // 1. Node sends ISOAddressClaim via publisher
    // 2. OutputCapture receives it via subscriber
    // 3. ResponseInjector can send response via publisher (if configured)
    // 4. Node receives response via subscriber
    // 5. Test assertions validate the sequence
    
    // Node creates and sends address claim
    addressClaim := &ISOAddressClaim{
        UniqueNumber: 12345,
        ManufacturerCode: 1851,
        // ... other fields
    }
    
    // Node publishes through shared pipeline
    err := h.publisher.Write(addressClaim)
    if err != nil {
        return err
    }
    
    // OutputCapture automatically captures via subscription
    // ResponseInjector can respond if triggers match
    // Test assertions validate behavior
    
    return nil
}
```

### Pipeline Flow for Address Claim Test

```
1. Node Under Test
    ↓ publisher.Write(ISOAddressClaim)
2. Publisher → Adapter → PacketStruct → Subscriber
    ↓                                        ↓
3. OutputCapture.onMessage()         ResponseInjector.onMessage()
    ↓ (captures for validation)              ↓ (checks triggers)
4. Test Assertions                    publisher.Write(ResponseClaim)
    ↓                                        ↓
5. Validate address claim sent         Back to step 2 (response flows through)
                                              ↓
                                       Node receives response
```

## Success Metrics

1. **Coverage**: All Node interface methods tested
2. **Realism**: Test scenarios based on real network captures
3. **Maintainability**: New test scenarios created in < 30 minutes
4. **Reliability**: Tests pass consistently with deterministic results
5. **Performance**: Full test suite runs in < 5 minutes
6. **Documentation**: Clear examples for each Node feature

## Dependencies

### Required Enhancements
- `convertcandumps`: Add `--consolidate-fast` flag for single-line fast PGN format ✅
- `rawendpoint`: Verify multi-frame handling from consolidated format
- New `nodetest` command: Long-running test harness with daemon mode
- Pipeline integration: Shared Publisher/Subscriber architecture

### Optional Enhancements
- HTTP API for test management in daemon mode
- Interactive debugging interface with step-through capability
- Real-time test monitoring and logging
- Scenario validation and syntax checking

This comprehensive testing framework provides the foundation for thoroughly validating the Node implementation while maintaining the flexibility to add new test scenarios as the Node interface evolves. 