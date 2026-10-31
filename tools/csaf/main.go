// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command csaf writes the project's machine-readable advisories (security.md §14.2): one CSAF 2.0
// document per advisory in `security/advisories/` - profile *security advisory* for a flaw of
// Hubtask's, *VEX* for a dependency's flaw Hubtask does not reach - with the directory's
// `index.txt`, `changes.csv`, a SHA-256 and SHA-512 file beside each document, and the provider
// metadata. With no advisory it writes nothing: an empty feed would claim a publishing process that
// has never run. The release attaches the result and signs it; the website publishes it under
// `/.well-known/csaf/`.
//
// Usage: csaf <advisories dir> <out dir> [base url]
package main

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Advisory is one file of `security/advisories/`, the project's own short form of a CSAF document.
type Advisory struct {
	ID             string    `yaml:"id"`
	Title          string    `yaml:"title"`
	Kind           string    `yaml:"kind"` // advisory or vex
	CVE            string    `yaml:"cve"`
	InitialRelease time.Time `yaml:"initial_release"`
	CurrentRelease time.Time `yaml:"current_release"`
	Revisions      []struct {
		Date    time.Time `yaml:"date"`
		Summary string    `yaml:"summary"`
	} `yaml:"revisions"`
	Summary string `yaml:"summary"`
	CVSS    *struct {
		Vector   string  `yaml:"vector"`
		Score    float64 `yaml:"score"`
		Severity string  `yaml:"severity"`
	} `yaml:"cvss_v3"`
	Affected      []string `yaml:"affected"`
	Fixed         []string `yaml:"fixed"`
	NotAffected   []string `yaml:"not_affected"`
	Justification string   `yaml:"justification"`
	Component     string   `yaml:"component"`
	Remediation   string   `yaml:"remediation"`
	References    []string `yaml:"references"`
}

var (
	idPattern       = regexp.MustCompile(`^HUBTASK-[0-9]{4}-[0-9]{4}$`)
	versionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)
	justifications  = []string{"component_not_present", "vulnerable_code_not_present", "vulnerable_code_not_in_execute_path", "vulnerable_code_cannot_be_controlled_by_adversary", "inline_mitigations_already_exist"}
	errNoAdvisories = errors.New("no advisories")
)

func main() {
	if len(os.Args) < 3 || len(os.Args) > 4 {
		fmt.Fprintln(os.Stderr, "usage: csaf <advisories dir> <out dir> [base url]")
		os.Exit(2)
	}
	base := "https://hubtask.eu/.well-known/csaf"
	if len(os.Args) == 4 {
		base = strings.TrimRight(os.Args[3], "/")
	}
	advisories, err := read(os.Args[1])
	if errors.Is(err, errNoAdvisories) {
		fmt.Println("csaf: no advisories, nothing written")
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "csaf:", err)
		os.Exit(1)
	}
	if err := write(advisories, os.Args[2], base); err != nil {
		fmt.Fprintln(os.Stderr, "csaf:", err)
		os.Exit(1)
	}
	fmt.Printf("csaf: %d documents -> %s\n", len(advisories), os.Args[2])
}

// read loads and checks every advisory of the directory, in identifier order.
func read(dir string) ([]Advisory, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, errNoAdvisories
	}
	var out []Advisory
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // G304: the directory the Makefile names
		if err != nil {
			return nil, err
		}
		var advisory Advisory
		if err := yaml.Unmarshal(raw, &advisory); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		if err := advisory.check(); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		out = append(out, advisory)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (a Advisory) check() error {
	switch {
	case !idPattern.MatchString(a.ID):
		return fmt.Errorf("id %q is not HUBTASK-YYYY-NNNN", a.ID)
	case strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Summary) == "":
		return errors.New("a title and a summary are required")
	case a.InitialRelease.IsZero():
		return errors.New("initial_release is required")
	case a.Kind != "advisory" && a.Kind != "vex":
		return fmt.Errorf("kind %q is neither advisory nor vex", a.Kind)
	case a.Kind == "advisory" && (len(a.Affected) == 0 || len(a.Fixed) == 0):
		return errors.New("an advisory names the affected and the fixed versions")
	case a.Kind == "vex" && (len(a.NotAffected) == 0 || a.CVE == "" || a.Component == ""):
		return errors.New("a VEX statement names the CVE, the component and the versions not affected")
	case a.Kind == "vex" && !slices.Contains(justifications, a.Justification):
		return fmt.Errorf("justification %q is not one of CSAF's", a.Justification)
	}
	for _, version := range slices.Concat(a.Affected, a.Fixed, a.NotAffected) {
		if !versionPattern.MatchString(version) {
			return fmt.Errorf("version %q is not X.Y.Z", version)
		}
	}
	return nil
}

func (a Advisory) current() time.Time {
	if a.CurrentRelease.IsZero() {
		return a.InitialRelease
	}
	return a.CurrentRelease
}

// Path is the document's place in the directory: the year of its first release and its lower-case
// identifier, as CSAF 2.0 §7.1.11 asks.
func (a Advisory) Path() string {
	return fmt.Sprintf("%d/%s.json", a.InitialRelease.UTC().Year(), strings.ToLower(a.ID))
}

func productID(version string) string { return "hubtask-" + version }

// Document is the advisory as CSAF 2.0 spells it.
func (a Advisory) Document() map[string]any {
	category := "csaf_security_advisory"
	if a.Kind == "vex" {
		category = "csaf_vex"
	}
	history := []any{}
	for i, revision := range a.Revisions {
		history = append(history, map[string]any{
			"date": stamp(revision.Date), "number": fmt.Sprint(i + 1), "summary": revision.Summary,
		})
	}
	if len(history) == 0 {
		history = append(history, map[string]any{"date": stamp(a.InitialRelease), "number": "1", "summary": "Initial version."})
	}
	versions := slices.Concat(a.Affected, a.Fixed, a.NotAffected)
	slices.Sort(versions)
	versions = slices.Compact(versions)
	branches := []any{}
	for _, version := range versions {
		branches = append(branches, map[string]any{
			"category": "product_version", "name": version,
			"product": map[string]any{"name": "Hubtask " + version, "product_id": productID(version)},
		})
	}
	ids := func(versions []string) []string {
		out := make([]string, 0, len(versions))
		for _, version := range versions {
			out = append(out, productID(version))
		}
		return out
	}
	status := map[string]any{}
	if len(a.Affected) > 0 {
		status["known_affected"] = ids(a.Affected)
	}
	if len(a.Fixed) > 0 {
		status["fixed"] = ids(a.Fixed)
	}
	if len(a.NotAffected) > 0 {
		status["known_not_affected"] = ids(a.NotAffected)
	}
	vulnerability := map[string]any{
		"notes":          []any{map[string]any{"category": "description", "text": a.Summary}},
		"product_status": status,
	}
	if a.CVE != "" {
		vulnerability["cve"] = a.CVE
	}
	if a.Kind == "vex" {
		vulnerability["flags"] = []any{map[string]any{"label": a.Justification, "product_ids": ids(a.NotAffected)}}
		notes, _ := vulnerability["notes"].([]any)
		vulnerability["notes"] = append(notes,
			map[string]any{"category": "other", "title": "Component", "text": a.Component})
	}
	if a.Remediation != "" && len(a.Affected) > 0 {
		vulnerability["remediations"] = []any{map[string]any{"category": "vendor_fix", "details": a.Remediation, "product_ids": ids(a.Affected)}}
	}
	if a.CVSS != nil && len(a.Affected) > 0 {
		vulnerability["scores"] = []any{map[string]any{
			"products": ids(a.Affected),
			"cvss_v3": map[string]any{
				"version": "3.1", "vectorString": a.CVSS.Vector, "baseScore": a.CVSS.Score, "baseSeverity": a.CVSS.Severity,
			},
		}}
	}
	references := []any{}
	for _, url := range a.References {
		references = append(references, map[string]any{"category": "external", "summary": url, "url": url})
	}
	document := map[string]any{
		"category": category, "csaf_version": "2.0", "lang": "en", "title": a.Title,
		"publisher": map[string]any{"category": "vendor", "name": "Hubtask", "namespace": "https://hubtask.eu"},
		"tracking": map[string]any{
			"id": a.ID, "status": "final", "version": fmt.Sprint(len(history)),
			"initial_release_date": stamp(a.InitialRelease), "current_release_date": stamp(a.current()),
			"revision_history": history,
		},
	}
	if len(references) > 0 {
		document["references"] = references
	}
	return map[string]any{
		"document": document,
		"product_tree": map[string]any{"branches": []any{map[string]any{
			"category": "vendor", "name": "Hubtask",
			"branches": []any{map[string]any{"category": "product_name", "name": "Hubtask", "branches": branches}},
		}}},
		"vulnerabilities": []any{vulnerability},
	}
}

func stamp(at time.Time) string { return at.UTC().Format(time.RFC3339) }

// write lays the directory out: the documents with their hashes, index.txt, changes.csv (newest
// first) and provider-metadata.json.
func write(advisories []Advisory, out, base string) error {
	// Every file is written through a root opened on the output directory, so an advisory whose
	// path climbs out of it is refused rather than written elsewhere.
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	root, err := os.OpenRoot(out)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	var index, changes []string
	latest := time.Time{}
	byDate := slices.Clone(advisories)
	sort.SliceStable(byDate, func(i, j int) bool { return byDate[i].current().After(byDate[j].current()) })
	for _, advisory := range advisories {
		encoded, err := json.MarshalIndent(advisory.Document(), "", "  ")
		if err != nil {
			return err
		}
		encoded = append(encoded, '\n')
		path := filepath.FromSlash(advisory.Path())
		if err := root.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return err
		}
		if err := root.WriteFile(path, encoded, 0o600); err != nil {
			return err
		}
		name := filepath.Base(path)
		sum256 := sha256.Sum256(encoded)
		sum512 := sha512.Sum512(encoded)
		if err := root.WriteFile(path+".sha256", []byte(hex.EncodeToString(sum256[:])+"  "+name+"\n"), 0o600); err != nil {
			return err
		}
		if err := root.WriteFile(path+".sha512", []byte(hex.EncodeToString(sum512[:])+"  "+name+"\n"), 0o600); err != nil {
			return err
		}
		index = append(index, advisory.Path())
		if advisory.current().After(latest) {
			latest = advisory.current()
		}
	}
	for _, advisory := range byDate {
		changes = append(changes, fmt.Sprintf("%q,%q", advisory.Path(), stamp(advisory.current())))
	}
	if err := root.WriteFile("index.txt", []byte(strings.Join(index, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if err := root.WriteFile("changes.csv", []byte(strings.Join(changes, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	metadata, err := json.MarshalIndent(map[string]any{
		"canonical_url":              base + "/provider-metadata.json",
		"distributions":              []any{map[string]any{"directory_url": base + "/"}},
		"last_updated":               stamp(latest),
		"list_on_CSAF_aggregators":   true,
		"metadata_version":           "2.0",
		"mirror_on_CSAF_aggregators": true,
		"publisher":                  map[string]any{"category": "vendor", "name": "Hubtask", "namespace": "https://hubtask.eu"},
		"role":                       "csaf_publisher",
	}, "", "  ")
	if err != nil {
		return err
	}
	return root.WriteFile("provider-metadata.json", append(metadata, '\n'), 0o600)
}
