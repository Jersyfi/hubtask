// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// What runs after every CI run (scripts/ci/after-ci.sh, ADR-0083), against the stand-in `gh` of
// maingreen_test.go: each write - a re-run, a hold, an issue - happens exactly when it should.

const aftermathPath = "scripts/ci/after-ci.sh"

// `workflow_run` runs with write rights for a fork's run as well, so the workflow must check out
// nothing of the run - the default branch, no `ref:` - and hand the run's values to the script only
// through the environment: an expression inside `run:` would put a branch name into a shell.
func TestTheAfterCIWorkflowExecutesNothingOfTheRun(t *testing.T) {
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Uses string            `yaml:"uses"`
				With map[string]string `yaml:"with"`
				Run  string            `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(readFile(t, "../../.github/workflows/after-ci.yml"), &workflow); err != nil {
		t.Fatalf("after-ci.yml does not parse: %v", err)
	}
	runs := 0
	for id, job := range workflow.Jobs {
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") {
				if _, ok := step.With["ref"]; ok {
					t.Errorf("after-ci.yml: job %q checks out a ref; it must check out the default branch only", id)
				}
			}
			if step.Run != "" {
				runs++
				if strings.Contains(step.Run, "${{") {
					t.Errorf("after-ci.yml: job %q puts an expression into `run:` %q - pass it through `env:`", id, step.Run)
				}
			}
		}
	}
	if runs == 0 {
		t.Fatal("after-ci.yml runs nothing - the file has moved, not shrunk")
	}
}

func runEnv(event, branch, conclusion, attempt string) map[string]string {
	return map[string]string{
		"MODE": "run", "RUN_ID": "77", "ATTEMPT": attempt, "CONCLUSION": conclusion,
		"EVENT": event, "BRANCH": branch, "HEAD_SHA": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
}

func jobsOf(id int, failed ...string) route {
	list := []string{`{"id":` + fmt.Sprint(id*10) + `,"name":"CI required","conclusion":"` + map[bool]string{true: "failure", false: "success"}[containsName(failed, "CI required")] + `"}`}
	for _, name := range failed {
		if name != "CI required" {
			list = append(list, fmt.Sprintf(`{"id":%d,"name":%q,"conclusion":"failure"}`, id*10+1, name))
		}
	}
	list = append(list, fmt.Sprintf(`{"id":%d,"name":"Documentation","conclusion":"success"}`, id*10+2))
	return route{fmt.Sprintf("api repos/%s/actions/runs/%d/jobs?per_page=100", stubRepo, id), `{"jobs":[` + strings.Join(list, ",") + `]}`}
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func prRuns(head, body string) route {
	return route{"api repos/" + stubRepo + "/actions/workflows/ci.yml/runs?head_sha=" + head + "&event=pull_request&per_page=1", body}
}

var (
	issuesArgs = "api repos/" + stubRepo + "/issues?state=all&labels=finding&per_page=100"
	redCommit  = route{"api repos/" + stubRepo + "/commits/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		`{"commit":{"message":"feat(admin): operators and machines (#1293)\n\nbody"}}`}
	redJobs = func(attempt string) route {
		return route{"api repos/" + stubRepo + "/actions/runs/77/attempts/" + attempt + "/jobs?per_page=100",
			`{"jobs":[{"name":"Format, lint, generation","conclusion":"success"},{"name":"Workspace (webapp)","conclusion":"failure"},{"name":"CI required","conclusion":"failure"}]}`}
	}
	// Five open pull requests: #1 failed at the guard alone, #2 failed a real job, #3 is a draft,
	// #4 is still running, #6 has no run, #7 is green.
	openPulls = []route{
		{"api repos/" + stubRepo + "/pulls?state=open&per_page=100", `[
			{"number":1,"draft":false,"head":{"sha":"h1"}},
			{"number":2,"draft":false,"head":{"sha":"h2"}},
			{"number":3,"draft":true,"head":{"sha":"h3"}},
			{"number":4,"draft":false,"head":{"sha":"h4"}},
			{"number":6,"draft":false,"head":{"sha":"h6"}},
			{"number":7,"draft":false,"head":{"sha":"h7"}}]`},
		prRuns("h1", `{"workflow_runs":[{"id":101,"status":"completed","conclusion":"failure"}]}`),
		jobsOf(101, "CI required"),
		prRuns("h2", `{"workflow_runs":[{"id":102,"status":"completed","conclusion":"failure"}]}`),
		jobsOf(102, "Unit and domain tests", "CI required"),
		prRuns("h4", `{"workflow_runs":[{"id":104,"status":"in_progress","conclusion":null}]}`),
		prRuns("h6", `{"workflow_runs":[]}`),
		prRuns("h7", `{"workflow_runs":[{"id":107,"status":"completed","conclusion":"success"}]}`),
		jobsOf(107),
	}
)

func with(base []route, more ...route) []route {
	return append(append([]route{}, base...), more...)
}

func TestAFailedRunIsReRunOnceAndARedMainIsFiled(t *testing.T) {
	t.Run("a pull request's first failure is re-run, nothing is filed", func(t *testing.T) {
		stub := newGHStub(t, []route{jobsOf(77, "Unit and domain tests", "CI required")})
		out, code := stub.run(t, aftermathPath, runEnv("pull_request", "claude/x", "failure", "1"))
		if code != 0 || strings.Contains(out, "::warning::") {
			t.Fatalf("exit %d, want 0 without a warning:\n%s", code, out)
		}
		if got := stub.wrote(t, "run "); len(got) != 1 || got[0] != "run rerun 77 --failed --repo "+stubRepo {
			t.Errorf("writes %v; want exactly one re-run", got)
		}
		if got := stub.wrote(t, "issue"); len(got) != 0 {
			t.Errorf("a pull request's failure filed %v", got)
		}
	})

	t.Run("a pull request that failed at the guard alone is not re-run at once", func(t *testing.T) {
		stub := newGHStub(t, []route{jobsOf(77, "CI required")})
		if out, code := stub.run(t, aftermathPath, runEnv("pull_request", "claude/x", "failure", "1")); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if got := stub.wrote(t, "run "); len(got) != 0 {
			t.Errorf("writes %v; want none - it waits for main", got)
		}
	})

	t.Run("a pull request's second failure is left alone", func(t *testing.T) {
		stub := newGHStub(t, nil)
		if out, code := stub.run(t, aftermathPath, runEnv("pull_request", "claude/x", "failure", "2")); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if got := stub.calls(t); len(got) != 0 {
			t.Errorf("calls %v; want none", got)
		}
	})

	t.Run("a refused re-run is a warning, not a failure", func(t *testing.T) {
		stub := newGHStub(t, []route{jobsOf(77, "Unit and domain tests", "CI required")})
		env := runEnv("pull_request", "dependabot/npm", "failure", "1")
		env["STUB_REFUSE"] = "run"
		out, code := stub.run(t, aftermathPath, env)
		if code != 0 || !strings.Contains(out, "::warning::") {
			t.Errorf("exit %d, want 0 with a warning:\n%s", code, out)
		}
	})

	t.Run("main's first failure: re-run, the issue opened, the green pull requests held", func(t *testing.T) {
		stub := newGHStub(t, with(openPulls, route{issuesArgs, `[]`}, redCommit, redJobs("1")))
		if out, code := stub.run(t, aftermathPath, runEnv("push", "main", "failure", "1")); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if len(stub.wrote(t, "run rerun 77 --failed")) != 1 {
			t.Errorf("main's first failure was not re-run: %v", stub.calls(t))
		}
		created := stub.wrote(t, "issue create")
		if len(created) != 1 {
			t.Fatalf("want one issue created, calls %v", stub.calls(t))
		}
		for _, want := range []string{"--title CI: main is red", "--label finding", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "(#1293)", "`Workspace (webapp)`", "Mends main: bbbbbbbbbbbb", "re-run once"} {
			if !strings.Contains(created[0], want) {
				t.Errorf("the issue does not carry %q:\n%s", want, created[0])
			}
		}
		if strings.Contains(created[0], "`Format, lint, generation`") {
			t.Error("the issue lists a job that passed")
		}
		if got := stub.wrote(t, "run rerun --job"); len(got) != 1 || got[0] != "run rerun --job 1070 --repo "+stubRepo {
			t.Errorf("holds %v; want only #7's CI required (job 1070), the one green pull request", got)
		}
	})

	t.Run("main's second failure: a closed issue is commented and reopened, nothing re-run but the hold", func(t *testing.T) {
		stub := newGHStub(t, with(openPulls,
			route{issuesArgs, `[{"number":40,"title":"CI: main is red (old)","state":"open"},{"number":41,"title":"CI: main is red","state":"closed"}]`},
			redCommit, redJobs("2")))
		if out, code := stub.run(t, aftermathPath, runEnv("push", "main", "failure", "2")); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if len(stub.wrote(t, "run rerun 77")) != 0 {
			t.Errorf("a second failure was re-run again: %v", stub.calls(t))
		}
		if len(stub.wrote(t, "issue comment 41 ")) != 1 || len(stub.wrote(t, "issue reopen 41 ")) != 1 || len(stub.wrote(t, "issue create")) != 0 {
			t.Errorf("want #41 commented and reopened, nothing created: %v", stub.calls(t))
		}
	})

	t.Run("a failure on another branch's push files nothing", func(t *testing.T) {
		stub := newGHStub(t, nil)
		if out, code := stub.run(t, aftermathPath, runEnv("push", "release", "failure", "1")); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if got := stub.calls(t); len(got) != 0 {
			t.Errorf("calls %v; want none", got)
		}
	})
}

func TestAGreenMainClosesTheIssueAndReRunsWhatWaitedForIt(t *testing.T) {
	routes := with(openPulls, mainRuns(mended, red), route{issuesArgs, `[{"number":41,"title":"CI: main is red","state":"open"}]`})

	for _, mode := range []map[string]string{runEnv("push", "main", "success", "1"), {"MODE": "sweep"}} {
		t.Run(mode["MODE"], func(t *testing.T) {
			stub := newGHStub(t, routes)
			if out, code := stub.run(t, aftermathPath, mode); code != 0 {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			if closed := stub.wrote(t, "issue close 41 "); len(closed) != 1 || !strings.Contains(closed[0], "dddddddddddd") {
				t.Errorf("#41 was not closed naming the green commit: %v", stub.calls(t))
			}
			if got := stub.wrote(t, "run "); len(got) != 1 || got[0] != "run rerun 101 --failed --repo "+stubRepo {
				t.Errorf("re-runs %v; want only #1's run 101, which failed at the guard alone", got)
			}
		})
	}

	t.Run("an issue already closed stays as it is", func(t *testing.T) {
		stub := newGHStub(t, with(openPulls, mainRuns(mended), route{issuesArgs, `[{"number":41,"title":"CI: main is red","state":"closed"}]`}))
		if out, code := stub.run(t, aftermathPath, map[string]string{"MODE": "sweep"}); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if got := stub.wrote(t, "issue"); len(got) != 0 {
			t.Errorf("issue writes %v; want none", got)
		}
	})

	t.Run("a sweep finding main red with no open issue files it and holds", func(t *testing.T) {
		stub := newGHStub(t, with(openPulls, mainRuns(mainRun(77, "completed", "failure", 2, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "2026-10-12T11:00:00Z"), green),
			route{issuesArgs, `[]`}, route{"api repos/" + stubRepo + "/actions/runs/77", `{"run_attempt":2}`}, redCommit, redJobs("2")))
		if out, code := stub.run(t, aftermathPath, map[string]string{"MODE": "sweep"}); code != 0 || !strings.Contains(out, "main is red") {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if len(stub.wrote(t, "issue create")) != 1 || len(stub.wrote(t, "run rerun --job 1070")) != 1 {
			t.Errorf("want the issue filed and #7 held: %v", stub.calls(t))
		}
	})

	t.Run("a sweep finding main red with the issue open only holds", func(t *testing.T) {
		stub := newGHStub(t, with(openPulls, mainRuns(red, green), route{issuesArgs, `[{"number":41,"title":"CI: main is red","state":"open"}]`}))
		if out, code := stub.run(t, aftermathPath, map[string]string{"MODE": "sweep"}); code != 0 {
			t.Fatalf("exit %d:\n%s", code, out)
		}
		if len(stub.wrote(t, "issue")) != 0 || len(stub.wrote(t, "run rerun --job 1070")) != 1 {
			t.Errorf("want no issue write and #7 held: %v", stub.calls(t))
		}
	})
}
