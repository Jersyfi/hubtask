// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package usecase

import "reflect"

// OutputMismatch checks an answer against the declared Output and names the first member that
// misses it: a required member absent or null, or a present member of another kind. Members the
// declaration does not name are allowed - it is what a client may rely on, not all there is.
//
// An MCP client refuses structured content that misses the tool's outputSchema, so a declaration
// is made only where a test holds the use case's real answers to it (ai-first.md §1.1).
func (d Descriptor) OutputMismatch(out Output) (string, bool) {
	for _, field := range d.Output {
		value, present := out[field.Name]
		if !present || value == nil {
			if field.Required {
				return field.Name, true
			}
			continue
		}
		if !ofKind(field.Kind, value) {
			return field.Name, true
		}
	}
	return "", false
}

// ofKind reports whether a value of an answer is of a declared kind. Reflection rather than a
// list of types, because an answer's lists are built of whichever element type the use case had.
func ofKind(kind Kind, value any) bool {
	v := reflect.ValueOf(value)
	switch kind {
	case KindString, KindID:
		_, ok := value.(string)
		return ok
	case KindBool:
		_, ok := value.(bool)
		return ok
	case KindInt:
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return true
		}
		return false
	case KindObject:
		return v.Kind() == reflect.Map
	case KindList, KindIDList:
		return v.Kind() == reflect.Slice
	default:
		return true
	}
}
