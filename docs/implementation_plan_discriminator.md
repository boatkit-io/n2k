# Implementation Plan: PGN Discriminator and Client/Runtime Separation

## Overview

This document outlines the implementation plan for two major improvements to the n2k codebase:

1. **PGN Discriminator System**: Efficient decoder selection for PGNs with multiple variants using match values
2. **Client/Runtime Separation**: Split generated code into client API and runtime components

## Problem Statement

### Current Issues
- Multiple decoder attempts for PGNs with variants cause performance overhead
- Match values exist in field definitions but are only used during decoding validation
- All generated code is in a single file, mixing client API and runtime concerns
- FieldDescriptors are generated but only FieldSpecs are used at runtime

### Goals
- Eliminate multiple decoder attempts through efficient pre-filtering
- Leverage match values for variant discrimination at generation time
- Separate client API from runtime implementation
- Optimize field reading using existing FieldSpec infrastructure

## 1. PGN Discriminator System

### 1.1 Architecture

The discriminator system will be generated at build time and provide efficient variant selection:

```
Packet Data → Discriminator → Selected Decoder → Parsed Struct
```

### 1.2 Generated Components

#### Discriminator Types
```go
// Generated discriminator for PGNs with multiple variants
type PgnDiscriminator struct {
    PgnInfo    *PgnInfo
    MatchSpecs []MatchFieldSpec
}

type MatchFieldSpec struct {
    FieldSpec  *FieldSpec
    MatchValue int
}
```

#### Discriminator Functions
```go
// Generated for each PGN with multiple variants
func DiscriminatePgn{PGN_ID}(data []uint8) (*PgnInfo, error)
func matchesVariant(stream *DataStream, matchSpecs []MatchFieldSpec) bool
```

### 1.3 Integration Points

#### Packet Struct Updates
```go
type Packet struct {
    // ... existing fields ...
    
    // Discriminator function for PGNs with multiple variants
    Discriminator func([]uint8) (*pgn.PgnInfo, error)
}
```

#### Enhanced AddDecoders Method
```go
func (p *Packet) AddDecoders() {
    p.GetManCode()
    
    if p.Discriminator != nil {
        // Use discriminator for PGNs with multiple variants
        if selectedVariant, err := p.Discriminator(p.Data); err == nil {
            p.Decoders = []func(pgn.MessageInfo, *pgn.DataStream) (any, error){selectedVariant.Decoder}
            return
        }
    }
    
    // Fallback to existing logic
    for _, d := range p.Candidates {
        if p.Proprietary && p.Manufacturer != d.ManId {
            continue
        }
        p.Decoders = append(p.Decoders, d.Decoder)
    }
}
```

### 1.4 Template Generation

#### Template Helper Functions
```go
// Check if PGN has multiple variants
func hasMultipleVariants(pgn PGN) bool {
    return len(PgnInfoLookup[pgn.PGN]) > 1
}

// Get all variants for a PGN
func getVariantsForPgn(pgn PGN) []PgnInfo {
    return PgnInfoLookup[pgn.PGN]
}
```

#### Discriminator Generation Template
```go
{{- range $pgn := .PGNDoc.PGNs }}
{{- if hasMultipleVariants $pgn }}
// Generated discriminator for {{ $pgn.Id }}
var pgn{{ $pgn.Id }}Discriminators = []PgnDiscriminator{
{{- range $variant := getVariantsForPgn $pgn }}
    {
        PgnInfo: &pgnList[{{ $variant.Index }}],
        MatchSpecs: []MatchFieldSpec{
{{- range $field := $variant.Fields }}
{{- if $field.Match }}
            {
                FieldSpec: pgnList[{{ $variant.Index }}].FieldSpecs["{{ $field.Id }}"],
                MatchValue: {{ $field.Match }},
            },
{{- end }}
{{- end }}
        },
    },
{{- end }}
}

func Discriminate{{ $pgn.Id }}(data []uint8) (*PgnInfo, error) {
    stream := NewDataStream(data)
    
    for _, discriminator := range pgn{{ $pgn.Id }}Discriminators {
        if matchesVariant(stream, discriminator.MatchSpecs) {
            return discriminator.PgnInfo, nil
        }
    }
    
    return nil, fmt.Errorf("no matching variant found for PGN {{ $pgn.PGN }}")
}
{{- end }}
{{- end }}
```

## 2. Client/Runtime Separation

### 2.1 File Structure

```
pkg/pgn/
├── client/                    # Client API (generated)
│   ├── types.go              # PGN struct definitions
│   ├── enums.go              # Enum types and constants
│   ├── subscribe.go          # Subscription interfaces
│   └── write.go              # Write interfaces
├── runtime/                  # Runtime implementation (generated)
│   ├── pgninfo.go           # PgnInfo, FieldSpecs
│   ├── decoders.go          # Decoder functions
│   ├── encoders.go          # Encoder functions
│   ├── discriminators.go    # Discriminator functions
│   └── fieldspecs.go        # FieldSpec definitions
├── datastream.go            # Core DataStream (hand-written)
├── messageinfo.go           # MessageInfo (hand-written)
└── pgninfo.go              # Lookup maps and utilities (hand-written)
```

### 2.2 Client API Components

#### types.go
- PGN struct definitions
- Repeating field structs
- Partial structs for PGN 126208

#### enums.go
- All enum types and constants
- String methods for enums
- Lookup maps

#### subscribe.go
- Subscription interfaces
- Event handling types

#### write.go
- Write interfaces
- Encoding helper functions

### 2.3 Runtime Components

#### pgninfo.go
- PgnInfo struct definitions
- PgnInfoLookup maps
- Utility functions

#### decoders.go
- All decoder functions
- Decode{PGN_ID} functions

#### encoders.go
- All encoder functions
- Encode methods for structs

#### discriminators.go
- Discriminator functions
- Match field specifications
- Variant matching logic

#### fieldspecs.go
- FieldSpec definitions
- FieldSpec generation utilities

### 2.4 Template Modifications

#### Split Template Files
```
cmd/pgngen/templates/
├── client/
│   ├── types.go.tmpl
│   ├── enums.go.tmpl
│   ├── subscribe.go.tmpl
│   └── write.go.tmpl
└── runtime/
    ├── pgninfo.go.tmpl
    ├── decoders.go.tmpl
    ├── encoders.go.tmpl
    ├── discriminators.go.tmpl
    └── fieldspecs.go.tmpl
```

#### Build Process Updates
```go
// cmd/pgngen/main.go
func generateClientAPI(pgnDoc *PGNDocument) error {
    // Generate client API files
    templates := []string{
        "client/types.go.tmpl",
        "client/enums.go.tmpl", 
        "client/subscribe.go.tmpl",
        "client/write.go.tmpl",
    }
    
    for _, template := range templates {
        if err := generateFile(template, pgnDoc); err != nil {
            return err
        }
    }
    return nil
}

func generateRuntime(pgnDoc *PGNDocument) error {
    // Generate runtime files
    templates := []string{
        "runtime/pgninfo.go.tmpl",
        "runtime/decoders.go.tmpl",
        "runtime/encoders.go.tmpl", 
        "runtime/discriminators.go.tmpl",
        "runtime/fieldspecs.go.tmpl",
    }
    
    for _, template := range templates {
        if err := generateFile(template, pgnDoc); err != nil {
            return err
        }
    }
    return nil
}
```

## 3. Implementation Phases

### Phase 1: Foundation
- [ ] Create new file structure
- [ ] Split existing template into client/runtime templates
- [ ] Update build process to generate separate files
- [ ] Verify existing functionality still works

### Phase 2: Discriminator System
- [ ] Add discriminator types to runtime
- [ ] Implement discriminator generation template
- [ ] Add discriminator lookup to PgnInfoLookup
- [ ] Update Packet struct with discriminator field

### Phase 3: Integration
- [ ] Update AddDecoders to use discriminators
- [ ] Add fallback logic for discriminator failures
- [ ] Update NewPacket to set discriminator

### Phase 4: Testing and Validation
- [ ] Create tests for match value uniqueness
- [ ] Add discriminator performance tests
- [ ] Verify all PGN variants work correctly
- [ ] Benchmark performance improvements

### Phase 5: Cleanup
- [ ] Remove unused FieldDescriptor generation
- [ ] Clean up old template code
- [ ] Update documentation
- [ ] Remove deprecated functions

## 4. Testing Strategy

### 4.1 Match Value Uniqueness Test
```go
func TestMatchValueUniqueness(t *testing.T) {
    for pgn, variants := range pgn.PgnInfoLookup {
        if len(variants) <= 1 {
            continue
        }
        
        signatures := make(map[string]bool)
        for _, variant := range variants {
            signature := buildMatchSignature(variant)
            if signatures[signature] {
                t.Errorf("Duplicate match signature for PGN %d: %s", pgn, signature)
            }
            signatures[signature] = true
        }
    }
}
```

### 4.2 Discriminator Performance Test
```go
func BenchmarkDiscriminator(b *testing.B) {
    // Test discriminator performance vs multiple decoder attempts
    for pgn, variants := range pgn.PgnInfoLookup {
        if len(variants) <= 1 {
            continue
        }
        
        discriminator := pgn.GetDiscriminator(pgn)
        if discriminator == nil {
            continue
        }
        
        // Benchmark discriminator vs traditional approach
        b.Run(fmt.Sprintf("PGN_%d_Discriminator", pgn), func(b *testing.B) {
            // Test discriminator performance
        })
        
        b.Run(fmt.Sprintf("PGN_%d_Traditional", pgn), func(b *testing.B) {
            // Test traditional multiple decoder approach
        })
    }
}
```

## 5. Migration Strategy

### 5.1 Backward Compatibility
- Maintain existing API during transition
- Gradual migration of internal components
- Deprecation warnings for old patterns

### 5.2 Rollout Plan
1. Implement discriminator system alongside existing code
2. Add feature flags to enable/disable discriminators
3. Gradual rollout with performance monitoring
4. Remove old code after validation

## 6. Success Metrics

### 6.1 Performance Improvements
- Reduction in decoder attempts for multi-variant PGNs
- Faster packet processing overall
- Lower CPU usage during high-throughput scenarios

### 6.2 Code Quality
- Cleaner separation of concerns
- Reduced generated code size
- Better maintainability

### 6.3 Developer Experience
- Clearer API boundaries
- Better error messages
- Improved debugging capabilities

## 7. Risk Mitigation

### 7.1 Technical Risks
- **Discriminator failures**: Fallback to existing logic
- **Template complexity**: Incremental implementation
- **Performance regression**: Comprehensive benchmarking

### 7.2 Mitigation Strategies
- Extensive testing with real N2K data
- Gradual rollout with monitoring
- Rollback plan if issues arise
- Performance regression testing

## 8. Future Enhancements

### 8.1 Advanced Discrimination
- Field ordering optimization
- Early termination strategies
- Caching of discriminator results

### 8.2 Client API Improvements
- Type-safe subscription interfaces
- Builder patterns for complex messages
- Validation helpers

### 8.3 Runtime Optimizations
- JIT compilation of discriminators
- Memory pool for DataStream objects
- Parallel processing support

---

This implementation plan provides a comprehensive roadmap for implementing both the discriminator system and client/runtime separation. The phased approach ensures minimal risk while delivering significant performance improvements and better code organization.
