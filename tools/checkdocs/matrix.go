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

	jobs, err := workflowJobs(root)
	if err != nil {
		return []string{err.Error()}
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

	sort.Strings(problems)
	return problems
}

// workflowJobs collects every job as `workflow.yml:job-id`.
func workflowJobs(root string) (map[string]bool, error) {
	entries, err := os.ReadDir(filepath.Join(root, workflowDir))
	if err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", workflowDir, err)
	}

	jobs := map[string]bool{}
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
		for _, match := range workflowJob.FindAllStringSubmatch(body, -1) {
			jobs[name+":"+match[1]] = true
		}
	}
	return jobs, nil
}
