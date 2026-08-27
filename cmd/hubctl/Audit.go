// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	auditPath       = "/audit"
	auditVerifyPath = "/audit:verify"
	auditExportPath = "/audit:export"
)

func auditGroup() group {
	return group{
		name:    "audit",
		summary: "the evidence trail: reading it, checking it, taking a copy",
		commands: []command{
			{
				name:    "query",
				usage:   "[--from <t>] [--to <t>] [--action <prefix>] [--actor <id>] [--target <id>] [--outcome <o>]",
				summary: "the entries that match, newest first",
				run:     auditQuery,
			},
			{
				name:    "verify",
				usage:   "--from <t> --to <t>",
				summary: "check the hash chain and the absence of gaps over a period",
				run:     auditVerify,
			},
			{
				name:    "export",
				usage:   "--from <t> --to <t> --target <id> [--format JSONL|CSV] [--follow] [--wait <d>]",
				summary: "write a signed copy of a period to a backup target",
				run:     auditExport,
				waits:   true,
			},
		},
	}
}

// auditEntries is the answer to `GET /audit`: an inline schema in the contract, so the generator
// makes no type for it and this is written out the way the REST layer writes its own half.
type auditEntries struct {
	Items      []openapi.AuditEntry `json:"items"`
	NextCursor *string              `json:"next_cursor"`
}

func auditQuery(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "audit", "query",
		"[--from <t>] [--to <t>] [--action <prefix>] [--actor <id>] [--target-type <t>] [--target <id>] [--outcome <o>]")
	from := flags.String("from", "", "the start of the period, as 2026-08-27 or 2026-08-27T09:00:00Z")
	to := flags.String("to", "", "the end of the period")
	action := flags.String("action", "", "a prefix, e.g. auth. or membership.role_changed")
	actor := flags.String("actor", "", "one account's own entries")
	targetType := flags.String("target-type", "", "the kind of object, e.g. container or legal_hold")
	target := flags.String("target", "", "one object's whole history, whatever was done to it")
	outcome := flags.String("outcome", "", "SUCCESS, DENIED or FAILED")
	limit := flags.Int("limit", 0, "how many entries (the server decides when unset)")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	if err := cli.addPeriod(query, *from, *to); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"action": *action, "target_type": *targetType, "outcome": *outcome, "cursor": *cursor,
	} {
		if value != "" {
			query.Set(name, value)
		}
	}
	for name, raw := range map[string]string{"actor_id": *actor, "target_id": *target} {
		if raw == "" {
			continue
		}
		parsed, err := cli.parseID("--"+strings.TrimSuffix(name, "_id"), raw)
		if err != nil {
			return err
		}
		query.Set(name, parsed.String())
	}
	if *limit > 0 {
		query.Set("limit", strconv.Itoa(*limit))
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page auditEntries
	if err := client.Get(ctx, auditPath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, entryTable(page.Items)); err != nil {
		return err
	}
	if !cli.JSON && page.NextCursor != nil && *page.NextCursor != "" {
		printf(cli.Err, "more entries follow; continue with --cursor %s\n", *page.NextCursor)
	}
	return nil
}

// verification is the answer to `POST /audit:verify`, written out for the same reason.
type verification struct {
	Valid          bool       `json:"valid"`
	Checked        int        `json:"checked"`
	FirstBrokenSeq *int64     `json:"first_broken_seq"`
	Gaps           []int64    `json:"gaps"`
	GapCount       int        `json:"gap_count"`
	SealedUntil    *time.Time `json:"sealed_until"`
}

// auditVerify fails the command when the chain does not hold. A broken chain printed as a table
// with `valid false` in it, and exit 0 beside it, is how a scheduled check reports for years that
// everything is fine.
func auditVerify(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "audit", "verify", "--from <t> --to <t>")
	from := flags.String("from", "", "the start of the period")
	to := flags.String("to", "", "the end of the period")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *from == "" || *to == "" {
		return usagef("audit verify needs --from and --to: a chain is checked over a period")
	}

	start, err := cli.parseInstant("--from", *from)
	if err != nil {
		return err
	}
	end, err := cli.parseInstant("--to", *to)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var checked verification
	if err := client.Post(ctx, auditVerifyPath,
		map[string]string{"from": start, "to": end}, &checked); err != nil {
		return err
	}

	if err := cli.Emit(checked, Table{
		Columns: []string{"valid", "checked", "first broken", "gaps", "anchored"},
		Rows: [][]string{{
			boolText(checked.Valid),
			strconv.Itoa(checked.Checked),
			sequence(checked.FirstBrokenSeq),
			gapSummary(checked),
			anchored(checked.SealedUntil),
		}},
	}); err != nil {
		return err
	}
	if !checked.Valid {
		return errorString(fmt.Sprintf(
			"the chain does not hold over that period: %d entries checked, %d missing",
			checked.Checked, checked.GapCount))
	}
	return nil
}

func auditExport(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "audit", "export", "--from <t> --to <t> --target <id> [--format JSONL|CSV] [--follow]")
	from := flags.String("from", "", "the start of the period")
	to := flags.String("to", "", "the end of the period")
	target := flags.String("target", "", "the backup target the archive is written to")
	format := flags.String("format", "", "JSONL for a second system, CSV for a spreadsheet")
	follow := flags.Bool("follow", false, "wait for the export to finish")
	wait := waitFlag(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *from == "" || *to == "" || *target == "" {
		return usagef("audit export needs --from, --to and --target")
	}

	start, err := cli.parseInstant("--from", *from)
	if err != nil {
		return err
	}
	end, err := cli.parseInstant("--to", *to)
	if err != nil {
		return err
	}
	targetID, err := cli.parseID("--target", *target)
	if err != nil {
		return err
	}

	request := map[string]any{"from": start, "to": end, "target_id": targetID.String()}
	if *format != "" {
		request["format"] = *format
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var accepted openapi.JobRef
	if err := client.Post(ctx, auditExportPath, request, &accepted); err != nil {
		return err
	}
	if !*follow {
		return cli.Emit(accepted, jobRefTable(accepted))
	}

	job, err := cli.followJob(ctx, client, accepted.JobId, *wait)
	if err != nil {
		return err
	}
	if err := cli.jobFailed(job); err != nil {
		return err
	}
	return cli.Emit(job, jobTable(job))
}

// addPeriod puts a period into a query, when there is one. Both ends are optional here - a query
// with neither is "everything the server will give me", which is a reasonable first command.
func (cli *CLI) addPeriod(query url.Values, from, to string) error {
	for name, raw := range map[string]string{"from": from, "to": to} {
		if raw == "" {
			continue
		}
		instant, err := cli.parseInstant("--"+name, raw)
		if err != nil {
			return err
		}
		query.Set(name, instant)
	}
	return nil
}

// parseInstant reads a moment the way a person types one.
//
// Two spellings: the full RFC 3339 the API speaks, and a bare date, which is what somebody
// checking "yesterday" actually types. A bare date is midnight UTC rather than local midnight -
// the trail is stored in UTC, and a period whose ends moved with the operator's timezone would
// answer differently in Berlin and in Dublin for the same command.
func (cli *CLI) parseInstant(what, raw string) (string, error) {
	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed.UTC().Format(time.RFC3339), nil
	}
	if parsed, err := time.Parse("2006-01-02", raw); err == nil {
		return parsed.UTC().Format(time.RFC3339), nil
	}
	return "", usagef("%s is a moment: 2026-08-27 or 2026-08-27T09:00:00Z, not %q", what, raw)
}

func entryTable(entries []openapi.AuditEntry) Table {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			shortTime(&entry.OccurredAt),
			entry.Action,
			string(entry.Outcome),
			actorOf(entry),
			targetOf(entry),
			text(entry.LegalBasis),
		})
	}
	return Table{
		Columns: []string{"when", "action", "outcome", "actor", "target", "legal basis"},
		Rows:    rows,
	}
}

// actorOf prefers the label the entry recorded at the time. That is what makes a trail readable
// after an account is gone - and after an erasure it is a pseudonym, which is the whole of
// audit.md §6 arriving in a column.
func actorOf(entry openapi.AuditEntry) string {
	if entry.Actor.Label != nil && *entry.Actor.Label != "" {
		return *entry.Actor.Label
	}
	if entry.Actor.Id != nil {
		return entry.Actor.Id.String()
	}
	if entry.Actor.Type != nil {
		return string(*entry.Actor.Type)
	}
	return "-"
}

func targetOf(entry openapi.AuditEntry) string {
	if entry.Target == nil {
		return "-"
	}
	kind := text(entry.Target.Type)
	if entry.Target.Label != nil && *entry.Target.Label != "" {
		return kind + " " + *entry.Target.Label
	}
	if entry.Target.Id != nil {
		return kind + " " + entry.Target.Id.String()
	}
	return kind
}

func sequence(value *int64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatInt(*value, 10)
}

// gapSummary says how many are missing and shows the first few. The API cuts the list at a
// hundred, and a table is a worse place for a hundred numbers than it is for three.
func gapSummary(checked verification) string {
	if checked.GapCount == 0 {
		return "none"
	}
	shown := checked.Gaps
	if len(shown) > 3 {
		shown = shown[:3]
	}
	parts := make([]string, 0, len(shown))
	for _, gap := range shown {
		parts = append(parts, strconv.FormatInt(gap, 10))
	}
	return strconv.Itoa(checked.GapCount) + " missing (" + strings.Join(parts, ", ") + "…)"
}

// anchored says when this tenant's chain was last anchored outside the database. Never, in every
// installation until external anchoring exists - and saying "never" is the honest version of a
// blank column, because the check proves the chain intact *inside* the database and nothing more
// (audit.md §3).
func anchored(sealedUntil *time.Time) string {
	if sealedUntil == nil {
		return "never"
	}
	return shortTime(sealedUntil)
}
