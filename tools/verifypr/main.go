// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command verifypr is the pull request check on a laptop (`make verify-pr`, ADR-0079).
//
// A draft is checked in the session that writes it, and CI runs once the pull request is ready. So
// before `gh pr ready`, this runs `make verify` and then every gate CI would run for this branch,
// selected by the same filters `ci.yml` selects them by (tools/cilocal), and the description
// against the template. On success it writes a stamp naming the commit it checked into the
// checkout's git directory; the Claude Code hook refuses `gh pr ready` without one for `HEAD`.
//
// The container gates take turns across every worktree on the machine, under a lock in the common
// git directory: their compose projects and host ports are fixed. When Docker does not answer they
// are reported as not run - CI runs them - rather than failing a session that cannot run them.
//
// Usage: verifypr [-base origin/main] [-body <file>] [-plan]
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Jersyfi/hubtask/tools/cilocal"
)

const (
	// StampName is the file in the checkout's git directory the hook reads (scripts/hooks).
	stampName = "hubtask-verify-pr"
	lockName  = "hubtask-container-gates.lock"

	stepTimeout = 45 * time.Minute
	gitTimeout  = 30 * time.Second
	dockerProbe = 15 * time.Second
	lockWait    = 30 * time.Minute
	lockPoll    = 15 * time.Second
)

type step struct {
	name      string
	command   string
	container bool
}

func main() {
	os.Exit(run())
}

func run() int {
	base := flag.String("base", "origin/main", "what the branch is compared with, as CI compares a pull request with its base")
	body := flag.String("body", "", "the pull request description; default: read from the pull request with gh")
	plan := flag.Bool("plan", false, "print what would run and run nothing")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := check(ctx, *base, *body, *plan); err != nil {
		fmt.Fprintf(os.Stderr, "\nverify-pr: %v\n", err)
		return 1
	}
	return 0
}

func check(ctx context.Context, base, body string, plan bool) error {
	root, err := git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	gitDir, err := git(ctx, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}

	if !plan {
		if dirty, err := git(ctx, "status", "--porcelain"); err != nil {
			return err
		} else if dirty != "" {
			return fmt.Errorf("the tree has uncommitted changes - the stamp names a commit, so commit first:\n%s", dirty)
		}
	}

	changed, err := git(ctx, "diff", "--name-only", base+"...HEAD")
	if err != nil {
		return fmt.Errorf("comparing with %s: %w (git fetch origin, or pass -base)", base, err)
	}
	files := strings.Fields(changed)

	workflow, err := os.ReadFile(cilocal.WorkflowPath)
	if err != nil {
		return err
	}
	filters, err := cilocal.LoadFilters(workflow)
	if err != nil {
		return err
	}
	touched := cilocal.Touched(filters, files)
	steps, notes := selectSteps(touched)

	fmt.Printf("verify-pr: %d files changed against %s; filters set: %s\n", len(files), base, setNames(touched))
	for _, s := range steps {
		fmt.Printf("  will run  %s\n", s.name)
	}
	for _, note := range notes {
		fmt.Printf("  CI only   %s\n", note)
	}
	fmt.Printf("  will run  the description (make gate-pr)\n")
	if plan {
		return nil
	}

	head, err := git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	stamp := filepath.Join(gitDir, stampName)
	_ = os.Remove(stamp) // a run that does not finish leaves no stamp, not an old one
	logs := filepath.Join(gitDir, "verify-pr")
	if err := os.MkdirAll(logs, 0o750); err != nil {
		return err
	}

	report := []string{}
	release := func() {}
	defer func() { release() }()
	dockerChecked, dockerUp := false, false

	for i, s := range steps {
		if s.container {
			if !dockerChecked {
				dockerChecked, dockerUp = true, dockerAnswers(ctx)
				if dockerUp {
					commonDir, err := git(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir")
					if err != nil {
						return err
					}
					if release, err = acquireLock(ctx, filepath.Join(commonDir, lockName), root); err != nil {
						return err
					}
				}
			}
			if !dockerUp {
				line := fmt.Sprintf("  not run   %-36s Docker did not answer - CI will run it", s.name)
				fmt.Println(line)
				report = append(report, line)
				continue
			}
		}
		line, err := runStep(ctx, s, filepath.Join(logs, fmt.Sprintf("%02d-%s.log", i, slug(s.name))))
		report = append(report, line)
		if err != nil {
			return err
		}
	}
	release()
	release = func() {}

	line, err := checkDescription(ctx, body, filepath.Join(logs, "description.md"), filepath.Join(logs, "zz-description.log"))
	report = append(report, line)
	if err != nil {
		return err
	}

	// A gate that rewrote a tracked file has found a diff - generated code, tokens - even when its
	// own exit status was zero. The stamp names a commit, and this tree is no longer that commit.
	if dirty, err := git(ctx, "status", "--porcelain"); err != nil {
		return err
	} else if dirty != "" {
		return fmt.Errorf("the gates left the tree changed - a generator found a diff to commit:\n%s", dirty)
	}
	if now, err := git(ctx, "rev-parse", "HEAD"); err != nil || now != head {
		return errors.New("HEAD moved while the gates ran - run verify-pr again")
	}

	content := head + "\n" + time.Now().UTC().Format(time.RFC3339) + "\n" + strings.Join(report, "\n") + "\n"
	if err := os.WriteFile(stamp, []byte(content), 0o600); err != nil {
		return err
	}
	fmt.Printf("\nverify-pr: green for %s - gh pr ready may follow, once the commit is pushed\n", head[:12])
	return nil
}

// selectSteps is the plan: `make tools-ensure` and `make verify` always, then every step of every
// job the changed filters start, in the order of cilocal.Jobs. The notes name the jobs that start
// and only CI can run.
func selectSteps(touched map[string]bool) ([]step, []string) {
	steps := []step{
		{name: "tools", command: "make tools-ensure"},
		{name: "make verify", command: "make verify"},
	}
	var notes []string
	for _, job := range cilocal.Jobs {
		if !cilocal.Runs(job.Filters, touched) {
			continue
		}
		if job.CIOnly != "" {
			notes = append(notes, job.ID+": "+job.CIOnly)
			continue
		}
		for _, s := range job.Steps {
			filters := s.Filters
			if len(filters) == 0 {
				filters = job.Filters
			}
			if cilocal.Runs(filters, touched) {
				steps = append(steps, step{name: s.Name, command: s.Command, container: job.Container})
			}
		}
	}
	return steps, notes
}

func runStep(ctx context.Context, s step, logPath string) (string, error) {
	started := time.Now()
	stepCtx, cancel := context.WithTimeout(ctx, stepTimeout)
	defer cancel()

	logFile, err := os.Create(logPath) //nolint:gosec // G304: a log under this checkout's git directory
	if err != nil {
		return "", err
	}
	defer func() { _ = logFile.Close() }()

	cmd := exec.CommandContext(stepCtx, "bash", "-c", s.command) //nolint:gosec // G204: the commands are tools/cilocal's table, not input
	cmd.Env = withTools(os.Environ())
	cmd.Stdout, cmd.Stderr = logFile, logFile
	runErr := cmd.Run()

	elapsed := time.Since(started).Round(time.Second)
	if runErr == nil {
		line := fmt.Sprintf("  ok        %-36s %s", s.name, elapsed)
		fmt.Println(line)
		return line, nil
	}
	line := fmt.Sprintf("  FAILED    %-36s %s", s.name, elapsed)
	fmt.Println(line)
	fmt.Printf("            log: %s\n%s", logPath, tail(logPath, 30))
	return line, fmt.Errorf("%s failed (%w) - fix it on the draft and run make verify-pr again", s.name, runErr)
}

// checkDescription runs the template check against the pull request's description: the job CI
// runs on every pull request that is ready, and the one a session forgets because it lives on
// GitHub rather than in the tree.
func checkDescription(ctx context.Context, body, fetched, logPath string) (string, error) {
	if body == "" {
		lookupCtx, cancel := context.WithTimeout(ctx, gitTimeout)
		defer cancel()
		out, err := exec.CommandContext(lookupCtx, "gh", "pr", "view", "--json", "body", "--jq", ".body").Output()
		if err != nil {
			return "", fmt.Errorf("the description could not be read with gh (%w) - pass BODY=<file>", err)
		}
		if err := os.WriteFile(fetched, out, 0o600); err != nil {
			return "", err
		}
		body = fetched
	}
	// The title is checked when it can be had: TITLE from the caller, or the pull request's own. A
	// branch without a pull request yet has no title, and CI checks it once there is one.
	if os.Getenv("TITLE") == "" {
		lookupCtx, cancel := context.WithTimeout(ctx, gitTimeout)
		defer cancel()
		if out, err := exec.CommandContext(lookupCtx, "gh", "pr", "view", "--json", "title", "--jq", ".title").Output(); err == nil {
			if err := os.Setenv("TITLE", strings.TrimSpace(string(out))); err != nil {
				return "", err
			}
		}
	}
	return runStep(ctx, step{name: "the description (gate-pr)", command: "make gate-pr BASE=origin/main BODY=" + strconv.Quote(body)}, logPath)
}

func dockerAnswers(ctx context.Context) bool {
	probeCtx, cancel := context.WithTimeout(ctx, dockerProbe)
	defer cancel()
	return exec.CommandContext(probeCtx, "docker", "info").Run() == nil
}

// acquireLock takes the machine-wide container lock: a directory, because creating one is atomic.
// A lock whose owner process is gone is taken over; one whose owner is alive is waited for.
func acquireLock(ctx context.Context, dir, root string) (func(), error) {
	deadline := time.Now().Add(lockWait)
	announced := false
	for {
		err := os.Mkdir(dir, 0o750)
		if err == nil {
			owner := fmt.Sprintf("%d\n%s\n%s\n", os.Getpid(), root, time.Now().UTC().Format(time.RFC3339))
			if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(owner), 0o600); err != nil {
				_ = os.RemoveAll(dir)
				return nil, err
			}
			return func() { _ = os.RemoveAll(dir) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}

		pid, holder := lockOwner(dir)
		if pid > 0 && !alive(pid) {
			fmt.Printf("  the container lock of %s (pid %d) is stale - taking it over\n", holder, pid)
			_ = os.RemoveAll(dir)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("the container gates are still held by %s after %s - try again later", holder, lockWait)
		}
		if !announced {
			fmt.Printf("  waiting for the container gates: %s holds them\n", holder)
			announced = true
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(lockPoll):
		}
	}
}

func lockOwner(dir string) (int, string) {
	raw, err := os.ReadFile(filepath.Join(dir, "owner")) //nolint:gosec // G304: the lock this program creates
	if err != nil {
		return 0, "another session"
	}
	lines := strings.Split(string(raw), "\n")
	pid, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	holder := "another session"
	if len(lines) > 1 && lines[1] != "" {
		holder = lines[1]
	}
	return pid, holder
}

// alive asks whether a process exists without signalling it. EPERM means it exists and belongs to
// somebody else, which is still a holder.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func git(ctx context.Context, args ...string) (string, error) {
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(gitCtx, "git", args...) //nolint:gosec // G204: fixed git subcommands
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}

// withTools puts the pinned tools first on the PATH, the way every CI job does.
func withTools(env []string) []string {
	tools, _ := filepath.Abs(".tools")
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = "PATH=" + tools + string(os.PathListSeparator) + strings.TrimPrefix(kv, "PATH=")
			return env
		}
	}
	return append(env, "PATH="+tools)
}

func setNames(touched map[string]bool) string {
	var names []string
	for name := range touched {
		names = append(names, name)
	}
	if len(names) == 0 {
		return "none"
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func slug(name string) string {
	return strings.Trim(nonWord.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

func tail(path string, lines int) string {
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the step log written above
	if err != nil {
		return ""
	}
	all := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	return "            " + strings.Join(all, "\n            ") + "\n"
}
