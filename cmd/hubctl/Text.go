// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// jsonText is a document a person hands in, as the UTF-8 JSON reads: without the byte order mark
// an editor writes, and decoded from the UTF-16 a Windows shell writes when `hubctl ... > file` is
// redirected. Without a mark the bytes are taken as they are.
func jsonText(raw []byte) []byte {
	switch {
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		return raw[3:]
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return fromUTF16(raw[2:], func(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 })
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return fromUTF16(raw[2:], func(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) })
	}
	return raw
}

func fromUTF16(raw []byte, unit func([]byte) uint16) []byte {
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		units = append(units, unit(raw[i:i+2]))
	}
	out := make([]byte, 0, len(units))
	for _, r := range utf16.Decode(units) {
		out = utf8.AppendRune(out, r)
	}
	return out
}
