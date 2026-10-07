// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command checkpr holds a pull request description to the template (the job `Pull request
// description` in ci.yml, part of `CI required`; `make gate-pr BODY=<file>` locally).
//
// A description is where the review, the use cases' checks and the Definition of Done are recorded,
// and the template is how AGENTS.md asks for them. It was skipped anyway, by a session that wrote
// descriptions from scratch with `gh pr create --body`: GitHub shows the template only to somebody
// who opens the form. This reads the description the way a reviewer would and names what is missing.
//
// Usage: checkpr -body <file> [-opened <RFC 3339>] [-base <ref> [-head <ref>]]
// (or the description on stdin). The Definition of Done is compared with the template's (dod.go).
// With -base it also reads the branch's history: the readiness record of every task it carries,
// merged migrations it changes, use cases it deletes (readiness.go). A rule added after a pull
// request was opened (-opened) does not apply to it.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func main() {
	bodyFile := flag.String("body", "", "the pull request description; stdin when empty")
	base := flag.String("base", "", "the pull request's base; empty reads no history")
	head := flag.String("head", "HEAD", "the pull request's head")
	openedAt := flag.String("opened", "", "when the pull request was opened, RFC 3339")
	flag.Parse()

	root, err := repositoryRoot()
	if err != nil {
		fail(err)
	}

	var raw []byte
	if *bodyFile == "" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(*bodyFile)
	}
	if err != nil {
		fail(fmt.Errorf("reading the description: %w", err))
	}

	required, err := templateHeadings(root)
	if err != nil {
		fail(err)
	}
	useCases, err := knownUseCases(root)
	if err != nil {
		fail(err)
	}

	dod, err := templateDoD(root)
	if err != nil {
		fail(err)
	}

	var opened time.Time
	if *openedAt != "" {
		if opened, err = time.Parse(time.RFC3339, *openedAt); err != nil {
			fail(fmt.Errorf("reading -opened: %w", err))
		}
	}
	var facts branchFacts
	if *base != "" {
		if facts, err = readHistory(root, *base, *head, opened); err != nil {
			fail(err)
		}
	}

	problems := check(string(raw), required, useCases)
	if heldTo(opened, dodSince) {
		problems = append(problems, dodProblems(string(raw), dod)...)
	}
	problems = append(problems, historyProblems(string(raw), facts, func(path string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) //nolint:gosec // G304: docs/backlog/ready/<TASK>.md, the task taken from the branch's own trailers
	})...)
	if len(problems) == 0 {
		fmt.Println("pull request description: every section of the template is there and filled")
		return
	}
	for _, problem := range problems {
		fmt.Fprintln(os.Stderr, "- "+problem)
	}
	fmt.Fprintf(os.Stderr, "\n%d problem(s) in the pull request description; the template is .github/PULL_REQUEST_TEMPLATE.md\n", len(problems))
	os.Exit(1)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}

// repositoryRoot finds go.mod upwards, so the command runs from anywhere in the tree.
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
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}
