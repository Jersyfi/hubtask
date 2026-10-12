// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The two scripts that keep a merge without catching up safe (ADR-0083): the guard at the end of
// `CI required`, and what runs after every CI run. Both talk to GitHub only through `gh`, so they
// are run here against a stand-in `gh` on the PATH that answers recorded API shapes and logs every
// call - which is what proves that a write (a re-run, an issue) happens exactly when it should.

const (
	guardPath = "scripts/ci/main-green.sh"
	stubRepo  = "octo/hub"
)

// route is one answer of the stand-in: the exact arguments, and what it prints.
type route struct{ args, body string }

type ghStub struct {
	dir string
	log string
}

func newGHStub(t *testing.T, routes []route) ghStub {
	t.Helper()
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq is not installed; the scripts read GitHub's answers with it, as the runners do")
	}
	dir := t.TempDir()
	var table strings.Builder
	for i, r := range routes {
		file := fmt.Sprintf("answer-%d.json", i)
		if err := os.WriteFile(filepath.Join(dir, file), []byte(r.body), 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&table, "%s\t%s\n", r.args, file)
	}
	if err := os.WriteFile(filepath.Join(dir, "routes"), []byte(table.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	// Writes (issue, run) answer nothing and are only logged - refused when STUB_REFUSE names the
	// command; every read must have a route, so a script asking something unexpected fails loudly
	// instead of reading an empty answer.
	stub := `#!/usr/bin/env bash
dir="$(cd "$(dirname "$0")" && pwd)"
all="$*"
printf '%s\n' "${all//$'\n'/ }" >> "$dir/calls"
case "$1" in issue|run)
	if [[ "$1" == "${STUB_REFUSE:-}" ]]; then echo "HTTP 403: refused" >&2; exit 1; fi
	exit 0 ;;
esac
while IFS=$'\t' read -r args file; do
	if [[ "$*" == "$args" ]]; then cat "$dir/$file"; exit 0; fi
done < "$dir/routes"
echo "stub gh: no route for: $*" >&2
exit 1
`
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	return ghStub{dir: dir, log: filepath.Join(dir, "calls")}
}

// run executes a script with the stand-in first on the PATH and only the environment given.
func (s ghStub) run(t *testing.T, script string, env map[string]string, args ...string) (string, int) {
	t.Helper()
	abs, err := filepath.Abs("../../" + script)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{abs}, args...)...)
	cmd.Env = []string{"PATH=" + s.dir + string(os.PathListSeparator) + os.Getenv("PATH"), "HOME=" + s.dir, "GITHUB_REPOSITORY=" + stubRepo}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("%s did not run: %v", script, err)
	}
	return string(out), code
}

func (s ghStub) calls(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(s.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func (s ghStub) wrote(t *testing.T, prefix string) []string {
	t.Helper()
	var hits []string
	for _, c := range s.calls(t) {
		if strings.HasPrefix(c, prefix) {
			hits = append(hits, c)
		}
	}
	return hits
}

const mainRunsArgs = "api repos/" + stubRepo + "/actions/workflows/ci.yml/runs?branch=main&event=push&per_page=30"

// mainRuns is the list of runs on main, in whatever order GitHub lists them.
func mainRuns(runs ...string) route {
	return route{mainRunsArgs, `{"workflow_runs":[` + strings.Join(runs, ",") + `]}`}
}

func mainRun(id int, status, conclusion string, attempt int, sha, created string) string {
	c := "null"
	if conclusion != "" {
		c = fmt.Sprintf("%q", conclusion)
	}
	return fmt.Sprintf(`{"id":%d,"status":%q,"conclusion":%s,"run_attempt":%d,"head_sha":%q,"created_at":%q}`, id, status, c, attempt, sha, created)
}

func pull(n int, title, body string) route {
	return route{fmt.Sprintf("api repos/%s/pulls/%d", stubRepo, n), fmt.Sprintf(`{"number":%d,"title":%q,"body":%q}`, n, title, body)}
}

var (
	green     = mainRun(11, "completed", "success", 1, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "2026-10-12T10:00:00Z")
	red       = mainRun(12, "completed", "failure", 1, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "2026-10-12T11:00:00Z")
	gone      = mainRun(13, "completed", "cancelled", 1, "cccccccccccccccccccccccccccccccccccccccc", "2026-10-12T12:00:00Z")
	going     = mainRun(14, "in_progress", "", 1, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", "2026-10-12T12:30:00Z")
	rerunning = mainRun(15, "in_progress", "", 2, "ffffffffffffffffffffffffffffffffffffffff", "2026-10-12T12:40:00Z")
	timedOut  = mainRun(16, "completed", "timed_out", 1, "9999999999999999999999999999999999999999", "2026-10-12T12:50:00Z")
	mended    = mainRun(21, "completed", "success", 1, "dddddddddddddddddddddddddddddddddddddddd", "2026-10-12T13:00:00Z")
)

func TestTheGuardHoldsAPullRequestWhileMainIsRed(t *testing.T) {
	cases := []struct {
		name   string
		routes []route
		code   int
		says   string
	}{
		{"main green", []route{mainRuns(green)}, 0, "main is green at aaaaaaaaaaaa"},
		{"main red", []route{mainRuns(red, green), pull(5, "feat(x): something", "No issue: n/a")}, 1, "::error::main is red"},
		{"a cancelled newest run is passed over", []route{mainRuns(gone, green)}, 0, "main is green"},
		{"a cancelled run does not hide a red one", []route{mainRuns(gone, red, green), pull(5, "fix: y", "")}, 1, "bbbbbbbbbbbb"},
		{"the list's order is not trusted", []route{mainRuns(green, red), pull(5, "fix: y", "")}, 1, "main is red"},
		{"a first attempt still going is not known yet", []route{mainRuns(going, green)}, 0, "main is green"},
		{"a re-run still going is red", []route{mainRuns(rerunning, green), pull(5, "fix: y", "")}, 1, "ffffffffffff"},
		{"a timed-out run is red", []route{mainRuns(timedOut, green), pull(5, "fix: y", "")}, 1, "999999999999"},
		{"no deciding run", []route{mainRuns(going)}, 0, "::notice::no deciding run"},
		{"the API failing fails the guard", nil, 1, "no route"},
		{"a Conventional revert", []route{mainRuns(red), pull(5, "revert: feat(x): something", "")}, 0, "is a revert"},
		{"a scoped revert", []route{mainRuns(red), pull(5, "revert(identity)!: the lock", "")}, 0, "is a revert"},
		{"GitHub's revert button", []route{mainRuns(red), pull(5, `Revert "feat(x): something"`, "")}, 0, "is a revert"},
		{"a title that only starts like one", []route{mainRuns(red), pull(5, "Reviewing the gates", "")}, 1, "main is red"},
		{"a title merely mentioning revert", []route{mainRuns(red), pull(5, "fix: revert-proof parsing", "")}, 1, "main is red"},
		{"a fix forward that says so", []route{mainRuns(red), pull(5, "fix(webapp): one getter", "## What and why\r\n\r\nMends main: bbbbbbbbbbbb\r\n\r\nNo issue: main is red\r\n")}, 0, "says it mends main"},
		{"the line must stand alone", []route{mainRuns(red), pull(5, "fix: y", "This text mentions Mends main: bbbbbbb in passing")}, 1, "main is red"},
		{"the line names a commit and nothing else", []route{mainRuns(red), pull(5, "fix: y", "Mends main: bbbbbbb, and more besides")}, 1, "main is red"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stub := newGHStub(t, c.routes)
			out, code := stub.run(t, guardPath, map[string]string{"PR": "5"})
			if code != c.code || !strings.Contains(out, c.says) {
				t.Errorf("exit %d, want %d; output should say %q:\n%s", code, c.code, c.says, out)
			}
		})
	}
}

func TestTheGuardStatesMainsState(t *testing.T) {
	for _, c := range []struct {
		runs []string
		want string
	}{
		{[]string{green}, "green aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa 11"},
		{[]string{red, green}, "red bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb 12"},
		{nil, "unknown"},
	} {
		stub := newGHStub(t, []route{mainRuns(c.runs...)})
		out, code := stub.run(t, guardPath, nil, "--state")
		if code != 0 || strings.TrimSpace(out) != c.want {
			t.Errorf("--state with %d runs: exit %d, %q; want %q", len(c.runs), code, out, c.want)
		}
	}
}
