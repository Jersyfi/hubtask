// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const advisory = `
id: HUBTASK-2026-0001
title: A shared view leaks a hidden field
kind: advisory
cve: CVE-2026-12345
initial_release: 2026-11-02T10:00:00Z
summary: A saved view's export carried a field hidden from its reader.
cvss_v3: { vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:L/I:N/A:N", score: 4.3, severity: MEDIUM }
affected: ["1.0.0", "1.0.1"]
fixed: ["1.0.2"]
remediation: Upgrade to 1.0.2.
references: ["https://github.com/Jersyfi/hubtask/security/advisories/GHSA-xxxx"]
`

const vex = `
id: HUBTASK-2026-0002
title: Not affected by a flaw in a dependency
kind: vex
cve: CVE-2026-23456
initial_release: 2026-11-03T08:00:00Z
summary: The vulnerable function of the dependency is never called.
component: example.org/lib@v1.2.3
not_affected: ["1.0.2"]
justification: vulnerable_code_not_in_execute_path
`

func dirWith(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// security.md §14.2: an advisory and a VEX statement become CSAF 2.0 documents of their profiles,
// laid out with index.txt, changes.csv (newest first), hashes and the provider metadata.
func TestTheAdvisoriesBecomeACSAFDirectory(t *testing.T) {
	advisories, err := read(dirWith(t, map[string]string{"a.yaml": advisory, "b.yaml": vex}))
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := write(advisories, out, "https://example.test/.well-known/csaf"); err != nil {
		t.Fatal(err)
	}

	var document map[string]any
	raw, err := os.ReadFile(filepath.Join(out, "2026", "hubtask-2026-0001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	head := document["document"].(map[string]any)
	if head["category"] != "csaf_security_advisory" || head["csaf_version"] != "2.0" ||
		head["tracking"].(map[string]any)["id"] != "HUBTASK-2026-0001" {
		t.Fatalf("the document's head: %v", head)
	}
	status := document["vulnerabilities"].([]any)[0].(map[string]any)["product_status"].(map[string]any)
	if len(status["known_affected"].([]any)) != 2 || status["fixed"].([]any)[0] != "hubtask-1.0.2" {
		t.Fatalf("the product status: %v", status)
	}

	raw, _ = os.ReadFile(filepath.Join(out, "2026", "hubtask-2026-0002.json"))
	if !strings.Contains(string(raw), `"csaf_vex"`) || !strings.Contains(string(raw), "vulnerable_code_not_in_execute_path") ||
		!strings.Contains(string(raw), "known_not_affected") {
		t.Fatalf("the VEX statement: %s", raw)
	}
	index, _ := os.ReadFile(filepath.Join(out, "index.txt"))
	if string(index) != "2026/hubtask-2026-0001.json\n2026/hubtask-2026-0002.json\n" {
		t.Fatalf("index.txt: %q", index)
	}
	changes, _ := os.ReadFile(filepath.Join(out, "changes.csv"))
	if !strings.HasPrefix(string(changes), `"2026/hubtask-2026-0002.json","2026-11-03T08:00:00Z"`) {
		t.Fatalf("changes.csv is not newest first: %q", changes)
	}
	sum, _ := os.ReadFile(filepath.Join(out, "2026", "hubtask-2026-0001.json.sha256"))
	if !strings.HasSuffix(strings.TrimSpace(string(sum)), "  hubtask-2026-0001.json") {
		t.Fatalf("the hash file: %q", sum)
	}
	metadata, _ := os.ReadFile(filepath.Join(out, "provider-metadata.json"))
	if !strings.Contains(string(metadata), `"canonical_url": "https://example.test/.well-known/csaf/provider-metadata.json"`) ||
		!strings.Contains(string(metadata), `"last_updated": "2026-11-03T08:00:00Z"`) {
		t.Fatalf("the provider metadata: %s", metadata)
	}
}

// No advisory, no feed: an empty directory would claim a process that never ran.
func TestNoAdvisoryWritesNothing(t *testing.T) {
	if _, err := read(t.TempDir()); !errors.Is(err, errNoAdvisories) {
		t.Fatalf("an empty directory answered %v", err)
	}
}

// A malformed advisory stops the release rather than publishing half of it.
func TestAMalformedAdvisoryIsRefused(t *testing.T) {
	for name, body := range map[string]string{
		"id":            strings.Replace(advisory, "HUBTASK-2026-0001", "GHSA-1", 1),
		"no fix":        strings.Replace(advisory, `fixed: ["1.0.2"]`, "", 1),
		"version":       strings.Replace(advisory, `"1.0.0"`, `"latest"`, 1),
		"justification": strings.Replace(vex, "vulnerable_code_not_in_execute_path", "trust_us", 1),
	} {
		if _, err := read(dirWith(t, map[string]string{"a.yaml": body})); err == nil {
			t.Errorf("%s: a malformed advisory was read", name)
		}
	}
}

// An advisory whose identifier climbs out of the output directory is refused, and nothing is
// written beside it.
func TestAnAdvisoryWhosePathClimbsOutIsRefused(t *testing.T) {
	parent := t.TempDir()
	out := filepath.Join(parent, "csaf")
	advisories := []Advisory{{ID: "../../escaped", InitialRelease: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}}
	if err := write(advisories, out, "https://example.test/.well-known/csaf"); err == nil {
		t.Fatal("an advisory outside the directory was written")
	}
	if _, err := os.Stat(filepath.Join(parent, "escaped.json")); !os.IsNotExist(err) {
		t.Fatalf("a file was written outside the directory: %v", err)
	}
}
