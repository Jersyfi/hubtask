// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/tools/losttests"
)

// githubPart prints the sections read from GitHub and fills in its rows of the measures.
func githubPart(b *bytes.Buffer, found map[string]measure, repo, reviewer string, w window, withJobs bool) error {
	p := w.before()
	pp := p.before()
	// A fix is held against the feats of the seven days before it, so the read starts that early.
	prs, err := mergedPullRequests(repo, window{from: pp.from.AddDate(0, 0, -7), to: w.to})
	if err != nil {
		return fmt.Errorf("pull requests: %w", err)
	}
	printPullRequests(b, mergedIn(prs, w))
	unreadFiles := readFiles(prs)
	// Three windows of runs: a red main is seen from the failed run that began it.
	runs, drafts, err := ciRuns(repo, window{from: pp.from, to: w.to})
	if err != nil {
		return fmt.Errorf("CI runs: %w", err)
	}
	printRuns(b, createdIn(runs, w), createdIn(drafts, w))
	now := time.Now()
	for id, read := range map[string]func(window) value{
		"pr_run_median_minutes":       func(x window) value { return prMedian(createdIn(runs, x)) },
		"pr_runs_failed_percent":      func(x window) value { return prFailed(createdIn(runs, x)) },
		"main_second_attempt_percent": func(x window) value { return secondAttempts(createdIn(runs, x)) },
		"fix_after_feat": func(x window) value {
			v := count(fixAfterFeat(prs, x))
			if unreadFiles > 0 {
				v.text += fmt.Sprintf(" (not read: %d pull requests whose merge commit is not in this clone)", unreadFiles)
			}
			return v
		},
	} {
		found[id] = measure{now: read(w), before: read(p), earlier: read(pp)}
	}
	found["main_red_hours"] = measure{now: redHours(runs, w, now), before: redHours(runs, p, now)}
	if withJobs {
		summary, err := summaryJobs()
		if err != nil {
			return fmt.Errorf("the summary job's names: %w", err)
		}
		jobs, unread, why := jobsOf(repo, createdIn(runs, w), summary)
		printJobs(b, jobs, unread, why)
	}
	state := unmeasured("an issue's or an alert's state is read as it is now")
	found["open_findings"] = measure{now: openIssues(repo, "finding"), before: state}
	found["open_decisions"] = measure{now: openIssues(repo, "decision"), before: state}
	for id, kind := range map[string]string{"alerts_code_scanning": "code-scanning", "alerts_secret_scanning": "secret-scanning", "alerts_dependabot": "dependabot"} {
		found[id] = measure{now: openAlerts(repo, kind), before: state}
	}
	found["outside_reviewer_comments"] = measure{now: reviewerComments(repo, reviewer, w), before: reviewerComments(repo, reviewer, p)}
	share := unmeasured("nothing records which comment led to a change")
	found["outside_reviewer_share"] = measure{now: share, before: share}

	lost, unread := lostTests(mergedIn(prs, w))
	printLostTests(b, lost, unread)
	found["lost_tests"] = measure{now: count(len(lost)), before: unmeasured("read for this window only")}
	if unread > 0 && len(lost) == 0 {
		found["lost_tests"] = measure{now: unmeasured("the merge commits of %d pull requests are not in this clone", unread), before: found["lost_tests"].before}
	}
	return nil
}

func mergedIn(prs []pullRequest, w window) []pullRequest {
	var out []pullRequest
	for _, pr := range prs {
		if w.has(pr.merged) {
			out = append(out, pr)
		}
	}
	return out
}

func createdIn(runs []run, w window) []run {
	var out []run
	for _, r := range runs {
		if w.has(r.created) {
			out = append(out, r)
		}
	}
	return out
}

// ----------------------------------------------------------------------------- runs

// ofPullRequests are the runs a pull request started; the others are main's.
func ofPullRequests(runs []run, yes bool) []run {
	var out []run
	for _, r := range runs {
		if r.pullRequest == yes {
			out = append(out, r)
		}
	}
	return out
}

const noRuns = "no such run in the window"

// counted is a number with the count it was taken from beside it, so a share of three runs does
// not read like a share of three hundred.
func counted(n float64, format string, of int, what string) value {
	v := number(n, format)
	v.text += fmt.Sprintf(" (n = %d %s)", of, what)
	return v
}

func prMedian(runs []run) value {
	var walls []float64
	for _, r := range ofPullRequests(runs, true) {
		walls = append(walls, r.wall.Minutes())
	}
	if len(walls) == 0 {
		return unmeasured(noRuns)
	}
	sort.Float64s(walls)
	return counted(quantile(walls, 0.5), "%.1f", len(walls), "runs")
}

func prFailed(runs []run) value {
	all := ofPullRequests(runs, true)
	if len(all) == 0 {
		return unmeasured(noRuns)
	}
	failed := 0
	for _, r := range all {
		if r.failed {
			failed++
		}
	}
	return counted(100*float64(failed)/float64(len(all)), "%.0f", len(all), "runs")
}

// secondAttempts is the share of main's runs whose newest attempt is not the first.
func secondAttempts(runs []run) value {
	all := ofPullRequests(runs, false)
	if len(all) == 0 {
		return unmeasured(noRuns)
	}
	again := 0
	for _, r := range all {
		if r.attempt > 1 {
			again++
		}
	}
	return counted(100*float64(again)/float64(len(all)), "%.0f", len(all), "runs")
}

// redHours is how long main was red inside the window: from the end of a failed run on main to
// the end of the next one that passed. It reads each run's last attempt, so a run that failed and
// passed when run again counts as passed. Time without a run counts as the state the last run
// left, so a red Friday is red until Monday's first green; the runs the window had stand beside
// the hours.
func redHours(runs []run, w window, now time.Time) value {
	main := ofPullRequests(runs, false)
	sort.Slice(main, func(i, j int) bool { return main[i].created.Before(main[j].created) })
	var total time.Duration
	var since time.Time
	red := false
	inside := func(from, to time.Time) time.Duration {
		if from.Before(w.from) {
			from = w.from
		}
		if to.After(w.to) {
			to = w.to
		}
		if to.After(from) {
			return to.Sub(from)
		}
		return 0
	}
	for _, r := range main {
		switch {
		case r.failed && !red:
			red, since = true, r.updated
		case !r.failed && red:
			red = false
			total += inside(since, r.updated)
		}
	}
	if red {
		total += inside(since, now)
	}
	return counted(total.Hours(), "%.1f", len(createdIn(main, w)), "`main` runs")
}

// ----------------------------------------------------------------------------- jobs

// job is one execution of a job that passed or failed. Every attempt of a run is read, so a job
// that failed and passed when run again counts twice, once as failed.
type job struct {
	name    string
	failed  bool
	minutes float64
}

type jobsPage struct {
	Jobs []struct {
		Name        string
		Status      string
		Conclusion  string
		StartedAt   time.Time `json:"started_at"`
		CompletedAt time.Time `json:"completed_at"`
	}
}

const ciWorkflow = ".github/workflows/ci.yml"

var (
	summaryJobName = regexp.MustCompile(`(?m)^  ci-required:\n(?:\s*#.*\n)*    name: (.*)$`)
	quoted         = regexp.MustCompile(`'([^']+)'`)
)

// summaryJobs are the names ci.yml gives the one job that sums up the others (ci-cd.md §3.2). It
// fails whenever another job does, so it would lead every table and say nothing. The names are
// read from the workflow, the one place that sets them.
func summaryJobs() (map[string]bool, error) {
	data, err := os.ReadFile(ciWorkflow)
	if err != nil {
		return nil, err
	}
	names := summaryJobsIn(data)
	if len(names) == 0 {
		return nil, fmt.Errorf("%s names no ci-required job", ciWorkflow)
	}
	return names, nil
}

func summaryJobsIn(workflow []byte) map[string]bool {
	names := map[string]bool{}
	if m := summaryJobName.FindSubmatch(workflow); m != nil {
		for _, q := range quoted.FindAllSubmatch(m[1], -1) {
			// The other quoted words of the expression are event names.
			if strings.Contains(string(q[1]), " ") {
				names[string(q[1])] = true
			}
		}
	}
	return names
}

// jobsOf reads the jobs of each run, attempt by attempt: the API fails to list every attempt of
// a run at once where a run has many. unread counts the runs it did not answer for - one lost
// answer leaves the report standing - and why is its first refusal.
func jobsOf(repo string, runs []run, summary map[string]bool) (jobs []job, unread int, why string) {
	for _, r := range runs {
		seen := map[string]bool{}
		for attempt := 1; attempt <= max(1, r.attempt); attempt++ {
			out, err := gh("api", "--paginate", fmt.Sprintf("repos/%s/actions/runs/%d/attempts/%d/jobs?per_page=100", repo, r.id, attempt))
			if err == nil {
				var read []job
				if read, err = parseJobs(out, summary, seen); err == nil {
					jobs = append(jobs, read...)
					continue
				}
			}
			unread++
			if why == "" {
				why = refusal(err)
			}
			break
		}
	}
	return jobs, unread, why
}

// parseJobs reads the pages of one attempt. A later attempt lists the jobs it did not run again
// with the times of the attempt that ran them; seen holds those, so each execution counts once.
func parseJobs(data []byte, summary, seen map[string]bool) ([]job, error) {
	var jobs []job
	dec := json.NewDecoder(bytes.NewReader(data))
	for dec.More() {
		var page jobsPage
		if err := dec.Decode(&page); err != nil {
			return nil, err
		}
		for _, j := range page.Jobs {
			if j.Status != "completed" || summary[j.Name] || (j.Conclusion != "success" && !failedConclusions[j.Conclusion]) {
				continue
			}
			key := j.Name + "\x00" + j.StartedAt.Format(time.RFC3339)
			if seen[key] {
				continue
			}
			seen[key] = true
			jobs = append(jobs, job{name: j.Name, failed: failedConclusions[j.Conclusion], minutes: j.CompletedAt.Sub(j.StartedAt).Minutes()})
		}
	}
	return jobs, nil
}

// jobStat is one job's name across the window's runs and their attempts.
type jobStat struct {
	name             string
	attempts, failed int
	median           float64
}

func jobStats(jobs []job) []jobStat {
	minutes := map[string][]float64{}
	failed := map[string]int{}
	for _, j := range jobs {
		minutes[j.name] = append(minutes[j.name], j.minutes)
		if j.failed {
			failed[j.name]++
		}
	}
	stats := make([]jobStat, 0, len(minutes))
	for name, m := range minutes {
		sort.Float64s(m)
		stats = append(stats, jobStat{name: name, attempts: len(m), failed: failed[name], median: quantile(m, 0.5)})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].name < stats[j].name })
	return stats
}

const worst = 10

func printJobs(b *bytes.Buffer, jobs []job, unread int, why string) {
	stats := jobStats(jobs)
	fmt.Fprintf(b, "### Jobs, the ten worst (this window)\n\n")
	fmt.Fprintf(b, "Every attempt of a job counts. The job that sums up the others is left out.\n\n")
	if unread > 0 {
		fmt.Fprintf(b, "Not read: the jobs of %d runs; the first of them: %s.\n\n", unread, why)
	}
	if len(stats) == 0 {
		fmt.Fprintf(b, "No finished job in the window.\n\n")
		return
	}
	sort.SliceStable(stats, func(i, j int) bool { return stats[i].failed > stats[j].failed })
	fmt.Fprintf(b, "| by failures | failed | attempts |\n|---|---|---|\n")
	for _, s := range stats[:min(worst, len(stats))] {
		if s.failed > 0 {
			fmt.Fprintf(b, "| %s | %d | %d |\n", s.name, s.failed, s.attempts)
		}
	}
	sort.SliceStable(stats, func(i, j int) bool { return stats[i].median > stats[j].median })
	fmt.Fprintf(b, "\n| by median minutes | median | attempts |\n|---|---|---|\n")
	for _, s := range stats[:min(worst, len(stats))] {
		fmt.Fprintf(b, "| %s | %.1f | %d |\n", s.name, s.median, s.attempts)
	}
	fmt.Fprintln(b)
}

// ----------------------------------------------------------------------------- issues and alerts

func openIssues(repo, label string) value {
	out, err := gh("api", "-X", "GET", "search/issues", "-f", fmt.Sprintf("q=repo:%s is:issue is:open label:%s", repo, label),
		"-f", "sort=created", "-f", "order=asc", "-f", "per_page=1")
	if err != nil {
		return unmeasured("%s", refusal(err))
	}
	var page struct {
		TotalCount int `json:"total_count"`
		Items      []struct {
			Number    int
			CreatedAt time.Time `json:"created_at"`
		}
	}
	if err := json.Unmarshal(out, &page); err != nil {
		return unmeasured("%v", err)
	}
	v := count(page.TotalCount)
	if len(page.Items) > 0 {
		v.text += fmt.Sprintf(", the oldest #%d of %s", page.Items[0].Number, page.Items[0].CreatedAt.Format("2006-01-02"))
	}
	return v
}

// openAlerts counts the open alerts of one kind. Where the API does not hand them out, the row
// says what it answered.
func openAlerts(repo, kind string) value {
	out, err := gh("api", "--paginate", fmt.Sprintf("repos/%s/%s/alerts?state=open&per_page=100", repo, kind), "--jq", ".[].created_at")
	if err != nil {
		return unmeasured("%s", refusal(err))
	}
	days := strings.Fields(string(out))
	v := count(len(days))
	if len(days) > 0 {
		sort.Strings(days)
		v.text += ", the oldest of " + days[0][:min(10, len(days[0]))]
	}
	return v
}

var httpStatus = regexp.MustCompile(`HTTP (\d)\d\d`)

// refusal is the API's own answer out of gh's error, or the error where it carries none. A 404
// is what the API says for a feature that is off and for one the token may not see; a 5xx is no
// answer at all, after the one retry.
func refusal(err error) string {
	m := httpStatus.FindStringSubmatch(err.Error())
	switch {
	case m == nil:
		return err.Error()
	case m[0] == "HTTP 404":
		return "the feature is off or the token may not read it (" + m[0] + ")"
	case m[1] == "5":
		return "the API did not answer (" + m[0] + ")"
	}
	return "the API refused the request (" + m[0] + ")"
}

// reviewerComments counts the review comments the outside reviewer made in the window (ci-cd.md
// §5). Its login is the caller's to give: the tool names no product.
func reviewerComments(repo, login string, w window) value {
	if login == "" {
		return unmeasured("no login of an outside reviewer was given (-reviewer)")
	}
	out, err := gh("api", "--paginate", fmt.Sprintf("repos/%s/pulls/comments?per_page=100&since=%s", repo, w.from.Format(time.RFC3339)),
		"--jq", fmt.Sprintf(".[] | select(.user.login == %q) | .created_at", login))
	if err != nil {
		return unmeasured("%s", refusal(err))
	}
	return commentsIn(string(out), w)
}

// commentsIn counts the moments inside the window. None is a count, not a missing measure: why
// there are none - not installed, not asked - is not something the API says.
func commentsIn(stamps string, w window) value {
	n := 0
	for _, stamp := range strings.Fields(stamps) {
		if t, err := time.Parse(time.RFC3339, stamp); err == nil && w.has(t) {
			n++
		}
	}
	v := count(n)
	if n == 0 {
		v.text = "none in the window"
	}
	return v
}

// ----------------------------------------------------------------------------- fix after feat

// readFiles fills in the paths each feat and fix changed, from its merge commit: the API cuts a
// pull request's list of files, a diff does not. It returns how many it could not read.
func readFiles(prs []pullRequest) (unread int) {
	for i, pr := range prs {
		if pr.kind == "other" {
			continue
		}
		out, err := git("-c", "core.quotepath=off", "diff", "--name-only", "--no-renames", pr.mergeCommit+"^", pr.mergeCommit)
		if pr.mergeCommit == "" || err != nil {
			unread++
			continue
		}
		prs[i].files = strings.Split(strings.TrimSpace(string(out)), "\n")
	}
	return unread
}

// packageDepth is how deep a client's package is named: apps/webapp/src/lib/<area>.
const packageDepth = 5

// packagesOf are the packages a change touched. A Go file's package is its directory. A file
// under apps/ or packages/ belongs to its directory too, named at most packageDepth segments
// deep, so two changes to the web client meet only where they touch the same area of it. Any
// other file belongs to no package.
func packagesOf(files []string) map[string]bool {
	out := map[string]bool{}
	for _, file := range files {
		dir := strings.Split(path.Dir(file), "/")
		switch {
		case (dir[0] == "apps" || dir[0] == "packages") && len(dir) > 1:
			out[strings.Join(dir[:min(packageDepth, len(dir))], "/")] = true
		case strings.HasSuffix(file, ".go"):
			out[path.Dir(file)] = true
		}
	}
	return out
}

// fixAfterFeat counts the fix pull requests merged in the window that touch a package a feat
// touched, the feat merged at most seven days earlier.
func fixAfterFeat(prs []pullRequest, w window) int {
	n := 0
	for _, fix := range prs {
		if fix.kind != "fix" || !w.has(fix.merged) {
			continue
		}
		touched := packagesOf(fix.files)
		for _, feat := range prs {
			if feat.kind != "feat" || !feat.merged.Before(fix.merged) || fix.merged.Sub(feat.merged) > 7*24*time.Hour {
				continue
			}
			if shares(touched, packagesOf(feat.files)) {
				n++
				break
			}
		}
	}
	return n
}

func shares(a, b map[string]bool) bool {
	for name := range a {
		if b[name] {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------------- lost tests

// lostTest is a test a merged pull request took away, with what its description says about it.
type lostTest struct {
	losttests.Test
	pr     int
	reason string
}

// lostTests reads the merge commit of each pull request against its parent, by the reading the
// pull request gate holds a branch to (tools/losttests). unread counts the pull requests whose
// merge commit this clone does not have.
func lostTests(prs []pullRequest) (lost []lostTest, unread int) {
	for _, pr := range prs {
		if pr.mergeCommit == "" {
			unread++
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		found, err := losttests.Between(ctx, ".", pr.mergeCommit+"^", pr.mergeCommit)
		cancel()
		if err != nil {
			unread++
			continue
		}
		for _, l := range found {
			lost = append(lost, lostTest{Test: l, pr: pr.number, reason: reasonFor(pr.body, l)})
		}
	}
	return lost, unread
}

const (
	// reasonLength is how much of a description's line the table prints.
	reasonLength = 120
	// lostRows bounds the table, so one large pull request does not push the weekly comment over
	// the length GitHub takes.
	lostRows = 30
)

// reasonFor is the line of the description that names the test, or "" where none does.
func reasonFor(body string, l losttests.Test) string {
	for _, row := range strings.Split(body, "\n") {
		if l.NamedIn(row) {
			return row
		}
	}
	return ""
}

// inert prints text a stranger may have written - a test's title, a path, a line of a
// description - as a code span, cut to limit runes. Inside one, a mention notifies nobody and a
// link, an image or HTML stays text; the backtick that would end the span becomes a quote.
func inert(text string, limit int) string {
	text = strings.ReplaceAll(strings.Join(strings.Fields(text), " "), "`", "'")
	if text == "" {
		return "—"
	}
	if r := []rune(text); len(r) > limit {
		text = string(r[:limit]) + "…"
	}
	// A table's cell ends at a bar, in a code span too.
	return "`" + strings.ReplaceAll(text, "|", "\\|") + "`"
}

func printLostTests(b *bytes.Buffer, lost []lostTest, unread int) {
	fmt.Fprintf(b, "### Tests deleted or skipped in merged pull requests (this window)\n\n")
	if unread > 0 {
		fmt.Fprintf(b, "Not read: %d pull requests whose merge commit is not in this clone.\n\n", unread)
	}
	if len(lost) == 0 {
		fmt.Fprintf(b, "None found in the merge commits read.\n\n")
		return
	}
	fmt.Fprintf(b, "| pull request | file | test | how | what its description says |\n|---|---|---|---|---|\n")
	for _, l := range lost[:min(lostRows, len(lost))] {
		reason := "the description does not name it"
		if l.reason != "" {
			reason = inert(l.reason, reasonLength)
		}
		fmt.Fprintf(b, "| #%d | %s | %s | %s | %s |\n", l.pr, inert(l.Path, reasonLength), inert(l.Name, reasonLength), l.How, reason)
	}
	if more := len(lost) - lostRows; more > 0 {
		fmt.Fprintf(b, "\nAnd %d more, not printed.\n", more)
	}
	fmt.Fprintln(b)
}
