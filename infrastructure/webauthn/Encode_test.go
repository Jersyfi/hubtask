// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"encoding/binary"
	"sort"
)

// A CBOR encoder for the tests' own authenticators - deliberately not in the adapter, which only ever
// reads. Maps are written in the order given; a test that needs a duplicate key writes raw bytes.

type cborEntry struct {
	key   any
	value any
}

// cborMapOf keeps the entries in order.
type cborMapOf []cborEntry

func encodeCBOR(v any) []byte {
	switch x := v.(type) {
	case int:
		return encodeInt(int64(x))
	case int64:
		return encodeInt(x)
	case []byte:
		return append(head(2, uint64(len(x))), x...)
	case string:
		return append(head(3, uint64(len(x))), x...)
	case bool:
		if x {
			return []byte{0xf5}
		}
		return []byte{0xf4}
	case nil:
		return []byte{0xf6}
	case []any:
		out := head(4, uint64(len(x)))
		for _, item := range x {
			out = append(out, encodeCBOR(item)...)
		}
		return out
	case cborMapOf:
		out := head(5, uint64(len(x)))
		for _, entry := range x {
			out = append(out, encodeCBOR(entry.key)...)
			out = append(out, encodeCBOR(entry.value)...)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		m := cborMapOf{}
		for _, k := range keys {
			m = append(m, cborEntry{k, x[k]})
		}
		return encodeCBOR(m)
	default:
		panic("encodeCBOR: unsupported value")
	}
}

func encodeInt(n int64) []byte {
	if n >= 0 {
		return head(0, uint64(n))
	}
	return head(1, uint64(-1-n))
}

func head(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n <= 0xff:
		return []byte{major<<5 | 24, byte(n)}
	case n <= 0xffff:
		b := []byte{major<<5 | 25, 0, 0}
		binary.BigEndian.PutUint16(b[1:], uint16(n))
		return b
	case n <= 0xffffffff:
		b := []byte{major<<5 | 26, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(b[1:], uint32(n))
		return b
	default:
		b := []byte{major<<5 | 27, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(b[1:], n)
		return b
	}
}
