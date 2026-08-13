// Copyright (C) 2026 Boatkit
//
// This work is licensed under the terms of the MIT license. For a copy,
// see <https://opensource.org/licenses/MIT>.
//
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
)

const pgnOverridesPath = "cmd/pgngen/pgn_overrides.json"

type pgnOverrides struct {
	PGNs       []*PGN
	MinLengths map[string]uint32
}

func (conv *canboatConverter) applyPGNOverrides() error {
	raw, err := os.ReadFile(pgnOverridesPath)
	if err != nil {
		return fmt.Errorf("read PGN overrides: %w", err)
	}

	var overrides pgnOverrides
	if err := json.Unmarshal(raw, &overrides); err != nil {
		return fmt.Errorf("parse PGN overrides: %w", err)
	}

	replaced, added, err := conv.mergePGNOverrides(overrides.PGNs)
	if err != nil {
		return err
	}
	minLengths, err := conv.applyPGNMinLengthOverrides(overrides.MinLengths)
	if err != nil {
		return err
	}
	log.Infof("Applied local PGN overrides: %d replaced, %d added, %d minimum lengths", replaced, added, minLengths)
	return nil
}

func (conv *canboatConverter) applyPGNMinLengthOverrides(overrides map[string]uint32) (int, error) {
	baseByID := make(map[string]*PGN, len(conv.PGNs))
	for _, definition := range conv.PGNs {
		baseByID[definition.Id] = definition
	}

	for id, minLength := range overrides {
		definition, exists := baseByID[id]
		if !exists {
			return 0, fmt.Errorf("minimum length override references unknown PGN ID %q", id)
		}
		if minLength == 0 {
			return 0, fmt.Errorf("minimum length override for %q must be greater than zero", id)
		}
		if definition.MinLength != 0 {
			if definition.MinLength == minLength {
				return 0, fmt.Errorf("minimum length override for %q is identical to the upstream definition; remove the local override", id)
			}
			return 0, fmt.Errorf("minimum length override for %q conflicts with upstream minimum length %d", id, definition.MinLength)
		}
		if definition.Length == 0 || minLength >= definition.Length {
			return 0, fmt.Errorf("minimum length override for %q must be shorter than its fixed length %d", id, definition.Length)
		}

		minimumBits := minLength * 8
		hasOptionalFieldBoundary := false
		for fieldIndex := range definition.Fields {
			if uint32(definition.Fields[fieldIndex].BitOffset) == minimumBits {
				hasOptionalFieldBoundary = true
				break
			}
		}
		if !hasOptionalFieldBoundary {
			return 0, fmt.Errorf("minimum length override for %q (%d bytes) does not end on a field boundary", id, minLength)
		}

		definition.MinLength = minLength
	}

	return len(overrides), nil
}

func (conv *canboatConverter) mergePGNOverrides(overrides []*PGN) (replaced, added int, err error) {
	baseByID := make(map[string]int, len(conv.PGNs))
	for index, definition := range conv.PGNs {
		baseByID[definition.Id] = index
	}

	seenOverrides := make(map[string]struct{}, len(overrides))
	for _, override := range overrides {
		if err := validatePGNOverride(override); err != nil {
			return 0, 0, err
		}
		if _, exists := seenOverrides[override.Id]; exists {
			return 0, 0, fmt.Errorf("duplicate PGN override ID %q", override.Id)
		}
		seenOverrides[override.Id] = struct{}{}

		if index, exists := baseByID[override.Id]; exists {
			upstream := conv.PGNs[index]
			if upstream.PGN != override.PGN {
				return 0, 0, fmt.Errorf(
					"PGN override %q changes PGN number from %d to %d",
					override.Id,
					upstream.PGN,
					override.PGN,
				)
			}
			if reflect.DeepEqual(upstream, override) {
				return 0, 0, fmt.Errorf(
					"PGN override %q is identical to the upstream definition; remove the local override",
					override.Id,
				)
			}
			conv.PGNs[index] = override
			replaced++
			continue
		}

		overrideSignature := pgnMatchSignature(override)
		for _, upstream := range conv.PGNs {
			if upstream.PGN == override.PGN && pgnMatchSignature(upstream) == overrideSignature {
				return 0, 0, fmt.Errorf(
					"PGN override %q conflicts with upstream definition %q for PGN %d and match fields %s",
					override.Id,
					upstream.Id,
					override.PGN,
					overrideSignature,
				)
			}
		}

		conv.PGNs = append(conv.PGNs, override)
		baseByID[override.Id] = len(conv.PGNs) - 1
		added++
	}

	return replaced, added, nil
}

func validatePGNOverride(override *PGN) error {
	switch {
	case override == nil:
		return fmt.Errorf("PGN override must not be null")
	case override.Id == "":
		return fmt.Errorf("PGN override must have an ID")
	case override.PGN == 0:
		return fmt.Errorf("PGN override %q must have a PGN number", override.Id)
	case override.Type == "":
		return fmt.Errorf("PGN override %q must have a transmission type", override.Id)
	case override.Length == 0:
		return fmt.Errorf("PGN override %q must have a length", override.Id)
	case len(override.Fields) == 0:
		return fmt.Errorf("PGN override %q must define fields", override.Id)
	default:
		return nil
	}
}

func pgnMatchSignature(definition *PGN) string {
	fields := make([]string, 0)
	for index := range definition.Fields {
		field := &definition.Fields[index]
		if field.Match == nil {
			continue
		}
		fields = append(fields, fmt.Sprintf("%d:%d:%d", field.BitOffset, field.BitLength, *field.Match))
	}
	sort.Strings(fields)
	return strings.Join(fields, ",")
}
