// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

// LogError is one kind of error a run saw, counted (accepted.md, "The owner's word on the
// performance tests"): every occurrence of the same message is one row, so a run's errors are a
// list somebody can fix rather than a log somebody has to read.
//
// First and Last are the first and last occurrence where the log line carries a time, and
// Scenarios the scenarios whose windows any occurrence fell in. Expected is the reason a row is
// not a finding (ExpectedRows); empty, the row is one.
type LogError struct {
	Source    string    `json:"source"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Count     int       `json:"count"`
	First     time.Time `json:"first,omitzero"`
	Last      time.Time `json:"last,omitzero"`
	Scenarios []string  `json:"scenarios,omitempty"`
	Expected  string    `json:"expected,omitempty"`
}

// Scenario is one scenario's window in a run, from Start to End. A scenario run in parts (S1 per
// workspace, between S2's) has one window per part under the same name.
type Scenario struct {
	Name  string    `json:"name"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ExpectedRow is a message a run sees by design, with the reason it is not a finding.
type ExpectedRow struct {
	Source string
	// Prefix is the masked message's opening, so the query a statement ran can narrow it.
	Prefix string
	Reason string
}

// ExpectedRows are kept out of the findings and counted (test/load/PLAN.md, "How the errors are
// collected"). Any other row is a finding.
var ExpectedRows = []ExpectedRow{
	{Source: "postgres", Prefix: "canceling statement due to user request (in CountFigureProgress)",
		Reason: "the figure deadline cuts a late count off while the read answers without it; the late count keeps it (domain-model.md §2, the 10⁷ write-up's finding 4)"},
	{Source: "postgres", Prefix: "canceling statement due to user request (in CountFigureDue)",
		Reason: "the figure deadline cuts a late count off while the read answers without it; the late count keeps it (domain-model.md §2, the 10⁷ write-up's finding 4)"},
}

// ErrorReport is a run's errors-<items>.json: the scenarios' windows, the findings and the
// expected rows apart.
type ErrorReport struct {
	Scenarios []Scenario `json:"scenarios"`
	Findings  []LogError `json:"findings"`
	Expected  []LogError `json:"expected"`
}

// Report sorts rows into findings and expected rows. Rows of the same source, level and message
// from several readers - the browser's walk of three workspaces - are merged first.
func Report(scenarios []Scenario, rows []LogError) ErrorReport {
	merged := newCounter(nil)
	for _, row := range rows {
		merged.merge(row)
	}
	report := ErrorReport{Scenarios: scenarios, Findings: []LogError{}, Expected: []LogError{}}
	for _, row := range merged.rows() {
		for _, expected := range ExpectedRows {
			if row.Source == expected.Source && strings.HasPrefix(row.Message, expected.Prefix) {
				row.Expected = expected.Reason
				break
			}
		}
		if row.Expected == "" {
			report.Findings = append(report.Findings, row)
		} else {
			report.Expected = append(report.Expected, row)
		}
	}
	return report
}

// counter folds occurrences into rows, placing each in the scenarios whose window holds it.
type counter struct {
	scenarios []Scenario
	byKey     map[[3]string]*LogError
}

func newCounter(scenarios []Scenario) *counter {
	return &counter{scenarios: scenarios, byKey: map[[3]string]*LogError{}}
}

func (c *counter) row(source, level, message string) *LogError {
	key := [3]string{source, level, message}
	row, known := c.byKey[key]
	if !known {
		row = &LogError{Source: source, Level: level, Message: message}
		c.byKey[key] = row
	}
	return row
}

func (c *counter) add(source, level, message string, at time.Time) {
	row := c.row(source, level, message)
	row.Count++
	if at.IsZero() {
		return
	}
	widen(row, at, at)
	for _, scenario := range c.scenarios {
		if !at.Before(scenario.Start) && !at.After(scenario.End) && !slices.Contains(row.Scenarios, scenario.Name) {
			row.Scenarios = append(row.Scenarios, scenario.Name)
		}
	}
}

func (c *counter) merge(other LogError) {
	row := c.row(other.Source, other.Level, other.Message)
	row.Count += other.Count
	if !other.First.IsZero() {
		widen(row, other.First, other.Last)
	}
	for _, name := range other.Scenarios {
		if !slices.Contains(row.Scenarios, name) {
			row.Scenarios = append(row.Scenarios, name)
		}
	}
}

func widen(row *LogError, first, last time.Time) {
	if row.First.IsZero() || first.Before(row.First) {
		row.First = first
	}
	if last.After(row.Last) {
		row.Last = last
	}
}

func (c *counter) rows() []LogError {
	out := make([]LogError, 0, len(c.byKey))
	for _, row := range c.byKey {
		sort.Strings(row.Scenarios)
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Message != out[j].Message {
			return out[i].Message < out[j].Message
		}
		return out[i].Source+out[i].Level < out[j].Source+out[j].Level
	})
	return out
}

// postgresSeverity is a PostgreSQL log line's severity and what follows it.
var postgresSeverity = regexp.MustCompile(`\b(ERROR|FATAL|PANIC|WARNING):\s+(.*)$`)

// varying are the parts of a message that differ between occurrences of one error: numbers,
// quoted values and identifiers.
var varying = regexp.MustCompile(`"[^"]*"|'[^']*'|[0-9a-f]{8}-[0-9a-f-]{27}|\b\d+\b`)

// ServerErrors reads the server's JSON log and answers its WARN and ERROR entries, one row per
// message, each placed in the scenarios whose window held it. A line that is not JSON (the test's
// own output, interleaved) is passed over.
func ServerErrors(r io.Reader, scenarios []Scenario) ([]LogError, error) {
	counted := newCounter(scenarios)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Time  string `json:"time"`
			Level string `json:"level"`
			Msg   string `json:"msg"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Msg == "" {
			continue
		}
		if level := strings.ToUpper(entry.Level); level == "WARN" || level == "ERROR" {
			// A time that does not parse leaves the row unplaced rather than the message uncounted.
			at, _ := time.Parse(time.RFC3339Nano, entry.Time)
			counted.add("server", level, entry.Msg, at.UTC())
		}
	}
	return counted.rows(), scanner.Err()
}

// postgresTime is a log line's opening time, PostgreSQL's default log_line_prefix.
var postgresTime = regexp.MustCompile(`(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) UTC`)

func postgresLineTime(line string) time.Time {
	match := postgresTime.FindStringSubmatch(line)
	if match == nil {
		return time.Time{}
	}
	at, err := time.Parse("2006-01-02 15:04:05.999999999", match[1])
	if err != nil {
		return time.Time{}
	}
	return at.UTC()
}

// migrationProbe is goose's first statement on an empty database, which asks for its version table
// before it exists: every run's stack starts that way, so the row says nothing about the product.
const migrationProbe = `relation "goose_db_version" does not exist`

// postgresStatement is the STATEMENT line PostgreSQL logs after an error, and the sqlc query name
// it opens with. Only the name is kept: the rest is SQL, and a literal in it could be content.
var postgresStatement = regexp.MustCompile(`\bSTATEMENT:\s+(?:-- name: (\w+))?`)

// PostgresErrors reads PostgreSQL's log and answers its WARNING, ERROR, FATAL and PANIC lines, one
// row per message with its values masked, named by the query its STATEMENT line ran when that is
// a named one, and placed in the scenarios whose window held it.
func PostgresErrors(r io.Reader, scenarios []Scenario) ([]LogError, error) {
	counted := newCounter(scenarios)
	var pending *LogError
	var pendingAt time.Time
	flush := func(query string) {
		if pending == nil {
			return
		}
		if query != "" {
			pending.Message += " (in " + query + ")"
		}
		counted.add(pending.Source, pending.Level, pending.Message, pendingAt)
		pending = nil
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if pending != nil {
			if statement := postgresStatement.FindStringSubmatch(line); statement != nil {
				flush(statement[1])
				continue
			}
			if strings.HasPrefix(line, "\t") {
				continue
			}
			flush("")
		}
		match := postgresSeverity.FindStringSubmatch(line)
		if match == nil || strings.HasPrefix(match[2], migrationProbe) {
			continue
		}
		pending = &LogError{Source: "postgres", Level: match[1], Message: varying.ReplaceAllString(match[2], "…")}
		pendingAt = postgresLineTime(line)
	}
	flush("")
	return counted.rows(), scanner.Err()
}
