// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command eventmatrix writes docs/audit/event-matrix.md from the use case catalogue.
//
// `audit.md` §4 names that file as "the full matrix, generated from the use case registry" and it
// has never existed. Generated rather than written, for the reason the section gives: the registry
// is the only honest source of what this system records, and a matrix maintained by hand is a
// matrix that is wrong by the second release - it would describe what somebody remembered to write
// down rather than what the running system does.
//
// It runs from `go generate ./...`, which `make generate` runs and `make gate-quick` fails on a
// diff of. So the file cannot drift: a use case whose audit declaration changes and whose matrix
// was not regenerated is a red build rather than a stale document.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// output is where the matrix lands, relative to the repository root. The generator finds the root
// by walking up to the go.mod, so that it produces the same file whichever directory
// `go generate` invoked it from.
const output = "docs/audit/event-matrix.md"

func main() {
	root, err := repositoryRoot()
	if err != nil {
		fail(err)
	}

	page := render(catalogue.Descriptors())
	path := filepath.Join(root, output)
	// The narrow modes gosec asks for. What lands in git is the content; the mode of a working
	// copy is not part of it, so there is nothing to trade away by taking the stricter ones.
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		fail(err)
	}
	if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "eventmatrix:", err)
	os.Exit(1)
}

// repositoryRoot walks up from the working directory until it finds the module file.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above %s", dir)
		}
		dir = parent
	}
}

// row is one line of the matrix.
type row struct {
	area     string
	action   string
	useCase  string
	target   string
	severity string
	required bool
	readOnly bool
}

// render produces the whole page.
func render(descriptors []usecase.Descriptor) string {
	rows := make([]row, 0, len(descriptors))
	for _, descriptor := range descriptors {
		action := string(descriptor.Audit.Action)
		rows = append(rows, row{
			area:     areaOf(action),
			action:   action,
			useCase:  descriptor.Name,
			target:   descriptor.Audit.TargetType,
			severity: string(descriptor.Audit.Severity),
			required: descriptor.Audit.Required,
			readOnly: descriptor.ReadOnly,
		})
	}
	// By area first, so that a reader looking for "what does this system record about deletions"
	// finds one table rather than four sections with the same heading.
	sort.Slice(rows, func(i, j int) bool {
		switch {
		case rows[i].area != rows[j].area:
			return rows[i].area < rows[j].area
		case rows[i].action != rows[j].action:
			return rows[i].action < rows[j].action
		default:
			return rows[i].useCase < rows[j].useCase
		}
	})

	var page strings.Builder
	page.WriteString(preamble(rows))

	area := ""
	for _, entry := range rows {
		if entry.area != area {
			area = entry.area
			fmt.Fprintf(&page, "\n## %s\n\n", area)
			page.WriteString("| Action | Use case | Target | Severity | Recorded |\n")
			page.WriteString("|---|---|---|---|---|\n")
		}
		fmt.Fprintf(&page, "| `%s` | %s | `%s` | %s | %s |\n",
			entry.action, entry.useCase, entry.target, entry.severity, recorded(entry))
	}
	return page.String()
}

// recorded says when an entry is written, which is the column a reader is actually looking for.
//
// Two states rather than "yes/no". A writing use case records what it did; a read records only
// when it is refused, because a trail that grew by being read would bury what it is for
// (audit.md §4, and `ListAuditEntries` for the reasoning).
func recorded(entry row) string {
	if entry.required {
		return "Every time"
	}
	if entry.readOnly {
		return "When refused"
	}
	return "Optional"
}

// areaOf groups the actions the way §4's table does: by the first segment of the code, which is
// the resource the action is about.
func areaOf(action string) string {
	prefix, _, found := strings.Cut(action, ".")
	if !found || prefix == "" {
		return "Other"
	}
	if name, known := areas[prefix]; known {
		return name
	}
	return strings.ToUpper(prefix[:1]) + prefix[1:]
}

// areas are the names §4 gives the groups, for the prefixes that have one. A prefix that is not
// here is titled from itself rather than dropped - a new area appearing in this document is how a
// reader learns that the system grew one.
var areas = map[string]string{
	"account":      "Accounts",
	"audit":        "Audit of the audit",
	"auth":         "Authentication",
	"backup":       "Backup and restore",
	"bucket":       "Structure: buckets and labels",
	"calendar":     "Views, exports and feeds",
	"comment":      "Data: comments",
	"container":    "Data: hubs and collections",
	"custom_field": "Structure: custom fields",
	"group":        "Permissions",
	"item":         "Data: entries",
	"job":          "Background work",
	"label":        "Structure: buckets and labels",
	"lifecycle":    "Retention and legal hold",
	"media":        "Data: media",
	"membership":   "Permissions",
	"recurrence":   "Recurrence and reminders",
	"reminder":     "Recurrence and reminders",
	"restore":      "Backup and restore",
	"retention":    "Retention and legal hold",
	"template":     "Templates",
	"trash":        "Data: the trash",
	"view":         "Views, exports and feeds",
}

// preamble is everything above the first table, including the counts - which are the numbers
// audit.md §4 and the release criteria talk about, and which nobody should have to count by hand.
func preamble(rows []row) string {
	actions := map[string]bool{}
	required := 0
	for _, entry := range rows {
		actions[entry.action] = true
		if entry.required {
			required++
		}
	}

	return fmt.Sprintf(`<!-- Generated by tools/eventmatrix from the use case catalogue. Do not edit by hand:
     run "make generate", which is what the pull request check runs and fails on a diff of. -->

# Event matrix

Every use case this build has, and what the audit trail records about it. `+"`audit.md`"+` §4 names
this file as the full matrix and gives an extract of it; this is the whole, and it is generated
from [the catalogue](../../core/application/catalogue/Catalogue.go) rather than maintained
alongside it, because a matrix written by hand is a matrix that is wrong by the second release.

**%d use cases, %d distinct action codes, %d of them recorded on every call.** A use case that
writes and declares no audit obligation fails the build (gate SG-13); a read declares one all the
same, because a *refused* read is recorded against the action that was refused.

The severity is what the entry carries when the use case succeeds. A refusal is recorded by the
authorisation service against the same action with `+"`outcome=DENIED`"+`, which is what makes the
trail complete without every developer having to remember it (audit.md §7).

What is **not** here is what audit.md §4 keeps out of the trail: the content of tasks, notes,
comments and attachments; passwords, tokens and secrets in any form; full IP addresses; and AI
prompts and responses. The `+"`changes`"+` of an entry are masked per field classification, so a
`+"`SENSITIVE`"+` value travels as a fingerprint and a `+"`SECRET`"+` one not at all.
`, len(rows), len(actions), required)
}
