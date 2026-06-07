<!-- markdownlint-disable first-line-heading -->
## [unreleased]

### Features

- Added `pkg/node`, a higher-level NMEA 2000 node API layered on `N2kService`.
  It handles device identity, address claiming, management PGN subscriptions,
  heartbeats, standard request responses, and safe node-scoped writes.
- Added read-only node mode. Nodes now default to passive monitoring and can be
  kept read-only with address `255`, allowing discovery and device-map updates
  without transmitting on the NMEA 2000 bus.
- Added known-device tracking to `pkg/node`, including address-claim observation,
  address-change detection, product/configuration info capture, PGN list capture,
  and device-change subscriptions.
- Added handling for NMEA group-function requests needed by node management,
  including partial decode support for PGN `126208`.
- Split generated public PGN types into `pkg/pgn` while keeping runtime decode
  and encode implementation details under `internal/pgn`.
- Added CAN frame hooks and replay-frame handling to `N2kService`, allowing live
  traffic capture and replay traffic to share the same decode/subscription
  pipeline.
- Added discriminator code generation and domain-range handling for PGNs that
  require field-driven type selection.
- Added and expanded command-line tooling for observing, replaying, filtering,
  writing, and benchmarking NMEA 2000 traffic.

### Fixes

- Fixed decoding of repeating dynamic-length PGN value fields.
- Fixed offset-only numeric fields so generated code preserves integer types
  where appropriate.
- Tightened node protocol behavior around address ownership, request handling,
  source-address assignment, and write gating.
- Improved CAN interface error handling and endpoint shutdown reporting.

### Documentation

- Rewrote `README.md` around the current package structure, node lifecycle,
  read-only behavior, known-device tracking, command usage, and development
  workflow.
- Expanded node behavior design documentation to describe lifecycle, address
  claiming, management request handling, and read-only monitoring behavior.
- Added virtual CAN limitations and testing notes for integration development.

### Development

- Reworked the mise task setup for code generation, builds, tests, formatting,
  linting, and release changelog generation.
- Added and updated tests for PGN generation, data-stream encoding/decoding,
  discriminator/domain behavior, node management behavior, read-only mode, and
  integration replay paths.
