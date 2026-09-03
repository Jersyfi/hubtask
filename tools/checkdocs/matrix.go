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

// The support matrix half of the gate (docs/architecture/support-matrix.md).
//
// The table's last column names the CI job that proves each row. That is the whole mechanism: a
// claim of support and the evidence for it live in one place, and this checks that the evidence
// exists. It runs in both directions, because both failures are real and neither is visible:
//
//   - A row whose job does not exist is a claim nobody can check. It happens when a job is renamed
//     or a platform is dropped from a workflow and the table is forgotten.
//   - A matrix job with no row is support that is being paid for and not promised - or, worse,
//     quietly removed evidence for a promise still on the page. Deleting a job must not be a way
//     to make a red build green.
//   - A matrix job nothing waits on is evidence nobody reads. The nightly turns a failure into an
//     issue from the `needs` list of its reporting job, so a matrix job missing from that list
//     fails in silence - which is what `matrix-hubctl` did, behind three rows of the table, for as
//     long as the reporting job has existed. A run that is red where nobody looks is the same
//     thing as a row nothing proves.
//
// Jobs count as matrix jobs by their identifier prefix, `matrix-`. A convention rather than a
// list, so that adding a platform is adding a job and a row - never editing this file.

const (
	matrixFile   = "docs/architecture/support-matrix.md"
	workflowDir  = ".github/workflows"
	matrixPrefix = "matrix-"
)

// jobReference matches the `workflow.yml:job-id` form the table's last column uses.
var jobReference = regexp.MustCompile(`\x60([a-z0-9-]+\.yml):([a-z0-9-]+)\x60`)

// workflowJob matches a job identifier in a workflow: two spaces of indentation, a name, a colon.
// Deliberately not a YAML parse - this needs the identifiers and their order, and a parser would
// need a dependency for what one regular expression states exactly.
var workflowJob = regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):\s*$`)

func checkSupportMatrix(root string) []string {
	table := read(root, matrixFile)
	if table == "" {
		return []string{matrixFile + " is missing or empty - the support matrix is what the gate exists for"}
	}

	workflows, err := readWorkflows(root)
	if err != nil {
		return []string{err.Error()}
	}

	jobs := map[string]bool{}
	for _, workflow := range workflows {
		for _, job := range workflow.jobs {
			jobs[workflow.file+":"+job] = true
		}
	}

	var problems []string
	claimed := map[string]bool{}
	for _, match := range jobReference.FindAllStringSubmatch(table, -1) {
		workflow, job := match[1], match[2]
		reference := workflow + ":" + job
		claimed[reference] = true

		if !jobs[reference] {
			problems = append(problems, fmt.Sprintf(
				"%s claims %s proves a row, and that job does not exist", matrixFile, reference))
		}
	}
	if len(claimed) == 0 {
		return append(problems, matrixFile+": no row names a CI job - the last column is the point of the table")
	}

	for reference := range jobs {
		_, job, _ := strings.Cut(reference, ":")
		if !strings.HasPrefix(job, matrixPrefix) || claimed[reference] {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s runs and no row in %s claims it - support that is paid for and not promised, "+
				"or a row somebody deleted", reference, matrixFile))
	}

	for _, workflow := range workflows {
		for _, job := range workflow.jobs {
			if !strings.HasPrefix(job, matrixPrefix) || workflow.needed[job] {
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"%s:%s runs and no job in that workflow waits on it - a matrix job outside the "+
					"reporting job's `needs` fails where nobody is told", workflow.file, job))
		}
	}

	sort.Strings(problems)
	return problems
}

// workflow is one file's job identifiers and the set of identifiers some job in it waits on.
type workflow struct {
	file   string
	jobs   []string
	needed map[string]bool
}

// readWorkflows collects the jobs of every workflow, and which of them something waits on.
func readWorkflows(root string) ([]workflow, error) {
	entries, err := os.ReadDir(filepath.Join(root, workflowDir))
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", workflowDir, err)
	}

	var workflows []workflow
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yml") {
			continue
		}
		content := read(root, filepath.Join(workflowDir, name))
		// Everything above `jobs:` is triggers and permissions, and those are indented the same
		// way. Cutting there is what keeps `contents:` from reading as a job.
		_, body, found := strings.Cut(content, "\njobs:\n")
		if !found {
			continue
		}

		current := workflow{file: name, needed: map[string]bool{}}
		for _, match := range workflowJob.FindAllStringSubmatch(body, -1) {
			current.jobs = append(current.jobs, match[1])
		}
		for _, dependency := range dependencies(body) {
			current.needed[dependency] = true
		}
		workflows = append(workflows, current)
	}
	return workflows, nil
}

// identifier matches a job identifier inside a `needs:` clause.
var identifier = regexp.MustCompile(`[a-z][a-z0-9-]*`)

// dependencies collects every job identifier something in the body waits on. Still not a YAML
// parse, and for the reason the job regexp above is not one: what is wanted is a set of
// identifiers, and each of the three forms `needs:` takes - one name, a flow sequence on one line
// or several, a block of dashed entries - is a run of identifiers between that keyword and the
// next line indented no deeper than it.
func dependencies(body string) []string {
	var found []string
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		keyword := strings.Index(line, "needs:")
		// Only where `needs:` opens the line. Anywhere else it is prose in a comment, or the
		// `needs` context inside an expression - neither declares a dependency.
		if keyword < 0 || strings.TrimSpace(line[:keyword]) != "" {
			continue
		}
		found = append(found, identifier.FindAllString(line[keyword+len("needs:"):], -1)...)

		for _, next := range lines[i+1:] {
			trimmed := strings.TrimSpace(next)
			if trimmed == "" {
				continue
			}
			if indentation(next) <= keyword {
				break
			}
			if strings.HasPrefix(trimmed, "#") {
				continue
			}
			found = append(found, identifier.FindAllString(next, -1)...)
		}
	}
	return found
}

func indentation(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}
