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

// A use case check cited by its number exists under the use case's How to check.
func TestUseCaseCheckReferences(t *testing.T) {
	known := map[string]string{"UC-ID-12": "x.md"}
	checks := map[string]ucChecks{"UC-ID-12": {checks: map[int]bool{1: true, 4: true}}}
	// Assembled, so that this file does not cite the missing checks itself.
	slash, word := "UC-ID-12/"+"9", "UC-ID-12 "+"check 7"
	root := writeTree(t, map[string]string{
		"a.go":   "// (UC-ID-12/4, " + slash + ")\n",
		"b.ts":   "// UC-ID-12: check 1 holds, " + word + " too\n",
		"c.json": `{"description": "UC-ID-12 (4)"}`,
	})
	got := strings.Join(checkUseCaseReferences(root, known, checks), "\n")
	for _, want := range []string{"UC-ID-12 check 9 is cited in a.go", "UC-ID-12 check 7 is cited in b.ts"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "check 4") || strings.Contains(got, "check 1 ") || strings.Count(got, "\n") != 1 {
		t.Errorf("a check that exists was reported:\n%s", got)
	}
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
