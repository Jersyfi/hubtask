// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cilocal

import (
	"slices"
	"testing"
)

const workflowFixture = `jobs:
  changes:
    steps:
      - uses: dorny/paths-filter
        with:
          filters: |
            workspace: &workspace-manifests
              - 'package.json'
            go:
              - '**/*.go'
              - 'core/**'
            webapp:
              - 'apps/webapp/**'
              - *workspace-manifests
  next:
    runs-on: ubuntu-latest
`

func TestLoadFiltersResolvesTheAnchor(t *testing.T) {
	filters, err := LoadFilters([]byte(workflowFixture))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := filters["webapp"], []string{"apps/webapp/**", "package.json"}; !slices.Equal(got, want) {
		t.Errorf("webapp = %v, want %v - an alias is the anchored list, flattened", got, want)
	}
	if len(filters) != 3 {
		t.Errorf("parsed %d filters, want 3 - the block ends where its indentation does", len(filters))
	}
}

func TestLoadFiltersRefusesAWorkflowWithoutTheBlock(t *testing.T) {
	if _, err := LoadFilters([]byte("jobs: {}\n")); err == nil {
		t.Error("a workflow without a filters block was accepted")
	}
}

func TestMatches(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"core/**", "core/domain/x.go", true},
		{"core/**", "corex/a.go", false},
		{"**/*.md", "docs/adr/README.md", true},
		{"**/*.md", "README.mdx", false},
		{"go.mod", "go.mod", true},
		{"go.mod", "sdk/go/go.mod", false},
		{"apps/*/package.json", "apps/webapp/package.json", true},
	}
	for _, c := range cases {
		if got := Matches(c.pattern, c.file); got != c.want {
			t.Errorf("Matches(%q, %q) = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}

func TestTouched(t *testing.T) {
	filters, err := LoadFilters([]byte(workflowFixture))
	if err != nil {
		t.Fatal(err)
	}
	touched := Touched(filters, []string{"package.json", "docs/x.txt"})
	if !touched["workspace"] || !touched["webapp"] || touched["go"] {
		t.Errorf("Touched = %v, want workspace and webapp only - a lockfile change is every package's", touched)
	}
}
