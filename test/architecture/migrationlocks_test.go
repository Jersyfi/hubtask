// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// lastReleasedMigration is the highest migration the last release (v0.9.1) carries. A table it
// created holds an operator's data, and a migration after it must not take a lock on that table
// that stops writes for the length of a scan (versioning-release.md §4). Raised with each release.
const lastReleasedMigration = 206

// notYetConcurrent are the unreleased, unmerged files that still break the rule, each to be
// rewritten before the suite merges: the index moved into a NO TRANSACTION file after them, the
// constraint added NOT VALID and validated there. An entry is removed when its file is fixed; a
// file not named here is held to the rule at once.
var notYetConcurrent = map[string]bool{
	"0211_entry_field_value.sql": true, "0215_calculated_field.sql": true,
	"0218_rule_run_step_taken.sql": true, "0220_field_read_restricted.sql": true,
	"0232_page_review.sql": true, "0253_agent_notice.sql": true, "0254_suggestion_agent.sql": true,
	"0255_import_kinds.sql": true, "0256_directory_provisioning.sql": true,
	"0257_webhook_chat_format.sql": true, "0258_session_view_as.sql": true,
	"0266_whiteboard_comment.sql": true, "0269_notification_approval.sql": true,
	"0273_notification_plan_exception.sql": true, "0278_calendar_push.sql": true,
	"0284_saml_provider.sql": true, "0286_stable_keys.sql": true,
	"0295_notification_request_reply.sql": true, "0298_register_field_kinds.sql": true,
	"0301_media_scan_hold.sql": true, "0305_mailbox_graph.sql": true, "0312_person_rules.sql": true,
	"0313_desk_logo.sql": true, "0316_notification_sla.sql": true,
}

var (
	createdTable   = regexp.MustCompile(`(?i)\bCREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)`)
	createdIndex   = regexp.MustCompile(`(?is)\bCREATE\s+(?:UNIQUE\s+)?INDEX\s+(CONCURRENTLY\s+)?(?:IF\s+NOT\s+EXISTS\s+)?[a-z_0-9]*\s*ON\s+(?:ONLY\s+)?([a-z_][a-z0-9_]*)`)
	alteredTable   = regexp.MustCompile(`(?is)^\s*ALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:ONLY\s+)?([a-z_][a-z0-9_]*)\s+(.*)$`)
	addsConstraint = regexp.MustCompile(`(?is)\bADD\s+CONSTRAINT\s+([a-z_0-9]+)\s+(CHECK|FOREIGN\s+KEY)\b`)
	validates      = regexp.MustCompile(`(?is)\bVALIDATE\s+CONSTRAINT\s+([a-z_0-9]+)`)
	lineComment    = regexp.MustCompile(`--[^\n]*`)
	dollarBlock    = regexp.MustCompile(`(?s)\$([a-z_]*)\$.*?\$([a-z_]*)\$`)
)

type migrationFile struct {
	number        int
	name          string
	transactional bool
	statements    []string
}

func readMigrations(t *testing.T) []migrationFile {
	t.Helper()
	paths, err := filepath.Glob("../../db/migrations/*.sql")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no migrations found: %v", err)
	}
	files := make([]migrationFile, 0, len(paths))
	for _, path := range paths {
		raw, err := os.ReadFile(path) //nolint:gosec // G304: the repository's own files
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
		})
	}
	return files
}

// No migration after the last release takes a write-stopping lock on a released table for a scan:
// an index is built CONCURRENTLY, a CHECK or FOREIGN KEY is added NOT VALID, and its VALIDATE runs
// outside the transaction that added it - in a NO TRANSACTION file, as 0209 and 0210 do. Inside
// one transaction the ADD's ACCESS EXCLUSIVE lock is held through the VALIDATE's scan.
func TestNoMigrationAfterTheReleaseBlocksWritesOnAReleasedTable(t *testing.T) {
	for _, violation := range lockViolations(t) {
		if !notYetConcurrent[violation.file] {
			t.Errorf("%s: %s", violation.file, violation.what)
		}
	}
}

// An entry of notYetConcurrent that no longer breaks the rule is removed, so the list only shrinks.
func TestEveryFileNotYetConcurrentStillNeedsIt(t *testing.T) {
	breaking := map[string]bool{}
	for _, violation := range lockViolations(t) {
		breaking[violation.file] = true
	}
	for name := range notYetConcurrent {
		if !breaking[name] {
			t.Errorf("%s keeps the rule now; remove it from notYetConcurrent", name)
		}
	}
}

type lockViolation struct{ file, what string }

func lockViolations(t *testing.T) []lockViolation {
	t.Helper()
	files := readMigrations(t)
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
	if len(released) == 0 {
		t.Fatal("no released table found; the parser no longer reads the migrations")
	}
	notValid := regexp.MustCompile(`(?i)\bNOT\s+VALID\b`)
	var found []lockViolation
	for _, file := range files {
		if file.number <= lastReleasedMigration {
			continue
		}
		addedHere := map[string]bool{}
		for _, statement := range file.statements {
			if match := createdIndex.FindStringSubmatch(statement); match != nil &&
				released[strings.ToLower(match[2])] && match[1] == "" {
				found = append(found, lockViolation{file.name, "an index on the released table " + match[2] + " without CONCURRENTLY"})
			}
			altered := alteredTable.FindStringSubmatch(statement)
			if altered == nil || !released[strings.ToLower(altered[1])] {
				continue
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

// The constant above follows the release: the newest v* tag without a pre-release suffix carries
// exactly that many migrations. Skipped where the checkout has no tags (a shallow clone).
func TestTheLastReleasedMigrationIsTheLastReleasesHighest(t *testing.T) {
	out, err := exec.Command("git", "tag", "--list", "v*", "--sort=-v:refname").Output()
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
	listing, err := exec.Command("git", "ls-tree", "--name-only", tag, "../../db/migrations/").Output() //nolint:gosec // G204: a tag read from git itself
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
