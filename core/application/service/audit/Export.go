// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ExportAuditTrailName = "ExportAuditTrail"

	// ExportedAction is the first `audit.*` action this system records about itself, and §5's
	// last line: "Every audit export itself produces an audit entry."
	//
	// A warning rather than an info, for the reason a backup run is one: this is the moment a
	// copy of the evidence leaves the installation, and "who took a copy of the trail on Tuesday,
	// and of which period" is a question with an answer.
	ExportedAction port.Action = "audit.exported"
)

// Format is what an export is written as.
type Format string

const (
	// FormatJSONL keeps the entry as it is - the nested actor, target, context and changes - and
	// is what a second system reads.
	FormatJSONL Format = "JSONL"
	// FormatCSV flattens it for a spreadsheet. What does not fit a column travels as JSON inside
	// one, which is a compromise the format makes rather than a loss of information.
	FormatCSV Format = "CSV"
)

func (f Format) Valid() bool { return f == FormatJSONL || f == FormatCSV }

// extension is what the data member is called.
func (f Format) extension() string {
	if f == FormatCSV {
		return "csv"
	}
	return "jsonl"
}

// Enqueuer is the half of the queue an export needs: it asks for work and never claims any.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// ExportAuditTrail accepts an export and answers the job that will write it.
type ExportAuditTrail struct {
	Jobs       Enqueuer
	Authorizer Authorizer
	Audit      port.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// ExportCommand is the input, typed.
type ExportCommand struct {
	Period   repository.Period
	Format   Format
	TargetID shared.ID
}

// Accepted is what a 202 hands back: the job to poll, and the export it will produce.
type Accepted struct {
	JobID    shared.ID
	ExportID shared.ID
}

// Execute accepts the export.
//
// The whole trail and nothing less. There is no export of one's own events: an export is evidence
// about an interval that somebody will read months from now, and one narrowed to whoever asked for
// it would be evidence about their selection. Somebody who may read only their own events reads
// them through `GET /audit`.
func (h ExportAuditTrail) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ExportCommand,
) (Accepted, error) {
	request := wholeTrail(actor)
	request.Action = ExportedAction
	request.TokenScope = auditExport
	request.TargetID = cmd.TargetID
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return Accepted{}, err
	}

	format := cmd.Format
	if format == "" {
		format = FormatJSONL
	}
	if !format.Valid() {
		return Accepted{}, shared.ErrValidation.
			WithDetail("audit.format_invalid").
			WithParams(map[string]string{"value": string(cmd.Format)}).
			WithFields(shared.FieldError{Path: "/format", Code: "audit.format_invalid"})
	}
	if cmd.TargetID.IsZero() {
		return Accepted{}, shared.ErrValidation.
			WithDetail("audit.export_target_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "audit.export_target_required"})
	}
	if cmd.Period.From.IsZero() || cmd.Period.To.IsZero() {
		return Accepted{}, shared.ErrValidation.
			WithDetail("audit.period_required").
			WithFields(shared.FieldError{Path: "/from", Code: "audit.period_required"})
	}
	if !cmd.Period.To.After(cmd.Period.From) {
		return Accepted{}, shared.ErrValidation.
			WithDetail("audit.period_invalid").
			WithFields(shared.FieldError{Path: "/to", Code: "audit.period_invalid"})
	}

	exportID := h.IDs.NewID()
	now := h.Clock.Now()
	var accepted Accepted

	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		jobID, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAuditExport,
			TenantID: actor.TenantID,
			Payload: map[string]any{
				"export_id": exportID.String(),
				"target_id": cmd.TargetID.String(),
				"from":      cmd.Period.From.UTC().Format(time.RFC3339Nano),
				"to":        cmd.Period.To.UTC().Format(time.RFC3339Nano),
				"format":    string(format),
			},
			DedupeKey: "audit-export:" + exportID.String(),
		})
		if err != nil {
			return err
		}
		accepted = Accepted{JobID: jobID, ExportID: exportID}

		// The entry is written in the same transaction as the job, which is what makes "every
		// export produces an audit entry" true rather than usual: an export that was accepted and
		// left no record would be the one an auditor asks about.
		return h.Audit.Append(ctx, port.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: ExportedAction, Outcome: port.OutcomeSuccess, Severity: port.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: trailTarget, TargetID: exportID,
			Context: port.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: port.Changes(
				port.Change{
					Field: "from", Classification: port.Open,
					To: cmd.Period.From.UTC().Format(time.RFC3339),
				},
				port.Change{
					Field: "to", Classification: port.Open,
					To: cmd.Period.To.UTC().Format(time.RFC3339),
				},
				port.Change{Field: "format", Classification: port.Open, To: string(format)},
				port.Change{
					Field: "target_id", Classification: port.Open, To: cmd.TargetID.String(),
				},
			),
		})
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// Descriptor registers the export in all three channels.
func (h ExportAuditTrail) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ExportAuditTrailName,
		Summary: "Writes a period of the audit trail to a backup target as an archive: the " +
			"entries as JSON Lines or CSV, a manifest naming the period and the stretch of the " +
			"chain it covers, a checksum per member, and a signature where the installation " +
			"holds a key. Answers the job that will write it, because an export over four " +
			"hundred days is not something a request can hold.",
		SideEffects: "Enqueues the job and records the export in the audit trail itself - a copy " +
			"of the evidence leaving the installation is an event an auditor asks about. The " +
			"archive is written to somebody else's machine.",
		TokenScope:  auditExport,
		Destructive: false,
		Input: []usecase.Field{
			{
				Name: "from", Kind: usecase.KindString, Required: true,
				Description: "The start of the period, inclusive. RFC 3339.",
			},
			{
				Name: "to", Kind: usecase.KindString, Required: true,
				Description: "The end of the period, exclusive. RFC 3339.",
			},
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "The backup target the archive is written to.",
			},
			{
				Name: "format", Kind: usecase.KindString,
				Enum:        []string{string(FormatJSONL), string(FormatCSV)},
				Description: "JSON Lines for a second system, CSV for a spreadsheet.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ExportedAction, TargetType: trailTarget,
			Severity: port.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExportAuditTrail) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	from, err := parseInstant(in.String("from"), "from")
	if err != nil {
		return nil, err
	}
	to, err := parseInstant(in.String("to"), "to")
	if err != nil {
		return nil, err
	}
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}

	accepted, err := h.Execute(ctx, actor, ExportCommand{
		Period:   repository.Period{From: from, To: to},
		Format:   Format(in.String("format")),
		TargetID: targetID,
	})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"job_id": accepted.JobID.String(), "export_id": accepted.ExportID.String(),
	}, nil
}

// ExportRequestOf reads the job's payload back into the input the archive writer takes.
//
// Here rather than in the worker, because the payload is this use case's own vocabulary: the row
// outlives the process that wrote it, and the two ends of that have to agree in one place
// (ADR-0008).
func ExportRequestOf(payload map[string]any, tenantID shared.ID) (ArchiveRequest, error) {
	text := func(key string) string {
		value, _ := payload[key].(string)
		return value
	}

	exportID, err := shared.ParseID(text("export_id"))
	if err != nil {
		return ArchiveRequest{}, shared.Internalf("audit: an export job without a readable %s", "export_id")
	}
	targetID, err := shared.ParseID(text("target_id"))
	if err != nil {
		return ArchiveRequest{}, shared.Internalf("audit: an export job without a readable %s", "target_id")
	}
	from, err := time.Parse(time.RFC3339Nano, text("from"))
	if err != nil {
		return ArchiveRequest{}, shared.Internalf("audit: an export job without a readable %s", "from")
	}
	to, err := time.Parse(time.RFC3339Nano, text("to"))
	if err != nil {
		return ArchiveRequest{}, shared.Internalf("audit: an export job without a readable %s", "to")
	}

	format := Format(text("format"))
	if !format.Valid() {
		return ArchiveRequest{}, shared.Internalf("audit: an export job naming an unknown format")
	}

	return ArchiveRequest{
		ExportID: exportID, TenantID: tenantID, TargetID: targetID,
		Period: repository.Period{From: from, To: to}, Format: format,
	}, nil
}
