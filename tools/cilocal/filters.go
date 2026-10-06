// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package cilocal is what the pull request pipeline decides, readable outside the pipeline: the
// path filters of `ci.yml`'s `changes` job, and which of its jobs a change runs (ADR-0079).
//
// Two readers need it. `make verify-pr` selects the gates a branch needs locally from the same
// filters CI selects them from, so that the two cannot drift; and `test/architecture` holds both
// the filters and the job table below to the workflow, so that a job added to `ci.yml` without a
// local counterpart, or a filter changed in one place only, is a red gate rather than a surprise.
package cilocal

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// WorkflowPath is where the pipeline lives, relative to the repository root.
const WorkflowPath = ".github/workflows/ci.yml"

// LoadFilters reads the `filters:` block the `changes` job hands to dorny/paths-filter.
//
// The block is a YAML document inside a YAML string, so it is extracted by indentation and parsed
// on its own. It carries an anchor and aliases (`*workspace-manifests`), which is exactly why it
// is parsed rather than pattern-matched: a reader has to see what a package's filter really
// contains, aliases resolved.
func LoadFilters(workflow []byte) (map[string][]string, error) {
	lines := strings.Split(string(workflow), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "filters: |" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil, fmt.Errorf("%s has no `filters: |` block - the changes job is what every other job reads", WorkflowPath)
	}

	const indent = "            " // the block scalar's own indentation inside the workflow
	var block []string
	for _, line := range lines[start:] {
		if strings.TrimSpace(line) == "" {
			block = append(block, "")
			continue
		}
		if !strings.HasPrefix(line, indent) {
			break
		}
		block = append(block, strings.TrimPrefix(line, indent))
	}

	// An alias inserts the anchored *sequence* as one item, so a package's list is a list with a
	// list inside it. dorny/paths-filter flattens that itself - the anchor is the documented way
	// to share a group of paths - so this flattens it the same way rather than refusing the file.
	var parsed map[string][]any
	if err := yaml.Unmarshal([]byte(strings.Join(block, "\n")), &parsed); err != nil {
		return nil, fmt.Errorf("the filters block does not parse as YAML: %w", err)
	}

	filters := make(map[string][]string, len(parsed))
	for name, items := range parsed {
		patterns, err := flatten(name, items)
		if err != nil {
			return nil, err
		}
		filters[name] = patterns
	}
	return filters, nil
}

func flatten(filter string, items []any) ([]string, error) {
	var patterns []string
	for _, item := range items {
		switch value := item.(type) {
		case string:
			patterns = append(patterns, value)
		case []any:
			nested, err := flatten(filter, value)
			if err != nil {
				return nil, err
			}
			patterns = append(patterns, nested...)
		default:
			return nil, fmt.Errorf("the filter %q holds a %T, which is neither a path nor a group of them", filter, item)
		}
	}
	return patterns, nil
}

// Matches covers the three shapes the filter list uses: a tree (`core/**`), an extension anywhere
// (`**/*.md`), and an exact path (`go.mod`). Deliberately not a general glob engine - a fourth
// shape should be read here by whoever writes it rather than resolved by a dependency.
func Matches(pattern, file string) bool {
	switch {
	case strings.HasPrefix(pattern, "**/*."):
		return strings.HasSuffix(file, strings.TrimPrefix(pattern, "**/*"))
	case strings.HasSuffix(pattern, "/**"):
		return strings.HasPrefix(file, strings.TrimSuffix(pattern, "**"))
	case strings.Contains(pattern, "*"):
		ok, err := path.Match(pattern, file)
		return err == nil && ok
	default:
		return pattern == file
	}
}

// Touched answers which filters a set of changed files sets, the way the `changes` job's outputs
// would read for a pull request carrying exactly those files.
func Touched(filters map[string][]string, files []string) map[string]bool {
	touched := map[string]bool{}
	for name, patterns := range filters {
		for _, file := range files {
			if matchesAny(patterns, file) {
				touched[name] = true
				break
			}
		}
	}
	return touched
}

func matchesAny(patterns []string, file string) bool {
	for _, pattern := range patterns {
		if Matches(pattern, file) {
			return true
		}
	}
	return false
}
