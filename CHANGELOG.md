## Change Log for open-ships/n2k

### 2026-03-31
Major housekeeping to simplify the repo and move fully under the open-ships org.

- Removed mage build system; replaced Makefile with a justfile using plain go commands
- Removed lintroller and its configuration
- Removed renovate, package.json, package-lock.json, and .releaserc.yaml
- Replaced semantic-release with date-based tagging (matching beacon's release process)
- Simplified CI: single test workflow using `go test`, release triggers on successful test run
- Removed `cmd/convertcandumps` and `cmd/replay` (and `pkg/endpoint/n2kfileendpoint`)
- Inlined `tugboat/pkg/canbus` and `tugboat/pkg/units` into this repo, removing the tugboat dependency
- Replaced `sirupsen/logrus` with `log/slog` throughout
- Upgraded to Go 1.25
- Changed module path from `github.com/boatkit-io/n2k` to `github.com/open-ships/n2k`
- Removed `.tool-versions`

## (Prior) Change Log for boatkit-io/n2k

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




