# Golang NMEA 2000 Library
## boatkit-io/n2k

boatkit-io/n2k comprises packages (and associated tools) supporting the exchange of NMEA 200 messages across a range of transports. Client go code can send and receive strongly typed [go](https://go.dev) data structures, with the library translating to/from a stream of NMEA 2000 messages. 

[NMEA 2000](https://www.nmea.org/content/STANDARDS/NMEA_2000) is a proprietary industry standard for inter-connecting marine electronic devices. This project leverages the great work of the [canboat](https://github.com/canboat/canboat) open-source project that has "reverse engineered the NMEA 2000 database by network observation and assembling data from public sources."

The canboat project includes and references valuable documentation for potential users of this package. This project's documentation assumes readers are familiar with and can reference that documentation as needed.

## Processing Overview

### Endpoint

Responsible for managing the interaction with the nmea gateway (or to replay a recording of message frames received from a gateway or other source). To support a new gateway create a new implementation that supports this interface definition.

The endpoint puts new message frames onto a channel. The data format is determined by the gateway or other source.

### Frame to Packet Adapter

Responsible for generating a "packet" from frames received on its input channel, and putting complete packets onto its output channel. The packet is an intermediate format used by subsequent processors.

This adapter can access a number of helper functions:
- is the PGN known (defined in canboat)
- is it Proprietary? 
- is it Fast or Single

### Packet to Struct Adapter

Receives packet on its input channel, decodes it, and puts the resulting Go struct on its output channel (or an UnknownPGN if it fails to decode the packet).

### Subscribe 

Subscribe is a separate package that manages subscribers and distributes go structs (in this case n2k-related) to them.




## Build instructions

To be provided

## Version History

See [changelog](./changelog.md)

## Related Projects

* [canboat](https://github.com/canboat/canboat) An open source collection of command line tools and data relevant to NMEA 2000 boat networks
* tbd

## License
[n2k license](./LICENSE)



