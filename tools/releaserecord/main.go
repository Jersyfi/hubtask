// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command releaserecord writes the record a release keeps of itself (versioning-release.md §3,
// F-13): which commit became which image, what it contains (the SBOM's and the licence list's
// hashes), which pipeline run built and checked it, which advisories it fixes, and the support
// policy it was released under - what a provider or a court asks of a release long after it
// shipped (Directive (EU) 2024/2853 on product liability; CRA Annex VII). It names the evidence by
// hash and address and copies none of it; the release attaches it beside the SBOM and signs it.
//
// Usage: releaserecord -version vX.Y.Z -commit <sha> -image <ref> -digest <sha256:…> -run <url>
//
//	-at <RFC 3339> -go <version> [-advisories dir] [file to hash …]
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Record is the release record's shape; its `schema` names the version of that shape.
type Record struct {
	Schema        string            `json:"schema"`
	Version       string            `json:"version"`
	Commit        string            `json:"commit"`
	ReleasedAt    string            `json:"released_at"`
	Image         string            `json:"image"`
	Digest        string            `json:"digest"`
	Pipeline      string            `json:"pipeline"`
	GoVersion     string            `json:"go_version"`
	Gates         []string          `json:"gates"`
	Evidence      map[string]string `json:"evidence"`
	Fixes         []string          `json:"fixes_advisories"`
	SupportPolicy string            `json:"support_policy"`
}

// The gates release.yml runs on the tag; the record states them, the pipeline run proves them.
var releaseGates = []string{"make verify", "sbom", "cosign signature", "build provenance", "third-party licences"}

const supportPolicy = "https://github.com/Jersyfi/hubtask/blob/main/docs/architecture/versioning-release.md#5-support-and-upgrade-policy"

func main() {
	version := flag.String("version", "", "the tag")
	commit := flag.String("commit", "", "the commit")
	image := flag.String("image", "", "the image reference")
	digest := flag.String("digest", "", "the image digest")
	run := flag.String("run", "", "the pipeline run's address")
	at := flag.String("at", "", "the release moment, RFC 3339")
	goVersion := flag.String("go", "", "the Go toolchain")
	advisories := flag.String("advisories", "security/advisories", "the advisories directory")
	flag.Parse()

	record, err := build(Inputs{
		Version: *version, Commit: *commit, Image: *image, Digest: *digest, Run: *run, At: *at,
		Go: *goVersion, Advisories: *advisories, Files: flag.Args(),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "releaserecord:", err)
		os.Exit(1)
	}
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "releaserecord:", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

// Inputs are the facts the pipeline hands over.
type Inputs struct {
	Version, Commit, Image, Digest, Run, At, Go, Advisories string
	Files                                                   []string
}

func build(in Inputs) (Record, error) {
	for name, value := range map[string]string{
		"version": in.Version, "commit": in.Commit, "image": in.Image, "digest": in.Digest, "run": in.Run, "at": in.At,
	} {
		if strings.TrimSpace(value) == "" {
			return Record{}, fmt.Errorf("-%s is required", name)
		}
	}
	if _, err := time.Parse(time.RFC3339, in.At); err != nil {
		return Record{}, fmt.Errorf("-at: %w", err)
	}
	evidence := map[string]string{}
	for _, file := range in.Files {
		raw, err := os.ReadFile(file) //nolint:gosec // G304: the files the workflow names
		if err != nil {
			return Record{}, err
		}
		sum := sha256.Sum256(raw)
		evidence[filepath.Base(file)] = "sha256:" + hex.EncodeToString(sum[:])
	}
	fixes, err := fixedIn(in.Advisories, strings.TrimPrefix(in.Version, "v"))
	if err != nil {
		return Record{}, err
	}
	return Record{
		Schema: "hubtask-release-record/1", Version: in.Version, Commit: in.Commit, ReleasedAt: in.At,
		Image: in.Image, Digest: in.Digest, Pipeline: in.Run, GoVersion: in.Go, Gates: releaseGates,
		Evidence: evidence, Fixes: fixes, SupportPolicy: supportPolicy,
	}, nil
}

// fixedIn answers the advisories whose fixed versions name this one (tools/csaf reads the same
// files); a missing directory is none.
func fixedIn(dir, version string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	fixes := []string{}
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // G304: the directory the workflow names
		if err != nil {
			return nil, err
		}
		var advisory struct {
			ID    string   `yaml:"id"`
			Fixed []string `yaml:"fixed"`
		}
		if err := yaml.Unmarshal(raw, &advisory); err != nil {
			return nil, errors.Join(fmt.Errorf("%s", file), err)
		}
		if slices.Contains(advisory.Fixed, version) {
			fixes = append(fixes, advisory.ID)
		}
	}
	slices.Sort(fixes)
	return fixes, nil
}
