// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command apicompat holds the API contract to the last release (`make gate-api-compat`,
// versioning-release.md §6).
//
// It compares `api/openapi.yaml` in the working tree with the same file at the newest `v*` tag
// behind HEAD, through oasdiff, and classifies what changed by versioning-release.md §2. A breaking
// change fails unless it is marked: a Conventional Commit title with `!` before the colon, or a
// `BREAKING CHANGE:` footer.
//
// With -base, only the breaking changes this branch adds are judged, and only this branch's commits
// (and -text, the pull request's title and description) can mark them: one marked break since the
// tag must not carry every later one through unmarked. A break already on the base was judged when
// it landed there. Without -base, every break since the tag is judged against every commit since it.
//
// Usage: apicompat [-since <ref>] [-base <ref>] [-text <file>]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	specPath    = "api/openapi.yaml"
	oasdiffPath = ".tools/oasdiff"

	gitTimeout     = 30 * time.Second
	oasdiffTimeout = 2 * time.Minute
)

func main() {
	since := flag.String("since", "", "what the contract is compared with; default: the newest v* tag behind HEAD")
	base := flag.String("base", "", "the commit the branch starts from; only breaks it adds are judged, only its commits mark them")
	text := flag.String("text", "", "a file with the pull request's title (first line) and description, which may mark a break too")
	flag.Parse()

	ctx := context.Background()
	if err := run(ctx, *since, *base, *text); err != nil {
		fmt.Fprintf(os.Stderr, "api-compat: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, since, base, textFile string) error {
	if since == "" {
		shallow, err := git(ctx, "rev-parse", "--is-shallow-repository")
		if err != nil {
			return err
		}
		if shallow == "true" {
			return errors.New("the checkout is shallow, so the last release cannot be found - fetch the whole history (fetch-depth: 0)")
		}
		tag, err := git(ctx, "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*", "HEAD")
		if err != nil {
			fmt.Println("api-compat: skipped - no release tag behind HEAD, so no released contract to compare with")
			return nil
		}
		since = tag
	}

	sinceBreaks, warnings, err := breaksAgainst(ctx, since)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		fmt.Printf("  warning   %s\n", w)
	}

	judged, marking := sinceBreaks, since+"..HEAD"
	if base != "" {
		baseBreaks, _, err := breaksAgainstAt(ctx, since, base)
		if err != nil {
			return err
		}
		judged, marking = added(sinceBreaks, baseBreaks), base+"..HEAD"
	}
	if len(judged) == 0 {
		fmt.Printf("api-compat: no breaking change against %s to judge (%d already on the base)\n", since, len(sinceBreaks))
		return nil
	}

	messages, err := commitMessages(ctx, marking)
	if err != nil {
		return err
	}
	var title, body string
	if textFile != "" {
		raw, err := os.ReadFile(textFile) //nolint:gosec // G304: a file the caller names, read as text
		if err != nil {
			return err
		}
		title, body, _ = strings.Cut(string(raw), "\n")
	}

	fmt.Printf("api-compat: %d breaking change(s) against %s:\n", len(judged), since)
	for _, b := range judged {
		fmt.Printf("  %s\n", b)
	}
	if by := markedBy(messages, title, body); by != "" {
		fmt.Printf("api-compat: marked as breaking by %s\n", by)
		return nil
	}
	return fmt.Errorf("not marked as breaking (versioning-release.md §2, §3) - make the change compatible, "+
		"or mark it: `!` before the colon of the commit's and the pull request's title and a "+
		"`BREAKING CHANGE: <what breaks, and the way across>` footer; renaming or removing a field is the "+
		"owner's decision. Commits read: %s", marking)
}

// severityLevels is where oasdiff's defaults part from versioning-release.md §2, in oasdiff's
// levels file format (one `rule-id level` per line, no comments - hence here). A new value in a
// response enum is MINOR there, because clients tolerate unknown values (api-guidelines.md §1);
// oasdiff calls it an error.
const severityLevels = "response-property-enum-value-added info\n"

// breaksAgainst compares the working tree's contract with the contract at ref.
func breaksAgainst(ctx context.Context, ref string) ([]change, []change, error) {
	return compare(ctx, ref, specPath)
}

// breaksAgainstAt compares the contract at ref with the contract at the commit at.
func breaksAgainstAt(ctx context.Context, ref, at string) ([]change, []change, error) {
	dir, err := os.MkdirTemp("", "apicompat-")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	revision := filepath.Join(dir, "revision.yaml")
	if err := writeAt(ctx, at, revision); err != nil {
		return nil, nil, err
	}
	return compare(ctx, ref, revision)
}

func compare(ctx context.Context, ref, revision string) ([]change, []change, error) {
	dir, err := os.MkdirTemp("", "apicompat-")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	released := filepath.Join(dir, "released.yaml")
	if err := writeAt(ctx, ref, released); err != nil {
		return nil, nil, err
	}
	levels := filepath.Join(dir, "levels.txt")
	if err := os.WriteFile(levels, []byte(severityLevels), 0o600); err != nil {
		return nil, nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, oasdiffTimeout)
	defer cancel()
	var stderr bytes.Buffer
	// External references are refused: the contract has none, and following one would let the
	// comparison reach the network.
	cmd := exec.CommandContext(runCtx, oasdiffPath, "breaking", released, revision, //nolint:gosec // G204: fixed arguments, paths this program wrote
		"--format", "json", "--severity-levels", levels, "--allow-external-refs=false")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("oasdiff: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parse(out)
}

func writeAt(ctx context.Context, ref, to string) error {
	spec, err := gitRaw(ctx, "show", ref+":"+specPath)
	if err != nil {
		return err
	}
	return os.WriteFile(to, spec, 0o600)
}

func commitMessages(ctx context.Context, revisions string) ([]string, error) {
	raw, err := gitRaw(ctx, "log", "--format=%B%x00", revisions)
	if err != nil {
		return nil, err
	}
	var messages []string
	for _, m := range strings.Split(string(raw), "\x00") {
		if m = strings.TrimSpace(m); m != "" {
			messages = append(messages, m)
		}
	}
	return messages, nil
}

func git(ctx context.Context, args ...string) (string, error) {
	out, err := gitRaw(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func gitRaw(ctx context.Context, args ...string) ([]byte, error) {
	gitCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(gitCtx, "git", args...) //nolint:gosec // G204: fixed git subcommands
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// change is one entry of oasdiff's JSON report.
type change struct {
	ID          string `json:"id"`
	Text        string `json:"text"`
	Level       int    `json:"level"`
	Operation   string `json:"operation"`
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

func (c change) String() string {
	return fmt.Sprintf("%s %s: %s [%s]", c.Operation, c.Path, c.Text, c.ID)
}

// oasdiff's levels: 1 info, 2 warning, 3 error.
const (
	levelWarning = 2
	levelError   = 3
)

// parse splits the report into breaking changes and warnings. A warning is a change oasdiff cannot
// classify alone; it is shown, and judged by whoever reads it.
func parse(report []byte) ([]change, []change, error) {
	var all []change
	if err := json.Unmarshal(report, &all); err != nil {
		return nil, nil, fmt.Errorf("oasdiff's report does not parse: %w", err)
	}
	var breaking, warnings []change
	for _, c := range all {
		switch {
		case c.Level >= levelError:
			breaking = append(breaking, c)
		case c.Level == levelWarning:
			warnings = append(warnings, c)
		}
	}
	return breaking, warnings, nil
}
