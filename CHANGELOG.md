## Change Log for boatkit-io/n2k

This document will be updated whenever changes are made to the main branch of this project.

### 2022-12-01
Initial restructuring of the package as we move it from private to public.

### 2023-09-29
- Preliminary support for variants of PGN 126208 that include a reference PGN and repeating pairs of the index to a field in the reference PGN and its value. To determine the length of the value requires looking up the reference PGN, potentially its Manufacturer (to deal with variants of proprietary PGNs), and the length of the specified field's value.  It would be possible to use reflection and return the value in an appropriate golang type, but for now the values are returned in a []uint8.
- Preliminary support for Key\_Value fields. These also require accessing information to determine the type of the value, based on the key. For now these also return the value in a []uint8.
- Note that for now the support for PGN 126208 only matches the Manufacturer in selecting the appropriate variant. In fact fields can vary based on at matches.
- Note that for PGN 126208 with command code of 1 (NmeaCommandGroupFunction), if the commanded PGN is Proprietary there's no way to select the correct variant, since the ManufacturerCode is not specified. (This works because the intended recipient is implemented by a specific manufacturer). In this case if we find multiple variants we return an error.
- Canboat PGN definitions without samples (that is, no logs including the PGN/variant have been submitted to Canboat) are tracked separately--when they're found in logs the samples should be submitted to the project.
- Imports canboat.json from an interim version forked on Boatkit while we wait for the project to address issues related to values with lengths not known in the PGN definition.

### 2023-10-05
Release v0.0.1

- Switched back to Canboat's canboat.json.

### 2023-10-07
Release v0.0.2
Moved from Pipeline constructed from channels to a much simpler model connecting each stage
through function variables.

### 2024-06-15
release v0.1.0
Bug Fixes
- Add package.json for semantic-release (ff5efe1)
- Add release workflow (30adef9)
- Add releaserc (e26e1e5)
- deps: update module github.com/boatkit-io/tugboat to v0.6.0 (#23) (01866a0)
- deps: update module github.com/schollz/progressbar/v3 to v3.14.2 (#25) (978463b)
- deps: update module github.com/stretchr/testify to v1.9.0 (#28) (15e920e)
- deps: update module golang.org/x/text to v0.15.0 (#24) (811cc94)

Features
- Supporting USBCANEndpoint, updated deps and build scripts (#36) (621cf9f)

### 2024-06-16
release v0.2.0
Features
- Added (tugboat) units directly into PGN type generation (#37) (30aad39)

### 2024-06-17
release v0.2.1
Bug Fixes
- Fast Packet Assembly fixed to detect incomplete sequences

### 2024-06-21
release v0.3.0
Features
--added command to record nmea2k messages in the single frame RAW format, using a USBCan device.
