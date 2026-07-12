# boatkit-io/n2k

`boatkit-io/n2k` is a Go library for reading, decoding, encoding, and writing
NMEA 2000 traffic. It provides:

- Endpoint implementations for CAN transports and captured logs.
- A decode/encode service that turns CAN frames into typed PGN structs.
- Generated public PGN types in `pkg/pgn`, based on CANboat data.
- A higher-level `pkg/node` API for standard NMEA 2000 node behavior such as
  address claiming, request handling, heartbeats, and observed-device tracking.

NMEA 2000 is a proprietary marine networking standard. This project relies on
the public reverse-engineering work in
[canboat](https://github.com/canboat/canboat) for PGN definitions.

## Packages

### `pkg/pgn`

Generated public PGN structs, constants, and enum values. Application code uses
these types when subscribing to decoded traffic or publishing messages.

### `pkg/endpoint`

Transport boundary for CAN frames. Endpoints implement:

```go
type Endpoint interface {
    Run(ctx context.Context) error
    Close() error
    SetOutput(MessageHandler)
    WriteFrame(can.Frame)
}
```

Current endpoint packages include SocketCAN, USB CAN, raw replay, and N2K file
support.

### `pkg/n2k`

The public message-processing service. `N2kService` connects an endpoint to the
CAN adapter, packet decoder, PGN struct decoder, subscription manager, and PGN
publisher.

Use it when an application needs direct access to decoded PGNs:

```go
endpoint := socketcanendpoint.NewSocketCANEndpoint(log, "can0")
svc := n2k.NewN2kService(endpoint, log)

_, err := svc.SubscribeToStruct(pgn.EngineParametersRapidUpdate{}, func(msg pgn.EngineParametersRapidUpdate) {
    // handle decoded engine update
})
if err != nil {
    return err
}

if err := svc.Start(ctx); err != nil {
    return err
}
defer svc.Stop()
```

Calling `N2kService.Write` writes directly to the bus. Use `pkg/node` when the
application should only write after a node has explicitly claimed an address.

### `pkg/node`

`pkg/node` provides standard NMEA 2000 node behavior on top of `N2kService`.
This is the intended entry point for applications that need a network identity,
address-claim lifecycle, standard management responses, heartbeat support, and a
device map.

```go
nodeImpl := node.NewFromService(svc)

err := nodeImpl.SetDeviceInfo(node.DeviceInfo{
    UniqueNumber:            123456,
    ManufacturerCode:        pgn.Garmin,
    DeviceFunction:          140,
    DeviceClass:             pgn.Navigation,
    IndustryGroup:           pgn.MarineIndustry,
    ArbitraryAddressCapable: true,
})
if err != nil {
    return err
}

nodeImpl.SetProductInfo(node.ProductInfo{
    NMEA2000Version: 2100,
    ProductCode:     101,
    ModelID:         "Example Node",
})

if err := nodeImpl.Start(); err != nil {
    return err
}
defer nodeImpl.Stop()

// 255 keeps the node in passive read-only monitoring mode.
if err := nodeImpl.ClaimAddress(node.ReadOnlyAddress); err != nil {
    return err
}
```

## Read-Only And Address Claiming

Nodes default to read-only monitoring. In read-only mode, the node still:

- Subscribes to management PGNs.
- Observes address claims and bus traffic.
- Builds and updates the known-device map.
- Publishes device-change callbacks.

In read-only mode, the node does not:

- Claim an address.
- Send address-claim frames.
- Send heartbeats.
- Respond to ISO requests, commanded-address requests, or group-function writes.
- Honor `Write` or `WriteTo` requests.

Use `node.ReadOnlyAddress` (`255`) or `ClaimAddress(255)` to keep the node
passive. Use `ClaimAddress(0..253)` to leave read-only mode and begin the
normal address-claim flow. Address `254` is the ISO null address and is rejected.

Starting an `N2kService` or a `Node` does not write to the bus by itself. A node
writes only after the client explicitly claims a writable address, or when the
client calls the lower-level `N2kService.Write` API directly.

## Known Devices

`Node.KnownDevices()` returns devices observed on the bus. The node tracks
devices by NAME when available, updates address changes, records product and
configuration information when responses are observed, and can notify callers
through `SubscribeToDeviceChanges`.

Read-only nodes still maintain this map, which makes passive monitoring useful
for applications that need discovery without transmitting on the NMEA 2000 bus.

## Commands

### `cmd/nodeintegration`

Runs a SocketCAN-backed integration exerciser for node behavior.

```sh
go run ./cmd/nodeintegration -iface can0
go run ./cmd/nodeintegration -iface can0 -address 110
go run ./cmd/nodeintegration -iface can0 -address 255
```

The default address is `255`, so the command starts in read-only monitoring
mode unless a writable address is provided. In read-only mode it skips active
request PGNs and only observes bus traffic.

### `cmd/dumpcan`

Connects to a CAN interface and dumps decoded NMEA 2000 PGNs to standard
output.

```sh
go run ./cmd/dumpcan -iface can0
```

### `cmd/convertcandumps`

Converts captured CAN/NMEA 2000 logs between supported formats, including
Yacht Devices `.ydr`, CANboat analyzer `.raw`, CANView `.CAN`, and Linux
`candump`-style `.n2k` files.

### `cmd/replay`

Replays converted `.n2k` files through the decode pipeline and prints the
resulting Go structs. This is useful for understanding real traffic and for
testing decoder behavior.

### `cmd/pgngen`

Generates PGN runtime and public types from CANboat PGN data. Generated output
is written under `internal/pgn`, `pkg/pgn`, and `cmd/filterraw`.

## Development

This repository uses [mise](https://mise.jdx.dev/) tasks.

```sh
mise run codegen
mise run build
mise run test
mise run test-integration
mise run test-release
mise run lint
mise run fmt
```

`mise run build` runs code generation when generated files are stale, then
builds all commands under `./cmd/...`.

`mise run test` is the fast development suite and skips replay-heavy integration
tests. `mise run test-integration` runs the replay corpus explicitly. Before a
release, run `mise run test-release`; it checks the pinned CANboat input, runs
lint, the full replay suite, race tests, build/codegen, and verifies generated
files are up to date.

PGN generation is locked to a specific CANboat release for reproducibility. Use
`mise run check-canboat` to report whether a newer stable CANboat release exists.
A newer upstream release is reported as a warning so bugfix releases can still
ship against the pinned input.

Run the full Go test suite directly with:

```sh
go test ./...
```

## Virtual CAN

Virtual CAN interfaces are supported for development and integration testing.
See [docs/testing-alternatives.md](docs/testing-alternatives.md) for setup and
testing options.

## Related Projects

- [canboat](https://github.com/canboat/canboat): open source NMEA 2000 tooling
  and PGN data.
- [brutella/can](https://github.com/brutella/can): CAN frame support used by
  this package.

## License

This project is licensed under the MIT license. See [LICENSE](LICENSE).
