// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// What the weekly job files (ci-cd.md §11): the report as one comment on the issue "Flow report",
// and one `finding` per measure over its threshold. The repository is public, so a title alone
// identifies nothing: an issue is the job's only when the job's login opened it, and a finding
// only when it also carries the label.

const (
	reportTitle  = "Flow report"
	findingLabel = "finding"
	// commentLimit is the length GitHub takes for a comment, in characters.
	commentLimit = 65536
)

// issue is an issue as the search answers it.
type issue struct {
	Number int
	Title  string
	State  string
	User   struct{ Login string }
	Labels []struct{ Name string }
}

func (i issue) labelled(name string) bool {
	for _, l := range i.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

// tracker is what filing does to the repository's issues.
type tracker interface {
	create(title, body string, labels ...string) (int, error)
	comment(number int, body string) error
	setState(number int, state string) error
}

// own finds the job's issue of a title: the oldest one its login opened, with the label where
// one is asked for. Anybody else's issue of the same title is not looked at.
func own(issues []issue, filer, title, label string) (issue, bool) {
	var found []issue
	for _, i := range issues {
		if i.Title == title && i.User.Login == filer && (label == "" || i.labelled(label)) {
			found = append(found, i)
		}
	}
	if len(found) == 0 {
		return issue{}, false
	}
	sort.Slice(found, func(a, b int) bool { return found[a].Number < found[b].Number })
	return found[0], true
}

// file posts the week. A breach without a finding opens one. A breach whose finding is open adds
// nothing: the weekly comment carries the value. A breach whose finding is closed opens it again
// with one comment. A measure back within its threshold closes its open finding with one comment.
// The findings come first, so a report too long to post does not keep them back.
func file(t tracker, issues []issue, filer, report string, breached []finding, within []standing, run []string) error {
	thread, known := own(issues, filer, reportTitle, "")
	if !known {
		number, err := t.create(reportTitle, strings.Join([]string{
			"The weekly flow report (`docs/architecture/ci-cd.md` §11): each week is one comment.",
			"",
			"It starts no work. A measure over its threshold gets a `finding` of its own.",
		}, "\n"))
		if err != nil {
			return err
		}
		thread = issue{Number: number, State: "open"}
	}
	trailer := append([]string{""}, run...)
	trailer = append(trailer, "* Report: #"+strconv.Itoa(thread.Number))
	for _, b := range breached {
		body := b.Body + "\n" + strings.Join(trailer, "\n")
		existing, ok := own(issues, filer, b.Title, findingLabel)
		switch {
		case !ok:
			if _, err := t.create(b.Title, body, findingLabel); err != nil {
				return err
			}
		case existing.State == "closed":
			if err := t.comment(existing.Number, body); err != nil {
				return err
			}
			if err := t.setState(existing.Number, "open"); err != nil {
				return err
			}
		}
	}
	for _, s := range within {
		existing, ok := own(issues, filer, s.Title, findingLabel)
		if !ok || existing.State != "open" {
			continue
		}
		if err := t.comment(existing.Number, "Within the threshold again: "+s.Value+"\n"+strings.Join(trailer, "\n")); err != nil {
			return err
		}
		if err := t.setState(existing.Number, "closed"); err != nil {
			return err
		}
	}
	comment := report + strings.Join(run, "\n")
	if n := utf8.RuneCountInString(comment); n > commentLimit {
		return fmt.Errorf("the report is %d characters and a comment takes %d: the findings are filed, the report is not posted", n, commentLimit)
	}
	if err := t.comment(thread.Number, comment); err != nil {
		return err
	}
	if thread.State == "closed" {
		return t.setState(thread.Number, "open")
	}
	return nil
}

// fileWeek reads the job's issues and files the week through the gh CLI.
func fileWeek(repo, filer, report string, breached []finding, within []standing) error {
	issues, err := issuesTitled(repo, filer)
	if err != nil {
		return fmt.Errorf("the job's issues: %w", err)
	}
	return file(github{repo: repo}, issues, filer, report, breached, within, runLines())
}

// runLines name the run that filed, where the job's environment says which it is.
func runLines() []string {
	server, repository, id, sha := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_SHA")
	if server == "" || repository == "" || id == "" {
		return nil
	}
	return []string{"* Run: " + server + "/" + repository + "/actions/runs/" + id, "* Commit: " + sha}
}

// issuesTitled are the issues whose title holds the report's, which every finding's title starts
// with. The search narrows by author; own holds each hit to the login and the title in full.
func issuesTitled(repo, filer string) ([]issue, error) {
	author := filer
	if app, ok := strings.CutSuffix(filer, "[bot]"); ok {
		author = "app/" + app
	}
	out, err := gh("api", "--paginate", "-X", "GET", "search/issues",
		"-f", fmt.Sprintf("q=repo:%s is:issue in:title %q author:%s", repo, reportTitle, author), "-f", "per_page=100")
	if err != nil {
		return nil, err
	}
	return parseIssues(out)
}

func parseIssues(data []byte) ([]issue, error) {
	var issues []issue
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var page struct{ Items []issue }
		if err := dec.Decode(&page); err != nil {
			return nil, err
		}
		issues = append(issues, page.Items...)
	}
	return issues, nil
}

// github files through the gh CLI, which carries the job's token.
type github struct{ repo string }

func (g github) create(title, body string, labels ...string) (int, error) {
	args := []string{"api", "-X", "POST", "repos/" + g.repo + "/issues", "-f", "title=" + title, "-f", "body=" + body}
	for _, l := range labels {
		args = append(args, "-f", "labels[]="+l)
	}
	out, err := ghOnce(args...)
	if err != nil {
		return 0, err
	}
	var created struct{ Number int }
	if err := json.Unmarshal(out, &created); err != nil {
		return 0, err
	}
	return created.Number, nil
}

func (g github) comment(number int, body string) error {
	_, err := ghOnce("api", "-X", "POST", fmt.Sprintf("repos/%s/issues/%d/comments", g.repo, number), "-f", "body="+body)
	return err
}

func (g github) setState(number int, state string) error {
	_, err := ghOnce("api", "-X", "PATCH", fmt.Sprintf("repos/%s/issues/%d", g.repo, number), "-f", "state="+state)
	return err
}
