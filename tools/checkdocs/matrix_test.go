// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"strings"
	"testing"
)

// matrixRoot is a repository holding only a support matrix and one workflow.
func matrixRoot(t *testing.T, table, workflow string) string {
	t.Helper()
	return writeTree(t, map[string]string{matrixFile: table, workflowDir + "/nightly.yml": workflow})
}

const reportingWorkflow = "on: push\njobs:\n  matrix-arm64:\n    runs-on: x\n  report:\n    needs: [matrix-arm64]\n"

// UC-INS-17 check 2: every supported row names the CI job that proves it, and the gate refuses a
// row without its job and a job without its row.
func TestTheSupportMatrixGateRefusesARowWithoutItsJobAndAJobWithoutItsRow(t *testing.T) {
	for _, testCase := range []struct {
		name, table, workflow, want string
	}{
		{"a row and its job", "| arm64 | supported | `nightly.yml:matrix-arm64` |\n", reportingWorkflow, ""},
		{"a row whose job does not exist", "| arm64 | supported | `nightly.yml:matrix-arm64` |\n| riscv | supported | `nightly.yml:matrix-riscv` |\n",
			reportingWorkflow, "nightly.yml:matrix-riscv proves a row, and that job does not exist"},
		{"a job no row claims", "| amd64 | supported | `nightly.yml:report` |\n", reportingWorkflow,
			"nightly.yml:matrix-arm64 runs and no row"},
		{"a job nothing waits on", "| arm64 | supported | `nightly.yml:matrix-arm64` |\n",
			"on: push\njobs:\n  matrix-arm64:\n    runs-on: x\n", "no job in that workflow waits on it"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			problems := checkSupportMatrix(matrixRoot(t, testCase.table, testCase.workflow))
			if testCase.want == "" {
				if len(problems) != 0 {
					t.Errorf("a matrix whose rows and jobs agree answered %v", problems)
				}
				return
			}
			if !strings.Contains(strings.Join(problems, "\n"), testCase.want) {
				t.Errorf("answered %v, want a problem saying %q", problems, testCase.want)
			}
		})
	}
}
