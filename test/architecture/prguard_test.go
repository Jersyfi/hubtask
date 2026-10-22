// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The hook is what makes ADR-0079 a rule rather than a paragraph: a session cannot open a pull
// request ready, and cannot mark one ready before the commit is the one GitHub has. Proven here on every run, against a real git repository with a real remote, because each
// refusal depends on git's answer.

const guardScript = "scripts/hooks/pr-transition-guard.sh"

func TestTheSettingsWireTheGuardToEveryBashCall(t *testing.T) {
	raw, err := os.ReadFile("../../.claude/settings.json")
	if err != nil {
		t.Fatalf(".claude/settings.json is not readable - without it no session runs the hook: %v", err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf(".claude/settings.json does not parse: %v", err)
	}
	for _, entry := range settings.Hooks["PreToolUse"] {
		for _, hook := range entry.Hooks {
			if entry.Matcher == "Bash" && hook.Type == "command" && strings.HasSuffix(hook.Command, guardScript) {
				return
			}
		}
	}
	t.Errorf(".claude/settings.json has no PreToolUse hook on Bash running %s (ADR-0079)", guardScript)
}

func TestTheGuardRefusesWhatADraftMayNotSkip(t *testing.T) {
	script, err := filepath.Abs("../../" + guardScript)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed; the hook reads its input with it")
	}

	dir := t.TempDir()
	remote := filepath.Join(dir, "remote.git")
	work := filepath.Join(dir, "work")
	gitIn(t, dir, "init", "-q", "--bare", remote)
	gitIn(t, dir, "init", "-q", "-b", "main", work)
	gitIn(t, work, "-c", "user.email=t@example.invalid", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "one")
	gitIn(t, work, "remote", "add", "origin", remote)
	gitIn(t, work, "push", "-q", "-u", "origin", "main")

	cases := []struct {
		name, command string
		prepare       func()
		refused       bool
	}{
		{"an unrelated command", "go test ./...", nil, false},
		{"a pull request opened ready", "gh pr create --title x --body-file b.md", nil, true},
		{"a pull request opened as a draft", "gh pr create --draft --title x", nil, false},
		{"a pull request opened as a draft, short flag", "gh pr create -d --title x", nil, false},
		{"ready with HEAD pushed", "gh pr ready 12", nil, false},
		{"ready back to draft", "gh pr ready --undo 12", nil, false},
		{"text that only mentions the command", `echo "gh pr readyness"`, nil, false},
		{"ready inside a compound command", "make check && gh pr ready", nil, false},
		{"ready with HEAD not pushed", "gh pr ready", func() {
			gitIn(t, work, "-c", "user.email=t@example.invalid", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "two")
		}, true},
	}
	for _, c := range cases {
		if c.prepare != nil {
			c.prepare()
		}
		refused, message := runGuard(t, script, work, c.command)
		if refused != c.refused {
			t.Errorf("%s (%q): refused=%v, want %v; the hook said %q", c.name, c.command, refused, c.refused, message)
		}
		if refused && !strings.Contains(message, "Refused") {
			t.Errorf("%s: the refusal does not say what to do: %q", c.name, message)
		}
	}
}

func runGuard(t *testing.T, script, dir, command string) (bool, string) {
	t.Helper()

	// The hook is started elsewhere and told the worker's directory, as Claude Code does for a
	// session in its own worktree: the HEAD it judges is the named checkout's.
	payload, err := json.Marshal(map[string]any{"tool_name": "Bash", "cwd": dir, "tool_input": map[string]string{"command": command}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.Dir = t.TempDir()
	cmd.Env = withoutGitVariables()
	cmd.Stdin = strings.NewReader(string(payload))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()

	var exit *exec.ExitError
	switch {
	case err == nil:
		return false, stderr.String()
	case errors.As(err, &exit) && exit.ExitCode() == 2:
		return true, stderr.String()
	default:
		t.Fatalf("the hook failed rather than deciding (%v): %s", err, stderr.String())
		return false, ""
	}
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = withoutGitVariables()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// withoutGitVariables is the caller's environment without GIT_*: under a git hook or `git rebase
// --exec`, GIT_DIR names the real repository, and a scratch `git init --bare` would turn it bare
// (known-traps.md, "Tests, gates and tooling").
func withoutGitVariables() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	return env
}
