// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"net/url"
	"strconv"
	"time"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const privacyRequestsPath = "/privacy/requests"

func dsrGroup() group {
	return group{
		name:    "dsr",
		summary: "the rights somebody has exercised, and the deadline on each",
		commands: []command{
			{
				name:    "create",
				usage:   "--kind ACCESS|ERASURE|PORTABILITY|RESTRICTION|OBJECTION|RECTIFICATION [--subject <id>] [--email <address>]",
				summary: "record a case",
				run:     dsrCreate,
			},
			{
				name:    "ls",
				usage:   "[--status <s>] [--kind <k>] [--due-within <days>] [--include-closed]",
				summary: "the open cases, soonest deadline first",
				run:     dsrList,
			},
			{
				name:    "start",
				usage:   "<id> [--mode ANONYMIZE|FULL_DELETE] [--target <id>]",
				summary: "begin the work, which for an access or an erasure is what runs it",
				run:     dsrStart,
			},
			{
				name:    "complete",
				usage:   "<id>",
				summary: "close a case that was answered by hand",
				run:     dsrComplete,
			},
			{
				name:    "reject",
				usage:   "<id> --reason <why>",
				summary: "refuse a case, with the reason - a refusal in time is an answer",
				run:     dsrReject,
			},
		},
	}
}

func dsrCreate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "dsr", "create",
		"--kind <kind> [--subject <id>] [--email <address>] [--scope TENANT|INSTALLATION] [--target <id>]")
	kind := flags.String("kind", "", "ACCESS, ERASURE, PORTABILITY, RESTRICTION, OBJECTION or RECTIFICATION")
	subject := flags.String("subject", "", "the account the case is about")
	email := flags.String("email", "", "who asked, for a request with no account behind it")
	scope := flags.String("scope", "", "TENANT, or INSTALLATION for every workspace the person is in")
	target := flags.String("target", "", "the backup target an access or portability export is written to")
	due := flags.String("due", "", "the deadline, when it is not thirty days from receipt")
	notes := flags.String("notes", "", "what was asked, in your own words")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *kind == "" {
		return usagef("dsr create needs --kind")
	}
	if *subject == "" && *email == "" {
		return usagef("dsr create needs --subject or --email: a case is about somebody")
	}

	create := openapi.DataSubjectRequestCreate{Kind: openapi.DataSubjectRequestKind(*kind)}
	if *subject != "" {
		subjectID, err := cli.parseID("--subject", *subject)
		if err != nil {
			return err
		}
		create.SubjectAccountId = &subjectID
	}
	if *email != "" {
		address := openapitypes.Email(*email)
		create.SubjectEmail = &address
	}
	if *scope != "" {
		asked := openapi.DataSubjectRequestScope(*scope)
		create.Scope = &asked
	}
	if *target != "" {
		targetID, err := cli.parseID("--target", *target)
		if err != nil {
			return err
		}
		create.TargetId = &targetID
	}
	if *due != "" {
		deadline, err := cli.parseInstant("--due", *due)
		if err != nil {
			return err
		}
		// parseInstant has already established the form, so this cannot fail; it is parsed again
		// rather than kept as a string because the generated payload carries a time.
		parsed, _ := time.Parse(time.RFC3339, deadline)
		create.DueAt = &parsed
	}
	create.Notes = optional(*notes)

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.DataSubjectRequest
	if err := client.Post(ctx, privacyRequestsPath, create, &created); err != nil {
		return err
	}
	return cli.Emit(created, caseTable([]openapi.DataSubjectRequest{created}))
}

func dsrList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "dsr", "ls",
		"[--status <s>] [--kind <k>] [--due-within <days>] [--include-closed] [--size <n>] [--cursor <c>]")
	status := flags.String("status", "", "RECEIVED, IN_PROGRESS, COMPLETED or REJECTED")
	kind := flags.String("kind", "", "the right that was exercised")
	dueWithin := flags.Int("due-within", 0, "only the cases falling due inside that many days, overdue included")
	includeClosed := flags.Bool("include-closed", false, "the answered ones too")
	size := flags.Int("size", 0, "how many per page")
	cursor := flags.String("cursor", "", "continue the previous page")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	query := url.Values{}
	for name, value := range map[string]string{"status": *status, "kind": *kind, "cursor": *cursor} {
		if value != "" {
			query.Set(name, value)
		}
	}
	if *dueWithin > 0 {
		query.Set("due_within_days", strconv.Itoa(*dueWithin))
	}
	if *includeClosed {
		query.Set("include_closed", "true")
	}
	if *size > 0 {
		query.Set("size", strconv.Itoa(*size))
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page casePage
	if err := client.Get(ctx, privacyRequestsPath, query, &page); err != nil {
		return err
	}
	if err := cli.Emit(page, caseTable(page.Data)); err != nil {
		return err
	}
	cli.reportMore(page.Page)
	return nil
}

// casePage is the answer to `GET /privacy/requests`: `data` and a `PageInfo`, as every listing in
// this API has, and inline in the contract like the others of this milestone.
type casePage struct {
	Data []openapi.DataSubjectRequest `json:"data"`
	Page openapi.PageInfo             `json:"page"`
}

// dsrStart is the transition that does the work. `RECEIVED → IN_PROGRESS` is what runs the export
// or the erasure, and the job completes the case - so a CLI with `create` and `complete` and
// nothing between them could raise a case and close it having done nothing at all.
func dsrStart(ctx context.Context, cli *CLI, args []string) error {
	const usage = "dsr start <id> [--mode ANONYMIZE|FULL_DELETE] [--target <id>]"
	requestID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "dsr", "start", "<id> [--mode ANONYMIZE|FULL_DELETE] [--target <id>]")
	mode := flags.String("mode", "", "ANONYMIZE keeps the authorship, FULL_DELETE takes their own contributions too")
	target := flags.String("target", "", "where an access or portability archive is written")
	handledBy := flags.String("handled-by", "", "the account taking the case on")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	update := openapi.DataSubjectRequestUpdate{}
	status := openapi.DataSubjectRequestStatusINPROGRESS
	update.Status = &status
	if *mode != "" {
		erasure := openapi.ErasureMode(*mode)
		update.ErasureMode = &erasure
	}
	if *target != "" {
		targetID, err := cli.parseID("--target", *target)
		if err != nil {
			return err
		}
		update.TargetId = &targetID
	}
	if *handledBy != "" {
		owner, err := cli.parseID("--handled-by", *handledBy)
		if err != nil {
			return err
		}
		update.HandledBy = &owner
	}
	return cli.patchCase(ctx, requestID.String(), update)
}

func dsrComplete(ctx context.Context, cli *CLI, args []string) error {
	const usage = "dsr complete <id>"
	requestID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "dsr", "complete", "<id>")
	notes := flags.String("notes", "", "what was done, for whoever reads the case later")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}

	update := openapi.DataSubjectRequestUpdate{Notes: optional(*notes)}
	status := openapi.DataSubjectRequestStatusCOMPLETED
	update.Status = &status
	return cli.patchCase(ctx, requestID.String(), update)
}

func dsrReject(ctx context.Context, cli *CLI, args []string) error {
	const usage = "dsr reject <id> --reason <why>"
	requestID, rest, err := cli.takeID(args, usage)
	if err != nil {
		return err
	}
	flags := commandFlags(cli, "dsr", "reject", "<id> --reason <why>")
	reason := flags.String("reason", "", "why the case is refused; a refusal without one is not an answer")
	if err := parseOnlyFlags(flags, rest, usage); err != nil {
		return err
	}
	if *reason == "" {
		return usagef("dsr reject needs --reason: a refusal within the period is an answer, silence is not")
	}

	update := openapi.DataSubjectRequestUpdate{RejectionReason: reason}
	status := openapi.DataSubjectRequestStatusREJECTED
	update.Status = &status
	return cli.patchCase(ctx, requestID.String(), update)
}

func (cli *CLI) patchCase(
	ctx context.Context, requestID string, update openapi.DataSubjectRequestUpdate,
) error {
	client, err := cli.client()
	if err != nil {
		return err
	}
	var updated openapi.DataSubjectRequest
	if err := client.Patch(ctx, privacyRequestsPath+"/"+requestID, update, &updated); err != nil {
		return err
	}
	return cli.Emit(updated, caseTable([]openapi.DataSubjectRequest{updated}))
}

func caseTable(cases []openapi.DataSubjectRequest) Table {
	rows := make([][]string, 0, len(cases))
	for _, one := range cases {
		rows = append(rows, []string{
			one.Id.String(),
			string(one.Kind),
			string(one.Status),
			subjectOf(one),
			shortTime(&one.DueAt),
			text(one.ResultArchive),
		})
	}
	return Table{
		Columns: []string{"id", "kind", "status", "subject", "due", "archive"},
		Rows:    rows,
	}
}

// subjectOf is who the case is about. The account identifier once there is one, and the address
// otherwise - and a dash after a full deletion, which is exactly right: the case outlives the
// person's record of themselves, and it has to.
func subjectOf(one openapi.DataSubjectRequest) string {
	if one.SubjectAccountId != nil {
		return one.SubjectAccountId.String()
	}
	if one.SubjectEmail != nil && *one.SubjectEmail != "" {
		return *one.SubjectEmail
	}
	return "-"
}
