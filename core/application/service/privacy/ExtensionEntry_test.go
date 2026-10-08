// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// An installation-wide extension, recorded in every workspace the person is a member of
// (UC-PRV-01 check 10). What the extension enqueues, and what one job writes. That the job's write
// and its completion are one transaction is the runner's, proved against PostgreSQL in
// test/integration.

var (
	otherTenant = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	thirdTenant = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
)

func entryJobs(jobs []queue.Request) []queue.Request {
	var found []queue.Request
	for _, job := range jobs {
		if job.Kind == queue.KindPrivacyExtensionEntry {
			found = append(found, job)
		}
	}
	return found
}

func TestAnInstallationWideExtensionQueuesAnEntryForEveryOtherWorkspace(t *testing.T) {
	h := newHarness()
	h.subjects.tenants = []shared.ID{tenantID, otherTenant, thirdTenant}
	id := h.recorded(t, operator(), domain.ScopeInstallation).String("id")

	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, operator(), extensionInput(id)); err != nil {
		t.Fatalf("extending: %v", err)
	}

	jobs := entryJobs(h.jobs.requests)
	if len(jobs) != 2 {
		t.Fatalf("%d entry jobs were queued, want one for each of the two other workspaces", len(jobs))
	}
	for i, tenant := range []shared.ID{otherTenant, thirdTenant} {
		job := jobs[i]
		if job.TenantID != tenant || job.DedupeKey != "dsr-extended:"+id+":"+tenant.String() {
			t.Errorf("job %d is for %s under %q", i, job.TenantID, job.DedupeKey)
		}
		entry, err := ExtensionEntryOf(job.Payload, job.TenantID)
		if err != nil {
			t.Fatalf("reading the payload back: %v", err)
		}
		if entry.RequestID.String() != id || entry.Reason != domain.ReasonComplexity ||
			entry.InformedOn.String() != "2026-08-26" || entry.ActorID != accountID ||
			entry.ActorLabel != "Anna Beispiel" || entry.DueAt.IsZero() || entry.OriginalDueAt.IsZero() {
			t.Errorf("the payload reads back as %+v", entry)
		}
	}
}

// A case with no address names nobody anywhere else: no question across workspaces, no job.
func TestAnInstallationWideCaseWithoutAnAddressQueuesNothing(t *testing.T) {
	h := newHarness()
	h.subjects.tenants = []shared.ID{otherTenant}
	out, err := h.registry(t).Invoke(t.Context(), CreateDataSubjectRequestName, operator(), usecase.Input{
		"kind": string(domain.KindAccess), "scope": string(domain.ScopeInstallation),
		"subject_account_id": subjectID.String(),
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, operator(), extensionInput(out.String("id"))); err != nil {
		t.Fatalf("extending: %v", err)
	}
	if h.subjects.asked != 0 || len(entryJobs(h.jobs.requests)) != 0 {
		t.Errorf("asked %d times, queued %d jobs", h.subjects.asked, len(entryJobs(h.jobs.requests)))
	}
}

// A workspace's own case is recorded by the extension itself and asks no other workspace.
func TestAWorkspacesOwnCaseQueuesNothing(t *testing.T) {
	h := newHarness()
	h.subjects.tenants = []shared.ID{otherTenant}
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")
	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), extensionInput(id)); err != nil {
		t.Fatalf("extending: %v", err)
	}
	if h.subjects.asked != 0 || len(entryJobs(h.jobs.requests)) != 0 {
		t.Errorf("asked %d times, queued %d jobs", h.subjects.asked, len(entryJobs(h.jobs.requests)))
	}
}

func TestTheJobWritesTheEntryInItsWorkspace(t *testing.T) {
	h := newHarness()
	h.subjects.tenants = []shared.ID{otherTenant}
	id := h.recorded(t, operator(), domain.ScopeInstallation).String("id")
	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, operator(), extensionInput(id)); err != nil {
		t.Fatalf("extending: %v", err)
	}
	job := entryJobs(h.jobs.requests)[0]
	entry, err := ExtensionEntryOf(job.Payload, job.TenantID)
	if err != nil {
		t.Fatalf("reading the payload: %v", err)
	}

	sink := &auditSink{}
	written, err := RecordExtensionEntry{
		Workspaces: &workspaceDouble{zone: "UTC"}, Audit: sink, Clock: clock.Fixed(now),
	}.Execute(t.Context(), entry)
	if err != nil || !written {
		t.Fatalf("writing the entry: %v (%v)", err, written)
	}
	recorded := sink.entries[0]
	if recorded.TenantID != otherTenant || recorded.Action != RequestExtendedAction ||
		recorded.TargetID.String() != id || recorded.ActorID != accountID ||
		recorded.ActorLabel != "Anna Beispiel" || recorded.LegalBasis != "dsr.access" ||
		recorded.Severity != audit.SeverityNotice {
		t.Errorf("the entry is %+v", recorded)
	}
	reason, _ := recorded.Changes["extension_reason"].(map[string]any)
	if reason["to"] != "COMPLEXITY" {
		t.Errorf("the entry records %v", recorded.Changes)
	}

	// A workspace that is gone gets nothing, and the job is done rather than failed.
	gone := &auditSink{}
	written, err = RecordExtensionEntry{
		Workspaces: &workspaceDouble{gone: true}, Audit: gone, Clock: clock.Fixed(now),
	}.Execute(t.Context(), entry)
	if err != nil || written || len(gone.entries) != 0 {
		t.Errorf("a gone workspace answered %v, %v, %d entries", err, written, len(gone.entries))
	}
}

func TestAPayloadThatDoesNotReadFailsTheJob(t *testing.T) {
	complete := ExtensionEntry{
		RequestID: shared.MustParseID("0192f000-0000-7000-8000-0000000000c9"), Kind: domain.KindAccess,
		OriginalDueAt: now, DueAt: now.Add(48 * time.Hour), Reason: domain.ReasonComplexity,
		InformedOn: domain.DayOf(now, now.Location()), ActorID: accountID,
	}.payload()
	for _, field := range []string{"request_id", "original_due_at", "due_at", "informed_on", "actor_id"} {
		broken := map[string]any{}
		for key, value := range complete {
			broken[key] = value
		}
		broken[field] = "not that"
		if _, err := ExtensionEntryOf(broken, otherTenant); shared.AsError(err).Code != shared.ErrInternal.Code {
			t.Errorf("a payload with %s broken was read: %v", field, err)
		}
	}
}
