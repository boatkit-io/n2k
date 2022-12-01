## boatkit-io/n2k

boatkit-io/n2k comprises packages (and associated tools) that convert a canbus-based stream of NMEA 2000 messages into strongly typed [go](https://go.dev) data structures. It provides a subscription model enabling registered clients to receive complete NMEA 2000 message instances for further processing.

[NMEA 2000](https://www.nmea.org/content/STANDARDS/NMEA_2000) is a proprietary industry standard for inter-connecting marine electronic devices. This project leverages the great work of the [canboat](https://github.com/canboat/canboat) open-source project that has "reverse engineered the NMEA 2000 database by network observation and assembling data from public sources."

The canboat project includes and references valuable documentation for potential users of these packages. This project's documentation assumes readers are familiar with and can reference that documentation as needed.

### Parts list

The *pgngen* tool downloads (and caches) the "canboat.json" file from the [canboat project] (https://github.com/canboat/canboat) and generates "pgninfo_generated.go" into the source tree for the n2k package. This file includes declarations for enumerations, populated structures for each pgn and field, and go functions (decoders) that marshals data received on the wire into the go struct. The build system first runs this tool before building the n2k package.

The *convertcandumps* tool translates between a number of can dump file formats. Run "convertcandumps --help" for more details. It optionally groups records by PGN, allowing testing on (a)particular PGN(s). You can translate other formats to the "n2k" format used by the replay tool described next.

The *replay* tool converts a specified (n2k format) dump file into a series of canbus frames and feeds them into a recognizer that converts them into complete higher-level frames, invokes the matching decoders, and invokes functions subscribed to specific or all pgns. It allows alternative providers of canbus frames to register. It provides logging options to generate json structures for all, only recognized, or only unrecognized results. Use this tool to understand how to integrate these packages into your go program.

## Overview of canbus stream processing

To connect a canbus frame stream use (https://github.com/boatkit-io/canbus) package. Provide the n2k interface name to NewService, which will open a channel via the canbus package. To replay dump files use the replay tool, described above.

Service.Run connects the handler (s.pgnBuilder.ProcessFrame) with the canbus channel. 
- For each canbus frame received by the handler:
- create an expanded data structure (Package)
- Assure the PGN and data lengths are non-zero
- Determine if we have one or more candidate recognizers for the PGN (generated from canboat.json)
- if known, determine if it's a fast(potentially requires combining multiple canbus frames) or single (canbus frame) pgn.
- if fast, and not the initial frame of the sequence, add it to a structure that caches the partial data and assembles the complete package for further processing. Note that these continuation frames can be received out of order.
- if it's the initial fast frame or a single frame
- determine if the pgn is proprietary
- if proprietary, extract the manufacturer id
- match the potential decoders for the pgn (using the pgn and manufacturer id for proprietary messages)
- apply the decoders to the single or complete multiframe message contents
- if a decoder completes without error, call registered listeners with the unmarshaled go structures
- otherwise create an unknown pgn structure and pass to any registered unknown pgn listeners
* Note that one proprietary PGN, 130824, has both a fast and a single variant that requires special handling.
* Also note that some devices and firmware versions send incomplete messages. Where possible decoders provide partial results.

## Build instructions

To be provided

## Version History

See [changelog](./changelog.md)

## Related Projects

* [canboat](https://github.com/canboat/canboat) An open source collection of command line tools and data relevant to NMEA 2000 boat networks
* tbd

## License
[n2k license](./LICENSE)



