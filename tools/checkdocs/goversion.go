// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// The Go version is stated in seventeen places across eight files: go.mod, five workflows, the
// Dockerfile, the support matrix, the README and the onboarding guide. Nothing kept them in step,
// and they had already drifted - a Dependabot pull request bumping the base image to 1.26 would
// have left the released binary built by a compiler no gate had ever run.
//
// go.mod is the source. Everything else has to agree with it on the major.minor, and this says so
// on every pull request rather than at the release where it matters.
//
// Only major.minor is compared, on both sides. The workflows state the exact patch on purpose -
// `setup-go` with a bare major.minor resolves to whichever patch the runner image happens to
// carry, and a tool built by `go install` from that Go then has to analyse a module whose
// `toolchain` directive may name a newer one. That mismatch is what makes go-licenses report
// "does not have module info" and golangci-lint refuse the module outright, on some runners and
// not others. Pinning the patch in CI removes the coin toss; comparing only major.minor here
// keeps this check from failing every time the patch moves in lockstep.

const goModFile = "go.mod"

// goDirective matches the version go.mod requires: `go 1.26.7`.
var goDirective = regexp.MustCompile(`(?m)^go (\d+\.\d+)(?:\.\d+)?\s*$`)

// goVersionSource is one place that repeats it, and how to find it there.
type goVersionSource struct {
	// path is the file, repository-relative. An empty Dir means one file; otherwise every .yml
	// in that directory is read.
	path string
	dir  string
	// pattern captures the major.minor in group 1.
	pattern *regexp.Regexp
	// atLeast is how many statements the file must contain. It guards the failure this check
	// would otherwise have: a reworded sentence stops matching, and a pattern that finds nothing
	// would pass silently.
	atLeast int
	// what names the statement in the failure message.
	what string
}

var goVersionSources = []goVersionSource{
	// Eight since the delegation workflow stopped building anything: it answers that delegation is
	// off and needs no Go toolchain to do it (C-10's follow-up). The number is a floor rather than
	// an equality for exactly this reason - it catches a pin that was reworded, not a step that was
	// deliberately removed.
	{dir: ".github/workflows", pattern: regexp.MustCompile(`go-version:\s*"(\d+\.\d+)(?:\.\d+)?"`), atLeast: 8, what: "go-version:"},
	{dir: ".github/workflows", pattern: regexp.MustCompile(`GO_VERSION:\s*"(\d+\.\d+)(?:\.\d+)?"`), atLeast: 2, what: "GO_VERSION:"},
	{path: "deploy/docker/Dockerfile", pattern: regexp.MustCompile(`FROM golang:(\d+\.\d+)`), atLeast: 1, what: "the build image"},
	{path: "docs/architecture/support-matrix.md", pattern: regexp.MustCompile(`\|\s*Go \(building from source\)\s*\|\s*(\d+\.\d+)\s*\|`), atLeast: 1, what: "the support matrix row"},
	{path: "README.md", pattern: regexp.MustCompile(`Go \(≥ (\d+\.\d+)\)`), atLeast: 1, what: "the technology table"},
	{path: "docs/onboarding.md", pattern: regexp.MustCompile(`Go (\d+\.\d+)\+`), atLeast: 1, what: "the prerequisites"},
}

// checkGoVersion reconciles every statement of the Go version with go.mod.
func checkGoVersion(root string) []string {
	required := goDirective.FindStringSubmatch(read(root, goModFile))
	if required == nil {
		return []string{goModFile + ": no `go` directive found - has the file changed shape?"}
	}
	want := required[1]

	var problems []string
	for _, source := range goVersionSources {
		files, err := goVersionFiles(root, source)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}

		found := 0
		for _, file := range files {
			for _, match := range source.pattern.FindAllStringSubmatch(read(root, file), -1) {
				found++
				if match[1] != want {
					problems = append(problems, fmt.Sprintf(
						"%s: %s says Go %s, go.mod requires %s", file, source.what, match[1], want))
				}
			}
		}
		if found < source.atLeast {
			// Either a statement was removed, or it was reworded past the pattern. Both mean
			// this check has stopped covering something it used to cover.
			problems = append(problems, fmt.Sprintf(
				"%s: found %d statement(s) of %q, expected at least %d - was one removed or reworded?",
				source.location(), found, source.what, source.atLeast))
		}
	}

	sort.Strings(problems)
	return problems
}

func (s goVersionSource) location() string {
	if s.dir != "" {
		return s.dir
	}
	return s.path
}

func goVersionFiles(root string, source goVersionSource) ([]string, error) {
	if source.dir == "" {
		if _, err := os.Stat(filepath.Join(root, source.path)); err != nil {
			return nil, fmt.Errorf("%s is missing, and it states the Go version", source.path)
		}
		return []string{source.path}, nil
	}

	entries, err := os.ReadDir(filepath.Join(root, source.dir))
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", source.dir, err)
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yml") {
			files = append(files, filepath.Join(source.dir, entry.Name()))
		}
	}
	return files, nil
}
