// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// Accepting an export (E-09, audit.md §5). The archive itself is written by the job; what is under
// test here is who may ask for one, what is refused, and the entry §5's last line requires.

type queueDouble struct {
	requests []queue.Request
	err      error
}

func (q *queueDouble) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	if q.err != nil {
		return "", q.err
	}
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"), nil
}

func newExportHarness() (ExportAuditTrail, *queueDouble, *auditSink, *authorizerDouble) {
	jobs := &queueDouble{}
	sink := &auditSink{}
	authorizer := &authorizerDouble{permits: true}
	return ExportAuditTrail{
		Jobs: jobs, Authorizer: authorizer, Audit: sink, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: &idSource{},
	}, jobs, sink, authorizer
}

func exportCommand(change func(*ExportCommand)) ExportCommand {
	cmd := ExportCommand{
		Period:   repository.Period{From: now.Add(-30 * 24 * time.Hour), To: now},
		Format:   FormatJSONL,
		TargetID: targetID,
	}
	change(&cmd)
	return cmd
}

func TestAnExportIsQueuedAndRecordsItself(t *testing.T) {
	export, jobs, sink, authorizer := newExportHarness()

	accepted, err := export.Execute(context.Background(), actor(), exportCommand(func(*ExportCommand) {}))
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}

	if len(jobs.requests) != 1 {
		t.Fatalf("%d jobs were enqueued", len(jobs.requests))
	}
	request := jobs.requests[0]
	if request.Kind != queue.KindAuditExport || request.TenantID != tenantID {
		t.Errorf("the job was enqueued as %+v", request)
	}
	if request.Payload["export_id"] != accepted.ExportID.String() {
		t.Errorf("the payload names the export %v", request.Payload["export_id"])
	}
	if request.Payload["format"] != string(FormatJSONL) {
		t.Errorf("the payload names the format %v", request.Payload["format"])
	}

	// §5's last line: every audit export produces an audit entry of its own. It is the first
	// `audit.*` action the system records about itself.
	if len(sink.entries) != 1 {
		t.Fatalf("an export wrote %d entries", len(sink.entries))
	}
	entry := sink.entries[0]
	if entry.Action != ExportedAction || entry.Severity != port.SeverityWarning {
		t.Errorf("the export was recorded as %s / %s", entry.Action, entry.Severity)
	}
	if entry.TargetID != accepted.ExportID {
		t.Errorf("the entry names the target %s", entry.TargetID)
	}
	// The period is in the entry, because "who took a copy of the trail, and of which period" is
	// the question the entry exists to answer.
	changes, _ := entry.Changes["from"].(map[string]any)
	if changes["to"] == nil {
		t.Errorf("the entry does not carry the period: %v", entry.Changes)
	}

	if authorizer.requests[0].TokenScope != auditExport {
		t.Errorf("the export asked for the scope %q", authorizer.requests[0].TokenScope)
	}
}

// An export is evidence about an interval. There is no export of one's own events - somebody who
// may read only those reads them through the list.
func TestAnExportNeedsTheWholeTrail(t *testing.T) {
	export, jobs, sink, authorizer := newExportHarness()
	authorizer.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := export.Execute(context.Background(), actor(), exportCommand(func(*ExportCommand) {})); err == nil {
		t.Fatal("an export was accepted from somebody who may not read the whole trail")
	}
	if len(jobs.requests) != 0 || len(sink.entries) != 0 {
		t.Errorf("a refused export enqueued %d jobs and wrote %d entries",
			len(jobs.requests), len(sink.entries))
	}
	if authorizer.requests[0].Permission != wholeTrailRequest().Permission {
		t.Errorf("the export asked for %s", authorizer.requests[0].Permission)
	}
}

func TestAnExportRefusesWhatItCannotWrite(t *testing.T) {
	cases := map[string]func(*ExportCommand){
		"no target": func(c *ExportCommand) { c.TargetID = "" },
		"no period": func(c *ExportCommand) { c.Period = repository.Period{} },
		"no end":    func(c *ExportCommand) { c.Period.To = time.Time{} },
		"an end before a start": func(c *ExportCommand) {
			c.Period = repository.Period{From: now, To: now.Add(-time.Hour)}
		},
		"a format nobody writes": func(c *ExportCommand) { c.Format = "PARQUET" },
	}

	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			export, jobs, sink, _ := newExportHarness()

			if _, err := export.Execute(context.Background(), actor(), exportCommand(change)); err == nil {
				t.Fatal("the export was accepted")
			}
			if len(jobs.requests) != 0 || len(sink.entries) != 0 {
				t.Error("a refused export left something behind")
			}
		})
	}
}

// The format is optional and JSON Lines is what an omitted one means: it is the one that keeps the
// entry as it is, and a caller who did not choose is not asking for a flattening.
func TestAnExportWithoutAFormatIsJSONLines(t *testing.T) {
	export, jobs, _, _ := newExportHarness()

	if _, err := export.Execute(context.Background(), actor(), exportCommand(func(c *ExportCommand) {
		c.Format = ""
	})); err != nil {
		t.Fatalf("exporting: %v", err)
	}
	if jobs.requests[0].Payload["format"] != string(FormatJSONL) {
		t.Errorf("the format defaulted to %v", jobs.requests[0].Payload["format"])
	}
}

// The job's payload is this use case's own vocabulary, and the row outlives the process that wrote
// it - so the two ends of it are tested against each other rather than by reading.
func TestThePayloadReadsBackAsTheRequestThatWroteIt(t *testing.T) {
	export, jobs, _, _ := newExportHarness()

	accepted, err := export.Execute(context.Background(), actor(), exportCommand(func(c *ExportCommand) {
		c.Format = FormatCSV
	}))
	if err != nil {
		t.Fatalf("exporting: %v", err)
	}

	request, err := ExportRequestOf(jobs.requests[0].Payload, tenantID)
	if err != nil {
		t.Fatalf("reading the payload back: %v", err)
	}
	switch {
	case request.ExportID != accepted.ExportID:
		t.Errorf("the export came back as %s", request.ExportID)
	case request.TargetID != targetID:
		t.Errorf("the target came back as %s", request.TargetID)
	case request.Format != FormatCSV:
		t.Errorf("the format came back as %s", request.Format)
	case !request.Period.To.Equal(now):
		t.Errorf("the period came back as %s..%s", request.Period.From, request.Period.To)
	}
}

func TestAPayloadThatCannotBeReadFailsTheJobRatherThanExportingSomethingElse(t *testing.T) {
	for _, payload := range []map[string]any{
		{},
		{"export_id": "not-an-id"},
		{"export_id": targetID.String(), "target_id": targetID.String(), "from": "yesterday"},
		{
			"export_id": targetID.String(), "target_id": targetID.String(),
			"from": now.Format(time.RFC3339Nano), "to": now.Format(time.RFC3339Nano),
			"format": "PARQUET",
		},
	} {
		if _, err := ExportRequestOf(payload, tenantID); err == nil {
			t.Errorf("the payload %v was read as an export", payload)
		}
	}
}

func TestTheExportDescriptorTakesWhatTheControllerSends(t *testing.T) {
	descriptor := ExportAuditTrail{}.Descriptor()

	if err := descriptor.ValidateInput(usecase.Input{
		"from":      now.Format(time.RFC3339Nano),
		"to":        now.Add(time.Hour).Format(time.RFC3339Nano),
		"target_id": targetID.String(),
		"format":    string(FormatCSV),
	}); err != nil {
		t.Fatalf("the input the REST controller builds is refused: %v", err)
	}
	// It writes, so SG-13 requires the declaration rather than leaving it to a judgement.
	if descriptor.ReadOnly || !descriptor.Audit.Required {
		t.Error("the export is declared as a read, or without its audit obligation")
	}
	if descriptor.TokenScope != auditExport {
		t.Errorf("the export declares the scope %q", descriptor.TokenScope)
	}
}
