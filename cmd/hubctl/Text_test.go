// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
)

// A document saved by an editor that writes a byte order mark, or by a Windows shell that writes
// UTF-16 when `hubctl config export > file` is redirected, is read as the JSON it holds.
func TestADocumentWithAByteOrderMarkIsRead(t *testing.T) {
	const document = `{"types":{"incident":{"level":"TASK","name":"Störung"}}}`
	utf16le := []byte{0xFF, 0xFE}
	for _, unit := range utf16.Encode([]rune(document)) {
		utf16le = append(utf16le, byte(unit), byte(unit>>8))
	}
	utf16be := []byte{0xFE, 0xFF}
	for _, unit := range utf16.Encode([]rune(document)) {
		utf16be = append(utf16be, byte(unit>>8), byte(unit))
	}
	for name, raw := range map[string][]byte{
		"UTF-8 with a mark": append([]byte{0xEF, 0xBB, 0xBF}, document...),
		"UTF-16 LE":         utf16le,
		"UTF-16 BE":         utf16be,
		"plain UTF-8":       []byte(document),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			read, err := readDocument(&CLI{Streams: Streams{In: bytes.NewReader(nil)}}, path)
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			types, _ := read["types"].(map[string]any)
			incident, _ := types["incident"].(map[string]any)
			if incident["name"] != "Störung" {
				t.Fatalf("read %v", read)
			}
			body, err := readBody(bytes.NewReader(raw), "-")
			if err != nil || body["types"] == nil {
				t.Fatalf("a --body from standard input: %v %v", body, err)
			}
		})
	}
}
