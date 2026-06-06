# NMEA 2000 Node Behavior Design

## Purpose

`pkg/node` should provide the standard behavior expected from a transmitting
NMEA 2000 node. Applications should be able to configure identity, product
metadata, supported PGNs, and application PGN handlers, then rely on the node
to perform the common network-management work required before sending data.

This document is a behavioral requirements document for `pkg/node`, not a copy
of the NMEA 2000 standard. It is based on public PGN definitions, observed
behavior in open-source implementations, and the current package API. When exact
conformance matters, the official NMEA 2000 and ISO 11783 documents remain the
authority.

## Scope

The node package is responsible for:

- Device identity and NAME construction.
- Address claim lifecycle.
- Responding to standard management requests.
- Sending standard metadata/status PGNs.
- Tracking other devices enough to manage address contention.
- Providing a safe write API that uses the claimed source address.
- Exposing lifecycle and state transitions that applications can test.

The node package is not responsible for:

- CAN frame encoding and decoding.
- Fast-packet fragmentation/reassembly, except where node behavior depends on
  the completed PGN.
- ISO transport protocol segmentation/reassembly.
- Domain-specific PGN generation such as engine, navigation, tank, or battery
  data.

## Sources And Reference Implementations

Public references used for this design:

- CANboat PGN documentation:
  https://canboat.github.io/canboat/canboat.html
- ttlappalainen NMEA2000 library documentation and source:
  https://ttlappalainen.github.io/NMEA2000/
- open-ships/n2k package documentation:
  https://pkg.go.dev/github.com/open-ships/n2k
- Public vendor PGN lists, for example Garmin VHF 315:
  https://www8.garmin.com/manuals/webhelp/GUID-395B9869-0DCF-4E17-A266-0D9FF08A27DE/EN-US/GUID-8E7A6446-ACB3-477F-9216-5B15325BB783.html
- NMEA public overview:
  https://www.nmea.org/nmea-2000.html

## Design Principles

- A node must not transmit application PGNs until it has claimed a source
  address.
- Address claim behavior must be deterministic and testable.
- The public API should keep applications out of common management details.
- The package should preserve existing pipeline boundaries: subscribe to decoded
  PGN structs, publish decoded PGN structs, and let lower layers handle framing.
- Missing or unsupported behavior should fail explicitly where possible rather
  than silently impersonating a complete NMEA 2000 node.

## Public API

The current API is a good starting point:

```go
type Node interface {
    Start() error
    Stop() error

    ClaimAddress(preferredAddress uint8) error
    GetNetworkAddress() uint8
    IsAddressClaimed() bool
    KnownDevices() []KnownDevice

    Write(pgnStruct any) error
    WriteTo(pgnStruct any, destination uint8) error

    SetDeviceInfo(info DeviceInfo) error
    SetProductInfo(info ProductInfo)
    SetConfigurationProvider(provider ConfigurationProvider)
    SetSupportedPGNs(transmit, receive []uint32)

    SetHeartbeatInterval(interval time.Duration)
    EnableHeartbeat(enable bool)
}
```

Future API additions should be considered for:

- Reading the address-claim state and failure reason.
- Registering application-level ISO request handlers.
- Choosing fixed-address versus arbitrary-address retry behavior.

## Lifecycle

### Start

`Start` must:

- Subscribe to PGNs required for node management.
- Create the node processing context and goroutine.
- Be idempotence-safe: calling `Start` on an already-started node returns an
  error and must not create duplicate subscriptions.
- If `ClaimAddress` was called before `Start`, begin address claiming after the
  processing goroutine starts.

Required subscriptions:

- 59392, ISO Acknowledgement.
- 59904, ISO Request.
- 60928, ISO Address Claim.
- 65240, ISO Commanded Address.
- 126208, NMEA Group Function, once group function support exists.

Transport protocol PGNs 60160 and 60416 are conditional lower-layer
infrastructure, not required node-management subscriptions. They should be
handled by the transport layer if ISO transport support exists. If no lower
layer owns ISO transport, `pkg/node` should document that limitation rather
than subscribing to transport-control PGNs as if they were ordinary node events.

### Stop

`Stop` must:

- Unsubscribe every subscription created by `Start`.
- Stop all node-owned timers.
- Stop the processing goroutine.
- Reset address state to unclaimed and network address to 255.
- Leave configured identity, product info, supported PGNs, and heartbeat
  settings intact so the node can be restarted.
- Return an error if called when not started.

## Device Identity And NAME

`SetDeviceInfo` must validate field sizes before accepting identity data:

- Unique number: 21 bits.
- Manufacturer code: 11 bits.
- Device instance lower: 3 bits.
- Device instance upper: 5 bits.
- Device function: 8 bits.
- Device class: 7 bits.
- System instance: 4 bits.
- Industry group: 3 bits.
- Arbitrary address capable: 1 bit.

The computed 64-bit NAME is used for address arbitration. Lower numeric NAME
has higher priority. The node must use the same packing when sending address
claims, handling commanded address, and comparing competing claims.

`SetDeviceInfo` should be required before claiming an address. If it has not
been called, `ClaimAddress` or `Start` should fail rather than claiming with a
zero NAME.

## Address Claiming

### Address Values

- 0 through 253 are usable source addresses in the current API.
- 254 is the null address and must not be claimed.
- 255 is the global address and must not be used as a source for normal
  application PGNs.

### Claim Sequence

When `ClaimAddress(preferredAddress)` is called:

1. Validate that the preferred address is claimable.
2. Set the node state to `claiming`.
3. Set the tentative network address to the preferred address.
4. Broadcast ISO Address Claim, PGN 60928, priority 6, target 255.
5. Listen for competing address claims for the same source address.
6. After the claim timeout, transition to `claimed` if no higher-priority
   contender won the address.

The default claim timeout should be configurable. A 250 ms timeout matches common
open-source behavior; longer timeouts may be useful on busy or bridged networks.

### Contention

When another device claims the node's tentative or claimed address:

- Compute the incoming NAME from PGN 60928.
- If the incoming NAME is lower than our NAME, the incoming device wins.
- If our NAME is lower than the incoming NAME, keep the address and immediately
  broadcast our own address claim.
- If the NAMEs are equal, treat it as a duplicate identity. The node should
  surface an error or state transition instead of silently accepting ambiguous
  identity. If a future implementation intentionally follows the NMEA2000
  library pattern of changing device instance on duplicate NAME, that behavior
  must be explicit and configurable.

When the node loses an address:

- If arbitrary addressing is enabled, choose another candidate address and
  restart the claim sequence.
- If arbitrary addressing is disabled or no candidate address remains, transition
  to an address-lost state, set source address to 254 or 255 as appropriate for
  the lower layer, and reject application writes.

Candidate address selection should avoid addresses currently known to be
claimed. For automatic selection, prefer deterministic behavior so tests and
logs are understandable.

### Known Device Map

The node should maintain a map of known devices keyed by source address:

```go
type KnownDevice struct {
    Address     uint8
    Name        uint64
    LastSeen    time.Time
    ProductInfo *ProductInfo
    ConfigInfo  *ConfigurationInfo
}
```

The map must be updated when address claims are received. If a source address is
claimed by a different NAME, the old entry must be replaced. Entries may expire
after a configurable idle period, but expiration must not cause the node to
forget an address during an active claim wait.

`KnownDevices` returns a sorted snapshot of this map so callers can inspect
observed devices without mutating node internals. `cmd/nodeintegration` uses
that snapshot to dump the device list when it changes and before exiting.

## Required Standard PGN Behavior

### Transmit

The node should transmit or be able to transmit:

- 59392, ISO Acknowledgement.
- 59904, ISO Request, when the node API exposes active requests.
- 60928, ISO Address Claim.
- 126464, PGN List, in response to requests.
- 126993, Heartbeat, when enabled.
- 126996, Product Information, in response to requests.
- 126998, Configuration Information, in response to requests once configured.

Conditional transport support:

- 60160, ISO Transport Protocol Data Transfer.
- 60416, ISO Transport Protocol Connection Management.

These PGNs are not required transmitted PGNs for every device implementation.
They are used only when ISO transport protocol is needed. Many NMEA 2000 devices
and replays never use them because their traffic is single-frame or fast-packet.
If ISO transport is implemented below `pkg/node`, the node must advertise
transport-dependent capabilities only when they are actually available.

### Receive

The node must receive and handle:

- 59392, ISO Acknowledgement.
- 59904, ISO Request.
- 60928, ISO Address Claim.
- 65240, ISO Commanded Address.

The node should receive and handle or delegate:

- 126208, NMEA Group Function.

Conditional lower-layer receive support:

- 60160, ISO Transport Protocol Data Transfer.
- 60416, ISO Transport Protocol Connection Management.

These are transport PGNs. `pkg/node` should normally see only the reassembled
application or management PGN, not the transport session frames themselves.

## ISO Request Handling

For PGN 59904, the node must ignore requests when:

- The requested PGN field is absent or invalid.
- The request is destination-specific and the destination is neither this node's
  address nor the global address.
- The node has not claimed an address, except for address-claim behavior that is
  explicitly allowed during claiming.

Built-in request responses:

- Request for 60928: send ISO Address Claim.
- Request for 126464: send PGN List for transmit and receive lists.
- Request for 126996: send Product Information if configured.
- Request for 126998: send Configuration Information if configured.

For unsupported requests directed at this node, the node should send ISO
Acknowledgement with a negative acknowledgement or access-denied/control code
appropriate to the PGN and request type. For global unsupported requests, the
node may remain silent to avoid unnecessary bus traffic unless the standard
requires a response for that PGN.

Applications must be able to register handlers for application PGNs. If a
handler is registered, the node should route matching ISO Requests to it before
sending a default negative acknowledgement.

## Product Information

`SetProductInfo` configures PGN 126996 responses.

Requirements:

- Product code, model ID, software version, model version, serial code,
  certification level, and load equivalency must be preserved exactly as
  configured within generated PGN field constraints.
- NMEA 2000 version scaling must be documented and consistently converted.
- Empty product information should either be rejected during configuration or
  produce an explicit "not configured" response path.

## Configuration Information

The node should support PGN 126998, but the configuration data belongs to the
device-specific software. `pkg/node` should provide protocol plumbing, request
routing, and acknowledgement behavior; it should not decide what configuration
values mean or whether a requested change is valid for a particular device.

Recommended data model:

```go
type ConfigurationInfo struct {
    ManufacturerInformation string
    InstallationDescription1 string
    InstallationDescription2 string
}
```

Recommended API shape:

```go
type ConfigurationProvider interface {
    GetConfigurationInfo() (ConfigurationInfo, error)
    SetConfigurationInfo(ConfigurationInfo) error
}

func SetConfigurationProvider(provider ConfigurationProvider)
```

At initialization, device-specific software should provide either static
configuration information or a provider that can read the current values from
device state. Requests for PGN 126998 should call the provider and return the
current configuration information.

Configuration changes must be acted upon by device-specific software. When the
node receives a supported request/command group function that changes
configuration information, it should:

1. Decode the requested change into `ConfigurationInfo`.
2. Call the configured provider or change handler.
3. Send a positive acknowledgement only if the device-specific handler accepts
   and applies or persists the change.
4. Send a negative acknowledgement when no handler is configured, the requested
   change is invalid, persistence fails, or the device rejects the change.

The node should not mutate cached configuration data before the device-specific
handler has accepted the change. If a handler is not configured, requests for
126998 should follow the unsupported or not-available ISO acknowledgement
policy.

Open design question: whether configuration changes should be represented by a
single read/write provider, separate read and change callbacks, or a more
general group-function handler. The behavioral requirement is that the device
application remains the authority for configuration values and side effects.

## PGN List

`SetSupportedPGNs(transmit, receive)` configures PGN 126464 responses.

Requirements:

- Include node-managed PGNs automatically unless the caller explicitly disables
  automatic management PGNs.
- Include application transmit and receive PGNs configured by the caller.
- Send separate transmit and receive list responses.
- Use transport/fast-packet support if the list is too large for a single
  packet at the lower layers.
- Keep the list stable and sorted unless caller order is deliberately preserved.

## Heartbeat

When enabled, heartbeat sends PGN 126993.

Requirements:

- Heartbeat must only start after the node has claimed an address.
- Send an initial heartbeat soon after address claim, then periodically.
- Default interval should be 60 seconds.
- Sequence counter increments modulo 256.
- Controller and equipment states should be configurable or derived from lower
  layer health in a future implementation. The current default can be
  error-active and operational.
- Disabling heartbeat stops the timer without changing other node state.

## ISO Commanded Address

When receiving PGN 65240:

- Ignore the command if the embedded NAME does not match this node.
- Ignore broadcast/null/new addresses that cannot be claimed.
- If the command matches this node and requests a different address, update the
  preferred address and restart address claiming.
- While re-claiming, reject application writes.

PGN 65240 is commonly transported as a multi-packet message. If lower layers do
not reassemble it into `pgn.IsoCommandedAddress`, this behavior cannot work and
the transport gap must be closed first.

## Write Behavior

`Write` and `WriteTo` must:

- Return an error if the node has not claimed an address.
- Set the PGN source address to the claimed node address.
- Set the destination for destination-specific writes.
- Preserve caller-specified PGN and priority unless the generated PGN metadata
  requires a default.
- Return publisher errors to the caller.

The node should not mutate user PGN structs after a failed write except for
documented `MessageInfo` source/destination updates.

## Transport And Fast Packet

NMEA 2000 uses both fast-packet messages and ISO transport protocol messages.
Fast packet is much more common in ordinary NMEA 2000 traffic. ISO transport
protocol, using PGNs 60416 and 60160, appears much less frequently in observed
candump/replay traffic and may not be supported by many device implementations.

`pkg/node` should avoid owning fragmentation. The behavioral requirement is:

- Node request/response behavior must work for management PGNs regardless of
  whether the encoded PGN is single-frame, fast-packet, or ISO transport.
- Product Information, Configuration Information, PGN List, and Commanded
  Address must not be limited by single-frame assumptions.
- ISO transport support is conditional. If it is unavailable, node must document
  and expose that limitation instead of advertising unsupported capabilities.
- `pkg/node` should not treat PGN 60416 responses as expected node-level
  behavior. Broadcast ISO transport may not have a response, and peer-to-peer
  RTS/CTS responses only occur when a target device participates in that
  transport session.

## Error Handling And Observability

The node should expose or log:

- Address claim started.
- Address claim succeeded.
- Address claim lost.
- Duplicate NAME detected.
- No address available.
- ISO request received and response decision.
- Standard PGN response write failures.
- Heartbeat write failures.

Public methods should return errors for caller-controlled failures. Background
failures should be observable through state, callbacks, logs, or a future event
channel.

## Thread Safety

All public methods must be safe to call from concurrent goroutines.

Internal requirements:

- Do not hold locks while calling user callbacks or publisher/subscriber
  methods.
- Timers must be stopped before being discarded.
- Stop must wait for the processing goroutine to exit.
- Subscription callbacks must tolerate shutdown races.

## Current Implementation Gap Analysis

Implemented or mostly implemented:

- Start/stop lifecycle with subscriptions.
- NAME validation and packing.
- Basic address claim send and 250 ms claim wait.
- Basic conflict handling when another lower NAME claims our address.
- Basic product information response.
- Basic configuration information response through a device provider.
- ISO Request for Address Claim.
- Basic PGN list response.
- Basic commanded-address handling.
- ISO Acknowledgement subscription and logging.
- Basic ISO NAK response for unsupported Configuration Information requests.
- Basic Group Function PGN 126208 request routing and unsupported command NAKs.
- Automatic inclusion of node-managed PGNs in PGN List.
- Package-level known-device snapshots, updated from address claims and enriched
  from observed Product Information and Configuration Information PGNs.
- Deterministic retry to the next known-free address after losing an address
  claim when arbitrary addressing is enabled.
- Heartbeat timer and sequence counter.
- Write gating until an address is claimed.
- Identity-before-claim validation.

Missing or incomplete:

- ISO Acknowledgement handling beyond logging, if callers need state changes or
  error callbacks from received ACK/NAK PGNs.
- Configuration Information change handling via Group Function write/command
  operations.
- Known-device expiry and configurable idle timeout.
- No-address-available reporting when all candidate addresses are known claimed.
- Duplicate-NAME handling.
- Configurable fixed-address versus arbitrary-address policy beyond the current
  `DeviceInfo.ArbitraryAddressCapable` behavior.
- Application ISO request handler routing.
- Advanced Group Function PGN 126208 support, including transmission interval,
  priority, and parameter write semantics.
- Clear transport/fast-packet capability boundary for node-managed PGNs,
  especially documenting ISO transport as conditional rather than mandatory.
- State/error observability beyond `IsAddressClaimed` and logs.

## Test Requirements

Unit tests should cover:

- NAME packing and validation limits.
- Start/stop subscription cleanup.
- Claim success after timeout.
- Claim before start and start before claim.
- Losing address to lower NAME.
- Keeping address against higher NAME and re-broadcasting claim.
- Duplicate NAME behavior.
- Arbitrary-address retry and no-address-available failure.
- Write rejection before claim and during re-claim.
- MessageInfo source/destination mutation on write.
- ISO Request filtering by destination.
- ISO Request responses for 60928, 126464, 126996, and 126998.
- Unsupported directed request acknowledgement policy.
- Heartbeat initial send, interval send, disable behavior, and sequence wrap.
- Commanded address matching and non-matching NAMEs.

Integration tests should verify:

- A node can claim an address on vcan/socketcan.
- A second node with a lower NAME wins contention.
- Product Information and PGN List can be requested by another participant.
- Fast-packet management PGNs are encoded and decoded by the lower pipeline as
  needed.
- ISO transport-backed management PGNs are verified only when the lower pipeline
  advertises ISO transport support.

## Suggested Implementation Order

1. Add state/error observability beyond `IsAddressClaimed` and logs.
2. Add duplicate-NAME handling and no-address-available reporting.
3. Add Configuration Information writes via Group Function command/write paths.
4. Add application ISO request handler routing.
5. Decide and document the transport/fast-packet ownership boundary, with ISO
   transport marked as conditional lower-layer support.
6. Add advanced Group Function support if required for target interoperability.
