// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// An installation-wide extension is recorded in every workspace the person is a member of
// (data-protection.md §4.1). One job per workspace, enqueued in the extension's own transaction:
// a loop after the commit would lose the entries a failure mid-loop never wrote, and a retry would
// meet a case already extended. Each job writes its entry in the transaction the runner opens in
// that workspace's scope, together with the job's completion, so a run that fails after the write
// is rolled back whole and its retry stores the one entry.
//
// The job decides nothing (rule 2): the operator's right was checked when the case was extended,
// and the payload carries who it was so the entry can say so.

// ExtensionEntry is one workspace's entry, as the extension hands it to the job.
type ExtensionEntry struct {
	TenantID      shared.ID
	RequestID     shared.ID
	Kind          domain.Kind
	OriginalDueAt time.Time
	DueAt         time.Time
	Reason        domain.ExtensionReason
	InformedOn    domain.Day
	ActorKind     appshared.ActorKind
	ActorID       shared.ID
	ActorLabel    string
}

// fanOut enqueues one entry job for every workspace the person of an installation-wide case is a
// member of, other than the case's own, which the extension records itself. A case with no address
// names nobody anywhere else: an account identifier belongs to one workspace by construction.
func (h ExtendDataSubjectRequest) fanOut(
	ctx context.Context, actor appshared.ActorContext, extended domain.Request,
) error {
	if extended.Scope != domain.ScopeInstallation || extended.SubjectEmail == "" {
		return nil
	}
	tenants, err := h.Cases.Subjects.Tenants(ctx, extended.SubjectEmail)
	if err != nil {
		return err
	}
	for _, tenant := range tenants {
		if tenant == actor.TenantID {
			continue
		}
		entry := ExtensionEntry{
			TenantID: tenant, RequestID: extended.ID, Kind: extended.Kind,
			OriginalDueAt: extended.OriginalDueAt, DueAt: extended.DueAt,
			Reason: extended.ExtensionReason, InformedOn: extended.InformedOn,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		}
		if _, err := h.Cases.Jobs.Enqueue(ctx, queue.Request{
			Kind:      queue.KindPrivacyExtensionEntry,
			TenantID:  tenant,
			Payload:   entry.payload(),
			DedupeKey: "dsr-extended:" + extended.ID.String() + ":" + tenant.String(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (e ExtensionEntry) payload() map[string]any {
	return map[string]any{
		"request_id":       e.RequestID.String(),
		"kind":             string(e.Kind),
		"original_due_at":  e.OriginalDueAt.UTC().Format(time.RFC3339),
		"due_at":           e.DueAt.UTC().Format(time.RFC3339),
		"extension_reason": string(e.Reason),
		"informed_on":      e.InformedOn.String(),
		"actor_kind":       string(e.ActorKind),
		"actor_id":         e.ActorID.String(),
		"actor_label":      e.ActorLabel,
	}
}

// ExtensionEntryOf reads a job's payload back. A payload that does not read is the system's own
// fault rather than anybody's input, so it fails as an internal error and the job dead-letters.
func ExtensionEntryOf(payload map[string]any, tenantID shared.ID) (ExtensionEntry, error) {
	text := func(key string) string {
		value, _ := payload[key].(string)
		return value
	}
	malformed := shared.Internalf("privacy: an extension entry job without a readable payload")

	requestID, err := shared.ParseID(text("request_id"))
	if err != nil {
		return ExtensionEntry{}, malformed
	}
	original, err := time.Parse(time.RFC3339, text("original_due_at"))
	if err != nil {
		return ExtensionEntry{}, malformed
	}
	due, err := time.Parse(time.RFC3339, text("due_at"))
	if err != nil {
		return ExtensionEntry{}, malformed
	}
	informed, err := domain.ParseDay(text("informed_on"))
	if err != nil {
		return ExtensionEntry{}, malformed
	}
	entry := ExtensionEntry{
		TenantID: tenantID, RequestID: requestID, Kind: domain.Kind(text("kind")),
		OriginalDueAt: original, DueAt: due, Reason: domain.ExtensionReason(text("extension_reason")),
		InformedOn: informed, ActorKind: appshared.ActorKind(text("actor_kind")),
		ActorLabel: text("actor_label"),
	}
	if raw := text("actor_id"); raw != "" {
		if entry.ActorID, err = shared.ParseID(raw); err != nil {
			return ExtensionEntry{}, malformed
		}
	}
	return entry, nil
}

// RecordExtensionEntry writes one workspace's entry.
type RecordExtensionEntry struct {
	Workspaces Workspaces
	Audit      audit.Sink
	Clock      clock.Clock
}

// Execute runs inside the transaction the caller opened in the entry's workspace, and answers
// whether an entry was written. A workspace that is suspended or awaiting deletion is written to
// all the same - the trail is that workspace's evidence whatever its state; one that is gone gets
// nothing, and the job is done.
func (r RecordExtensionEntry) Execute(ctx context.Context, entry ExtensionEntry) (bool, error) {
	if _, err := r.Workspaces.Find(ctx); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	err := r.Audit.Append(ctx, audit.Entry{
		TenantID: entry.TenantID, OccurredAt: r.Clock.Now(),
		Action: RequestExtendedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: entry.ActorKind, ActorID: entry.ActorID, ActorLabel: entry.ActorLabel,
		TargetType: requestTarget, TargetID: entry.RequestID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(extensionChanges(domain.Request{
			Kind: entry.Kind, OriginalDueAt: entry.OriginalDueAt, DueAt: entry.DueAt,
			ExtensionReason: entry.Reason, InformedOn: entry.InformedOn,
		})...),
		LegalBasis: LegalBasisOf(entry.Kind),
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
