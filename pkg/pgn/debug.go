package pgn

import (
	"fmt"
	"reflect"
	"strings"
)

// DebugDumpPGN produces a human-readable, single-line string representation of a decoded
// PGN struct. It uses reflection to iterate over all fields, printing their names and values.
// This is intended for logging and diagnostic output -- not for serialization.
//
// The output format is: "StructName: Field1=value1, Field2=value2, ..."
// Embedded MessageInfo fields are flattened (not nested), and the Timestamp field is omitted
// for brevity since it is usually available from other context.
//
// Example output: "VesselHeading: PGN=pgn.VesselHeadingPgn(127250), SourceId=..., Heading=1.5"
func DebugDumpPGN(p any) string {
	tp := reflect.TypeOf(p)
	return tp.Name() + ": " + strings.Join(dumpFields(p), ", ")
}

// dumpFields recursively extracts field name=value pairs from a struct using reflection.
// It handles several special cases that arise from the PGN struct conventions:
//   - "Info" (MessageInfo): recursed into and flattened so its fields appear at the top level
//   - "Timestamp": skipped entirely (too noisy for debug output)
//   - "PGN", "SourceId", "IndustryId": printed with both the Go representation and the
//     numeric value, because these are typed constants whose %#v includes the type name
//   - Fields containing "Repeating": treated as slices of sub-structs and recursively dumped
//   - Pointer fields: printed as "nil" when nil, or dereferenced and printed otherwise
//   - All other types: printed with %#v for unambiguous Go-syntax output
func dumpFields(p any) []string {
	vp := reflect.ValueOf(p)
	tp := reflect.TypeOf(p)

	fieldStrs := make([]string, 0)
	for i := 0; i < tp.NumField(); i++ {
		tf := tp.Field(i)
		vf := vp.Field(i)
		// Flatten the embedded MessageInfo struct so its fields appear alongside the PGN fields.
		if tf.Name == "Info" && tf.Type.Kind() == reflect.Struct {
			fieldStrs = append(fieldStrs, dumpFields(vf.Interface())...)
			continue
		}
		switch tf.Name {
		case "Timestamp":
			// Omitted from debug output -- timestamps add noise and are available elsewhere.
		case "PGN":
			// Show both the typed constant name and the raw numeric value for easy identification.
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%#v(%d)", tf.Name, vf.Interface(), vf.Uint()))
		case "SourceId", "IndustryId":
			// Same treatment as PGN: show the typed constant and the underlying integer.
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%#v(%d)", tf.Name, vf.Interface(), vf.Uint()))
		default:
			if strings.Contains(tf.Name, "Repeating") {
				// Repeating field groups are slices of sub-structs. Recursively dump
				// each element, wrapping in braces and joining with commas.
				strI := make([]string, 0)
				for i := 0; i < vf.Len(); i++ {
					strI = append(strI, "{"+strings.Join(dumpFields(vf.Index(i).Interface()), ", ")+"}")
				}
				fieldStrs = append(fieldStrs, fmt.Sprintf("%s: [%s]", tf.Name, strings.Join(strI, ", ")))
			} else {
				vStr := ""
				switch tf.Type.Kind() {
				case reflect.String:
					vStr = vf.String()
				case reflect.Pointer:
					// Nullable fields in PGN structs are represented as pointers.
					// Show "nil" for absent data, or the dereferenced value otherwise.
					if vf.Pointer() == 0 {
						vStr = "nil"
					} else {
						vfi := vf.Elem()
						vStr = fmt.Sprintf("%#v", vfi.Interface())
					}
				case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Slice, reflect.Interface:
					vStr = fmt.Sprintf("%#v", vf.Interface())
				case reflect.Bool:
					vStr = fmt.Sprintf("%t", vf.Interface())
				default:
					// Safety net for types not yet handled -- makes it obvious in output.
					vStr = fmt.Sprintf("Unhandled PGN field type: %d, %#v", tf.Type.Kind(), vf.Interface())
				}
				fieldStrs = append(fieldStrs, tf.Name+"="+vStr)
			}
		}
	}

	return fieldStrs
}
