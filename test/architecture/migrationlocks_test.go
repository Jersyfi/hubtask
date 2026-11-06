// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// lastReleasedMigration is the highest migration the last release (v0.9.1) carries. A table it
// created holds an operator's data, and a migration after it must not take a lock on that table
// that stops writes for the length of a scan (versioning-release.md §4). Raised with each release.
const lastReleasedMigration = 206

// mergedBeforeTheCheck are main's migrations of milestones CD and F7, merged before this check met
// them. A merged migration never changes (rule 12): the lock each takes on its released table is
// what the release that ships them takes.
var mergedBeforeTheCheck = map[string]bool{"0211_own_domain.sql": true, "0214_app_client.sql": true}

var (
	createdTable   = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
	createdIndex   = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?[a-z_0-9]*\s*ON\s+(ONLY\s+)?([a-z_][a-z0-9_]*)`)
	alteredTable   = regexp.MustCompile(`(?is)^\s*ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([a-z_][a-z0-9_]*)\s+(.*)$`)
	addsConstraint = regexp.MustCompile(`(?is)\bADD\s+CONSTRAINT\s+([a-z_0-9]+)\s+(CHECK|FOREIGN\s+KEY)\b`)
	validates      = regexp.MustCompile(`(?is)\bVALIDATE\s+CONSTRAINT\s+([a-z_0-9]+)`)
	// The clauses of one ALTER TABLE; a column's own CHECK or REFERENCES is checked against every
	// row under the ALTER's ACCESS EXCLUSIVE lock.
	clauseStart = regexp.MustCompile(`(?i),(\s*(?:ADD|ALTER|DROP|VALIDATE|RENAME)\b)`)
	addsColumn  = regexp.MustCompile(`(?is)^\s*ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)\b(.*)$`)
	inlineCheck = regexp.MustCompile(`(?i)\b(CHECK|REFERENCES)\b`)
	lineComment = regexp.MustCompile(`--[^\n]*`)
	dollarBlock = regexp.MustCompile(`(?s)\$([a-z_]*)\$.*?\$([a-z_]*)\$`)
)

type migrationFile struct {
	number        int
	name          string
	transactional bool
	statements    []string
	// executed is the Up text without comments and without function bodies: what runs when the
	// file does, DO blocks included.
	executed string
}

func readMigrations(t *testing.T) []migrationFile {
	t.Helper()
	paths, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	files := make([]migrationFile, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		number, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil {
			t.Fatalf("%s: no number", name)
		}
		text := string(raw)
		up, _, _ := strings.Cut(text, "-- +goose Down")
		body := dollarBlock.ReplaceAllString(lineComment.ReplaceAllString(up, ""), "")
		files = append(files, migrationFile{
			number: number, name: name,
			transactional: !strings.Contains(text, "-- +goose NO TRANSACTION"),
			statements:    strings.Split(body, ";"),
			executed:      withoutFunctionBodies(lineComment.ReplaceAllString(up, "")),
		})
	}
	return files
}

// No migration after the last release takes a write-stopping lock on a released table for a scan:
// an index is built CONCURRENTLY, a CHECK or FOREIGN KEY is added NOT VALID, and its VALIDATE runs
// outside the transaction that added it - in a NO TRANSACTION file, as 0216 and 0217 do. Inside
// one transaction the ADD's ACCESS EXCLUSIVE lock is held through the VALIDATE's scan.
func TestNoMigrationAfterTheReleaseBlocksWritesOnAReleasedTable(t *testing.T) {
	for _, violation := range lockViolations(t) {
		t.Errorf("%s: %s", violation.file, violation.what)
	}
}

type lockViolation struct{ file, what string }

func lockViolations(t *testing.T) []lockViolation {
	t.Helper()
	files := readMigrations(t)
	released := releasedTables(files)
	if len(released) == 0 {
		t.Fatal("no released table found; the parser no longer reads the migrations")
	}
	notValid := regexp.MustCompile(`(?i)\bNOT\s+VALID\b`)
	var found []lockViolation
	for _, file := range files {
		if file.number <= lastReleasedMigration || mergedBeforeTheCheck[file.name] {
			continue
		}
		addedHere := map[string]bool{}
		for _, statement := range file.statements {
			if match := createdIndex.FindStringSubmatch(statement); match != nil &&
				released[strings.ToLower(match[3])] && match[1] == "" && match[2] == "" {
				// ON ONLY builds nothing; the migrator builds each partition's share concurrently.
				found = append(found, lockViolation{file.name, "an index on the released table " + match[3] + " without CONCURRENTLY"})
			}
			altered := alteredTable.FindStringSubmatch(statement)
			if altered == nil || !released[strings.ToLower(altered[1])] {
				continue
			}
			for _, clause := range strings.Split(clauseStart.ReplaceAllString(altered[2], "\x00$1"), "\x00") {
				column := addsColumn.FindStringSubmatch(clause)
				if column == nil {
					continue
				}
				if check := inlineCheck.FindStringSubmatch(column[2]); check != nil {
					found = append(found, lockViolation{file.name, "column " + column[1] + " added to the released table " + altered[1] + " with an inline " + strings.ToUpper(check[1])})
				}
			}
			if match := addsConstraint.FindStringSubmatch(altered[2]); match != nil {
				addedHere[strings.ToLower(match[1])] = true
				if !notValid.MatchString(altered[2]) {
					found = append(found, lockViolation{file.name, match[1] + " added to the released table " + altered[1] + " without NOT VALID"})
				}
			}
			if match := validates.FindStringSubmatch(altered[2]); match != nil &&
				file.transactional && addedHere[strings.ToLower(match[1])] {
				found = append(found, lockViolation{file.name, match[1] + " validated in the transaction that added it to " + altered[1]})
			}
		}
	}
	return found
}

var (
	functionStart   = regexp.MustCompile(`(?is)\bCREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\b[^$]*(\$[a-z_]*\$)`)
	alterStatement  = regexp.MustCompile(`(?im)^\s*ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([a-z_][a-z0-9_]*)`)
	updateStatement = regexp.MustCompile(`(?i)\bUPDATE\s+(?:ONLY\s+)?([a-z_][a-z0-9_]*)\s+SET\b`)
)

// withoutFunctionBodies drops each CREATE FUNCTION's quoted body, which is defined, not run.
func withoutFunctionBodies(text string) string {
	var out strings.Builder
	for {
		loc := functionStart.FindStringSubmatchIndex(text)
		if loc == nil {
			out.WriteString(text)
			return out.String()
		}
		tag := text[loc[2]:loc[3]]
		out.WriteString(text[:loc[2]])
		rest := text[loc[3]:]
		end := strings.Index(rest, tag)
		if end < 0 {
			return out.String()
		}
		text = rest[end+len(tag):]
	}
}

// No transaction that alters a released table also rewrites a released table's rows: the ALTER's
// ACCESS EXCLUSIVE lock is held until the commit, so every reader and writer of that table waits
// for the whole backfill. A backfill runs in a NO TRANSACTION file of its own and commits by batch,
// as 0219 does.
func TestNoMigrationBackfillsInsideTheTransactionThatAltersAReleasedTable(t *testing.T) {
	files := readMigrations(t)
	released := releasedTables(files)
	for _, file := range files {
		if file.number <= lastReleasedMigration || !file.transactional {
			continue
		}
		var altered []string
		for _, match := range alterStatement.FindAllStringSubmatch(file.executed, -1) {
			if released[strings.ToLower(match[1])] && !slices.Contains(altered, match[1]) {
				altered = append(altered, match[1])
			}
		}
		if len(altered) == 0 {
			continue
		}
		for _, match := range updateStatement.FindAllStringSubmatch(file.executed, -1) {
			if released[strings.ToLower(match[1])] {
				t.Errorf("%s: updates the released table %s in the transaction that alters %s", file.name, match[1], strings.Join(altered, ", "))
			}
		}
	}
}

func releasedTables(files []migrationFile) map[string]bool {
	released := map[string]bool{}
	for _, file := range files {
		if file.number > lastReleasedMigration {
			continue
		}
		for _, statement := range file.statements {
			for _, match := range createdTable.FindAllStringSubmatch(statement, -1) {
				released[strings.ToLower(match[1])] = true
			}
		}
	}
	return released
}

// The constant above follows the release: the newest v* tag without a pre-release suffix carries
// exactly that many migrations. Skipped where the checkout has no tags (a shallow clone).
func TestTheLastReleasedMigrationIsTheLastReleasesHighest(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "git", "tag", "--list", "v*", "--sort=-v:refname").Output()
	if err != nil {
		t.Skip("no git here")
	}
	var tag string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.Contains(line, "-") {
			tag = line
			break
		}
	}
	if tag == "" {
		t.Skip("no release tag in this checkout")
	}
	listing, err := exec.CommandContext(t.Context(), "git", "ls-tree", "--name-only", tag, "../../db/migrations/").Output()
	if err != nil {
		t.Skipf("reading %s: %v", tag, err)
	}
	highest := 0
	for _, path := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		if number, err := strconv.Atoi(strings.SplitN(filepath.Base(path), "_", 2)[0]); err == nil && number > highest {
			highest = number
		}
	}
	if highest != lastReleasedMigration {
		t.Errorf("%s carries migrations up to %d; lastReleasedMigration says %d", tag, highest, lastReleasedMigration)
	}
}
