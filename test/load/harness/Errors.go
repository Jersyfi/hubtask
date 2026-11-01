// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package harness

import (
	"bufio"
	"encoding/json"
	"io"
	"regexp"
	"sort"
	"strings"
)

// LogError is one kind of error a run saw, counted (accepted.md, "The owner's word on the
// performance tests"): every occurrence of the same message is one row, so a run's errors are a
// list somebody can fix rather than a log somebody has to read.
type LogError struct {
	Source  string `json:"source"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// postgresSeverity is a PostgreSQL log line's severity and what follows it.
var postgresSeverity = regexp.MustCompile(`\b(ERROR|FATAL|PANIC|WARNING):\s+(.*)$`)

// varying are the parts of a message that differ between occurrences of one error: numbers,
// quoted values and identifiers.
var varying = regexp.MustCompile(`"[^"]*"|'[^']*'|[0-9a-f]{8}-[0-9a-f-]{27}|\b\d+\b`)

// ServerErrors reads the server's JSON log and answers its WARN and ERROR entries, one row per
// message. A line that is not JSON (the test's own output, interleaved) is passed over.
func ServerErrors(r io.Reader) ([]LogError, error) {
	counted := map[LogError]int{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var entry struct {
			Level string `json:"level"`
			Msg   string `json:"msg"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Msg == "" {
			continue
		}
		if level := strings.ToUpper(entry.Level); level == "WARN" || level == "ERROR" {
			counted[LogError{Source: "server", Level: level, Message: entry.Msg}]++
		}
	}
	return rows(counted), scanner.Err()
}

// migrationProbe is goose's first statement on an empty database, which asks for its version table
// before it exists: every run's stack starts that way, so the row says nothing about the product.
const migrationProbe = `relation "goose_db_version" does not exist`

// postgresStatement is the STATEMENT line PostgreSQL logs after an error, and the sqlc query name
// it opens with. Only the name is kept: the rest is SQL, and a literal in it could be content.
var postgresStatement = regexp.MustCompile(`\bSTATEMENT:\s+(?:-- name: (\w+))?`)

// PostgresErrors reads PostgreSQL's log and answers its WARNING, ERROR, FATAL and PANIC lines, one
// row per message with its values masked, and named by the query its STATEMENT line ran when that
// is a named one.
func PostgresErrors(r io.Reader) ([]LogError, error) {
	counted := map[LogError]int{}
	var pending *LogError
	flush := func(query string) {
		if pending == nil {
			return
		}
		if query != "" {
			pending.Message += " (in " + query + ")"
		}
		counted[*pending]++
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
	}
	flush("")
	return rows(counted), scanner.Err()
}

func rows(counted map[LogError]int) []LogError {
	out := make([]LogError, 0, len(counted))
	for row, n := range counted {
		row.Count = n
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Message < out[j].Message
	})
	return out
}
