# Testing Alternatives

This project can be tested with live CAN hardware, Linux virtual CAN, or replay files. Use the lightest option that exercises the behavior you need.

## Virtual CAN

Linux `vcan` interfaces are supported through the SocketCAN endpoint when using tugboat `v0.9.0` or newer.

The SocketCAN path handles `vcan` differently from real CAN:

- accepts `vcan` as a valid link type
- skips bitrate inspection and configuration
- brings a down `vcan` link up with `ip link set <iface> up`
- uses `brutella/can` for frame I/O after the link is ready

Set up a virtual CAN interface:

```bash
sudo modprobe vcan
sudo ip link add dev vcan0 type vcan
sudo ip link set vcan0 up
```

Run a listener:

```bash
go run ./cmd/dumpcan -iface vcan0
```

Send sample NMEA 2000 frames from another terminal:

```bash
go run ./cmd/canwriter -iface vcan0 -count 3 -interval 50ms -pattern nmea -pgn 127488 -source 110 -verbose
```

`dumpcan` should decode those frames as `EngineParametersRapidUpdate`.

## Replay Files

Replay files are the best option for deterministic parser and subscription tests. They do not require CAN hardware, kernel CAN modules, or elevated privileges.

```bash
go run ./cmd/replay -replayFile /path/to/capture.n2k
go run ./cmd/replay -rawReplayFile /path/to/capture.raw
```

Use `convertcandumps` to convert raw candump output into replayable data:

```bash
go run ./cmd/convertcandumps -input capture.log -output capture.raw
```

## Real CAN Hardware

Use real hardware when validating link setup, bus behavior, device timing, or interactions with actual NMEA 2000 devices.

```bash
go run ./cmd/dumpcan -iface can0
go run ./cmd/spewpgns -iface can0
```

Real CAN links require bitrate configuration. The SocketCAN endpoint configures CAN links for the requested bitrate before opening the bus.

## Choosing An Option

- Use replay files for repeatable automated tests.
- Use `vcan0` for local end-to-end SocketCAN testing without hardware.
- Use real CAN hardware before relying on behavior that depends on adapters, wiring, bus load, or physical devices.
