package n2k

import (
	"fmt"
	"reflect"
	"strings"
)

func DebugDumpPGN(p interface{}) string {
	tp := reflect.TypeOf(p)
	return tp.Name() + ": " + strings.Join(dumpFields(p), ", ")
}

func dumpFields(p interface{}) []string {
	vp := reflect.ValueOf(p)
	tp := reflect.TypeOf(p)

	fieldStrs := make([]string, 0)
	for i := 0; i < tp.NumField(); i++ {
		tf := tp.Field(i)
		vf := vp.Field(i)
		if tf.Name == "Info" && tf.Type.Kind() == reflect.Struct {
			fieldStrs = append(fieldStrs, dumpFields(vf.Interface())...)
			continue
		}
		switch tf.Name {
		case "Timestamp":
			// skip
		case "PGN":
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%#v(%d)", tf.Name, vf.Interface(), vf.Uint()))
		case "SourceId", "IndustryId":
			fieldStrs = append(fieldStrs, fmt.Sprintf("%s=%#v(%d)", tf.Name, vf.Interface(), vf.Uint()))
		default:
			if strings.Contains(tf.Name, "Repeating") {
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
					if vf.Pointer() == 0 {
						vStr = "nil"
					} else {
						vfi := vf.Elem()
						vStr = fmt.Sprintf("%#v", vfi.Interface())
					}
				case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Slice, reflect.Interface:
					vStr = fmt.Sprintf("%#v", vf.Interface())
				default:
					vStr = fmt.Sprintf("Unhandled PGN field type: %d, %#v", tf.Type.Kind(), vf.Interface())
				}
				fieldStrs = append(fieldStrs, tf.Name+"="+vStr)
			}
		}
	}

	return fieldStrs
}
