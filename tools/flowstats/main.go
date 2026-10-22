// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Command flowstats measures how work flows through the repository over a window of days
// (`make flowstats`, ADR-0089): how long a pull request takes from its first commit to the merge,
// what CI costs in wall clock, and - on a machine that keeps Claude Code transcripts - what a
// session spends waiting and how many tokens it reads. It prints one Markdown report, so two windows
// are compared like with like, by the same code.
//
// The GitHub part reads through the gh CLI. The transcript part reads the JSON lines Claude Code
// writes under ~/.claude/projects, only the directories whose name contains the repository's name;
// it reads usage figures, timestamps and the commands a session ran, and prints no content.
//
// Usage: flowstats -since 2026-10-13 -until 2026-10-22 [-transcripts <dir>] [-github=false]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const callTimeout = 2 * time.Minute

func main() {
	since := flag.String("since", "", "first day of the window, YYYY-MM-DD (UTC)")
	until := flag.String("until", "", "last day of the window, YYYY-MM-DD (UTC), included")
	repo := flag.String("repo", "Jersyfi/hubtask", "the GitHub repository")
	transcripts := flag.String("transcripts", defaultTranscripts(), "Claude Code's projects directory; empty skips the transcripts")
	github := flag.Bool("github", true, "read pull requests and CI runs from GitHub")
	flag.Parse()

	from, err := time.Parse("2006-01-02", *since)
	if err != nil {
		fail("-since: %v", err)
	}
	to, err := time.Parse("2006-01-02", *until)
	if err != nil {
		fail("-until: %v", err)
	}
	w := window{from: from, to: to.AddDate(0, 0, 1)}

	fmt.Printf("## Flow %s … %s\n\n", *since, *until)
	if *github {
		prs, err := mergedPullRequests(*repo, w)
		if err != nil {
			fail("pull requests: %v", err)
		}
		printPullRequests(prs)
		runs, err := ciRuns(*repo, w)
		if err != nil {
			fail("CI runs: %v", err)
		}
		printRuns(runs)
	}
	if *transcripts != "" {
		s, err := readTranscripts(*transcripts, filepath.Base(*repo), w)
		if err != nil {
			fail("transcripts: %v", err)
		}
		printSessions(s)
	}
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "flowstats: "+format+"\n", a...)
	os.Exit(1)
}

func defaultTranscripts() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	return dir
}

type window struct{ from, to time.Time }

func (w window) has(t time.Time) bool { return !t.Before(w.from) && t.Before(w.to) }

func gh(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...) //nolint:gosec // G204: gh with arguments this tool builds
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(2, len(args))], " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// ----------------------------------------------------------------------------- pull requests

// pullRequest is a merged pull request, reduced to the moments the report measures.
type pullRequest struct {
	kind        string // feat, fix or other, from the Conventional Commit title
	firstCommit time.Time
	opened      time.Time
	ready       time.Time
	merged      time.Time
}

const prQuery = `query($q: String!, $after: String) {
  search(query: $q, type: ISSUE, first: 25, after: $after) {
    pageInfo { hasNextPage endCursor }
    nodes { ... on PullRequest {
      title createdAt mergedAt headRefName
      commits(first: 100) { nodes { commit { authoredDate } } }
      timelineItems(itemTypes: [READY_FOR_REVIEW_EVENT], last: 1) { nodes { ... on ReadyForReviewEvent { createdAt } } }
    } }
  }
}`

type prPage struct {
	Data struct {
		Search struct {
			PageInfo struct {
				HasNextPage bool
				EndCursor   string
			}
			Nodes []struct {
				Title       string
				CreatedAt   time.Time
				MergedAt    time.Time
				HeadRefName string
				Commits     struct {
					Nodes []struct {
						Commit struct{ AuthoredDate time.Time }
					}
				}
				TimelineItems struct {
					Nodes []struct{ CreatedAt time.Time }
				}
			}
		}
	}
}

var titleKind = regexp.MustCompile(`^(\w+)`)

func mergedPullRequests(repo string, w window) ([]pullRequest, error) {
	q := fmt.Sprintf("repo:%s is:pr is:merged merged:%s..%s", repo, w.from.Format("2006-01-02"), w.to.AddDate(0, 0, -1).Format("2006-01-02"))
	var prs []pullRequest
	after := ""
	for {
		args := []string{"api", "graphql", "-f", "query=" + prQuery, "-f", "q=" + q}
		if after != "" {
			args = append(args, "-f", "after="+after)
		}
		out, err := gh(args...)
		if err != nil {
			return nil, err
		}
		var page prPage
		if err := json.Unmarshal(out, &page); err != nil {
			return nil, err
		}
		for _, n := range page.Data.Search.Nodes {
			if strings.HasPrefix(n.HeadRefName, "dependabot/") {
				continue
			}
			pr := pullRequest{kind: "other", opened: n.CreatedAt, ready: n.CreatedAt, merged: n.MergedAt, firstCommit: n.CreatedAt}
			if m := titleKind.FindString(n.Title); m == "feat" || m == "fix" {
				pr.kind = m
			}
			for _, c := range n.Commits.Nodes {
				if c.Commit.AuthoredDate.Before(pr.firstCommit) {
					pr.firstCommit = c.Commit.AuthoredDate
				}
			}
			if t := n.TimelineItems.Nodes; len(t) > 0 && t[0].CreatedAt.After(pr.ready) {
				pr.ready = t[0].CreatedAt
			}
			prs = append(prs, pr)
		}
		if !page.Data.Search.PageInfo.HasNextPage {
			return prs, nil
		}
		after = page.Data.Search.PageInfo.EndCursor
	}
}

func printPullRequests(prs []pullRequest) {
	fmt.Printf("### Merged pull requests\n\n")
	fmt.Printf("Hours, median / 75th percentile. Dependabot left out.\n\n")
	fmt.Printf("| kind | n | first commit → merged | opened → ready | ready → merged |\n|---|---|---|---|---|\n")
	for _, kind := range []string{"feat", "fix", "other", "all"} {
		var total, draft, merge []float64
		for _, p := range prs {
			if kind != "all" && p.kind != kind {
				continue
			}
			total = append(total, p.merged.Sub(p.firstCommit).Hours())
			draft = append(draft, p.ready.Sub(p.opened).Hours())
			merge = append(merge, p.merged.Sub(p.ready).Hours())
		}
		fmt.Printf("| %s | %d | %s | %s | %s |\n", kind, len(total), spread(total), spread(draft), spread(merge))
	}
	fmt.Println()
}

// ----------------------------------------------------------------------------- CI

// run is one finished run of ci.yml.
type run struct {
	pullRequest bool
	failed      bool
	wall        time.Duration // created → last update: queueing included, as a worker waits it
}

type runsPage struct {
	WorkflowRuns []struct {
		Event      string
		Status     string
		Conclusion string
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
	} `json:"workflow_runs"`
}

// noOp is the wall clock below which a run checked nothing: a draft's run, or one cancelled at once.
const noOp = 2 * time.Minute

func ciRuns(repo string, w window) ([]run, error) {
	var runs []run
	// The API answers at most 1,000 runs per query, so the window is read a day at a time.
	for day := w.from; day.Before(w.to); day = day.AddDate(0, 0, 1) {
		path := fmt.Sprintf("repos/%s/actions/workflows/ci.yml/runs?per_page=100&created=%s", repo, day.Format("2006-01-02"))
		out, err := gh("api", "--paginate", path)
		if err != nil {
			return nil, err
		}
		dec := json.NewDecoder(bytes.NewReader(out))
		for dec.More() {
			var page runsPage
			if err := dec.Decode(&page); err != nil {
				return nil, err
			}
			for _, r := range page.WorkflowRuns {
				if r.Status != "completed" || r.Conclusion == "cancelled" {
					continue
				}
				runs = append(runs, run{
					pullRequest: strings.HasPrefix(r.Event, "pull_request"),
					failed:      r.Conclusion == "failure",
					wall:        r.UpdatedAt.Sub(r.CreatedAt),
				})
			}
		}
	}
	return runs, nil
}

func printRuns(runs []run) {
	fmt.Printf("### CI (ci.yml)\n\n")
	fmt.Printf("| runs | n | wall minutes, median / p75 | failed |\n|---|---|---|---|\n")
	var noOps int
	var noOpTime time.Duration
	for _, pr := range []bool{true, false} {
		var walls []float64
		failed := 0
		for _, r := range runs {
			if r.pullRequest != pr {
				continue
			}
			if pr && r.wall < noOp {
				noOps++
				noOpTime += r.wall
				continue
			}
			walls = append(walls, r.wall.Minutes())
			if r.failed {
				failed++
			}
		}
		name := "pull request, checked something"
		if !pr {
			name = "main"
		}
		fmt.Printf("| %s | %d | %s | %s |\n", name, len(walls), spread(walls), percent(failed, len(walls)))
	}
	fmt.Printf("| pull request, checked nothing (draft) | %d | %.1f h in total | — |\n\n", noOps, noOpTime.Hours())
}

// ----------------------------------------------------------------------------- transcripts

// sessions is what the transcripts of the window add up to.
type sessions struct {
	files, calls                  int
	cacheRead, cacheWrite, output int64
	inputAbove400k, inputTotal    int64
	rebuilds                      int
	rebuildTokens                 int64
	active, waiting               time.Duration
	checkRuns, waitCalls          int
}

// line is the part of a transcript line the report reads.
type line struct {
	Type      string
	Timestamp time.Time
	Message   struct {
		ID    string
		Usage *struct {
			InputTokens              int64 `json:"input_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
		}
		Content json.RawMessage
	}
}

type block struct {
	Type      string
	ID        string
	ToolUseID string `json:"tool_use_id"`
	Name      string
	Input     struct{ Command string }
}

var (
	// waitCommand is a command that only waits: for a background run, a file, a pull request.
	waitCommand = regexp.MustCompile(`\bsleep\b|\buntil\b`)
	// checkCommand is a command that runs the repository's checks.
	checkCommand = regexp.MustCompile(`make\s+(verify|gate-|check\b)`)
)

const (
	// rebuildTokens is a cache write large enough to be the whole context written again, not the
	// newest exchange appended.
	rebuildTokens = 50_000
	// idle is the gap after which time between two lines no longer counts as working.
	idle = 15 * time.Minute
)

func readTranscripts(dir, repoName string, w window) (sessions, error) {
	var s sessions
	seen := map[string]bool{}
	// Read through a root, so that a link inside the projects directory cannot lead the walk out
	// of it.
	root, err := os.OpenRoot(dir)
	if err != nil {
		return s, err
	}
	defer func() { _ = root.Close() }()
	err = fs.WalkDir(root.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".jsonl") || !strings.Contains(path, repoName) {
			return nil
		}
		data, err := root.ReadFile(path)
		if err != nil {
			return err
		}
		if addTranscript(&s, data, w, seen) {
			s.files++
		}
		return nil
	})
	return s, err
}

// addTranscript adds one transcript file's lines inside the window; it reports whether any were.
func addTranscript(s *sessions, data []byte, w window, seen map[string]bool) bool {
	var prev, prevCall time.Time
	pending := map[string]time.Time{}
	waits := map[string]bool{}
	counted := false
	for _, raw := range bytes.Split(data, []byte("\n")) {
		var l line
		if len(raw) == 0 || json.Unmarshal(raw, &l) != nil || l.Timestamp.IsZero() || !w.has(l.Timestamp) {
			continue
		}
		counted = true
		if !prev.IsZero() {
			if gap := l.Timestamp.Sub(prev); gap < idle {
				s.active += gap
			}
		}
		prev = l.Timestamp
		var blocks []block
		_ = json.Unmarshal(l.Message.Content, &blocks)
		switch l.Type {
		case "assistant":
			if u := l.Message.Usage; u != nil && !seen[l.Message.ID] {
				seen[l.Message.ID] = true
				input := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
				s.calls++
				s.cacheRead += u.CacheReadInputTokens
				s.cacheWrite += u.CacheCreationInputTokens
				s.output += u.OutputTokens
				s.inputTotal += input
				if input > 400_000 {
					s.inputAbove400k += input
				}
				if u.CacheCreationInputTokens > rebuildTokens && !prevCall.IsZero() {
					s.rebuilds++
					s.rebuildTokens += u.CacheCreationInputTokens
				}
				prevCall = l.Timestamp
			}
			for _, b := range blocks {
				if b.Type != "tool_use" || b.Name != "Bash" {
					continue
				}
				pending[b.ID] = l.Timestamp
				if waitCommand.MatchString(b.Input.Command) {
					waits[b.ID] = true
					s.waitCalls++
				}
				if checkCommand.MatchString(b.Input.Command) {
					s.checkRuns++
				}
			}
		case "user":
			for _, b := range blocks {
				if started, ok := pending[b.ToolUseID]; ok && b.Type == "tool_result" {
					if waits[b.ToolUseID] {
						s.waiting += l.Timestamp.Sub(started)
					}
					delete(pending, b.ToolUseID)
				}
			}
		}
	}
	return counted
}

func printSessions(s sessions) {
	fmt.Printf("### Sessions (Claude Code transcripts on this machine)\n\n")
	if s.calls == 0 {
		fmt.Printf("No transcript in the window.\n\n")
		return
	}
	fmt.Printf("| measure | value |\n|---|---|\n")
	fmt.Printf("| transcript files (sessions and subagents) | %d |\n", s.files)
	fmt.Printf("| model calls | %d |\n", s.calls)
	fmt.Printf("| cache read / cache write / output, million tokens | %.0f / %.0f / %.1f |\n", mil(s.cacheRead), mil(s.cacheWrite), mil(s.output))
	fmt.Printf("| input tokens in calls with more than 400k context | %s |\n", percent64(s.inputAbove400k, s.inputTotal))
	fmt.Printf("| whole context written again (cache rebuilds) | %d calls, %.0f M tokens, %s of cache writes |\n", s.rebuilds, mil(s.rebuildTokens), percent64(s.rebuildTokens, s.cacheWrite))
	fmt.Printf("| active hours / of which waiting in sleep or until loops | %.1f / %.1f (%s) |\n", s.active.Hours(), s.waiting.Hours(), percent64(int64(s.waiting), int64(s.active)))
	fmt.Printf("| runs of make verify, gate-* or check | %d |\n\n", s.checkRuns)
}

// ----------------------------------------------------------------------------- figures

func mil(n int64) float64 { return float64(n) / 1e6 }

func percent(part, whole int) string { return percent64(int64(part), int64(whole)) }

func percent64(part, whole int64) string {
	if whole == 0 {
		return "—"
	}
	return fmt.Sprintf("%.0f %%", 100*float64(part)/float64(whole))
}

// spread is "median / 75th percentile" of the values, one decimal.
func spread(v []float64) string {
	if len(v) == 0 {
		return "—"
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return fmt.Sprintf("%.1f / %.1f", quantile(s, 0.5), quantile(s, 0.75))
}

// quantile reads the q-quantile of sorted values by the nearest rank.
func quantile(sorted []float64, q float64) float64 {
	i := int(q * float64(len(sorted)))
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	if q == 0.5 && len(sorted)%2 == 0 {
		return (sorted[i-1] + sorted[i]) / 2
	}
	return sorted[i]
}
