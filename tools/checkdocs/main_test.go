// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// An ADR number that resolves to nothing is found in every kind of file the code is written in.
func TestADRReferencesInEveryFileType(t *testing.T) {
	// Assembled, so that this file does not cite the missing number itself.
	missing := "ADR-" + "0002"
	files := map[string]string{"docs/adr/ADR-0001-x.md": "# ADR-0001\n"}
	for _, name := range []string{"a.go", "b.ts", "c.svelte", "d.mjs", "e.json", "f.py", "g.html", "h.css", "i.sh", "j.md", "Makefile"} {
		files["src/"+name] = "see " + missing + "\n"
	}
	files["src/ok.ts"] = "see ADR-0001\n"
	files["node_modules/x/k.ts"] = "see " + missing + "\n"

	got := strings.Join(checkADRReferences(writeTree(t, files)), "\n")
	for _, name := range []string{"a.go", "b.ts", "c.svelte", "d.mjs", "e.json", "f.py", "g.html", "h.css", "i.sh", "j.md", "Makefile"} {
		if !strings.Contains(got, "src/"+name) {
			t.Errorf("the citation in %s was not found:\n%s", name, got)
		}
	}
	if strings.Contains(got, "node_modules") || strings.Contains(got, "ok.ts") {
		t.Errorf("a citation that resolves, or one under node_modules, was reported:\n%s", got)
	}
}
