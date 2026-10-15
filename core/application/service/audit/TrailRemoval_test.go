// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"context"
	"encoding/json"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type workspaceLookup struct{ err error }

func (w workspaceLookup) Find(context.Context) (identity.Workspace, error) {
	return identity.Workspace{}, w.err
}

// The payload survives the queue's JSON round trip, where a count comes back as a float.
func TestATrailRemovalReadsBackFromItsPayload(t *testing.T) {
	removal := TrailRemoval{TenantID: tenantID, Partition: "audit_log_2025_05", Month: "2025-05", Entries: 12}
	encoded, err := json.Marshal(removal.Payload())
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	read, err := TrailRemovalOf(payload, tenantID)
	if err != nil || read != removal {
		t.Fatalf("read back %+v (%v), want %+v", read, err, removal)
	}
	if _, err := TrailRemovalOf(payload, ""); err == nil {
		t.Error("a payload without its workspace was accepted")
	}
	if _, err := TrailRemovalOf(map[string]any{"partition": "audit_log_2025_05"}, tenantID); err == nil {
		t.Error("a payload without its month was accepted")
	}
}

// The workspace's trail records the month that went: by the installation, with the month and the
// count and nothing an entry said.
func TestARemovedMonthIsRecordedInTheWorkspacesTrail(t *testing.T) {
	sink := &auditSink{}
	record := RecordTrailRemoval{Workspaces: workspaceLookup{}, Audit: sink, Clock: clock.Fixed(now)}

	written, err := record.Execute(t.Context(), TrailRemoval{
		TenantID: tenantID, Partition: "audit_log_2025_05", Month: "2025-05", Entries: 12,
	})
	if err != nil || !written || len(sink.entries) != 1 {
		t.Fatalf("written %v, %d entries (%v)", written, len(sink.entries), err)
	}
	entry := sink.entries[0]
	if entry.Action != TrailRemovedAction || entry.ActorKind != appshared.ActorSystem ||
		entry.TenantID != tenantID || !entry.OccurredAt.Equal(now) {
		t.Errorf("recorded %+v", entry)
	}
	fields := map[string]any{}
	for field, change := range entry.Changes {
		if value, ok := change.(map[string]any); ok {
			fields[field] = value["to"]
		}
	}
	if fields["month"] != "2025-05" || fields["entries"] != int64(12) || len(fields) != 2 {
		t.Errorf("the entry says %v", fields)
	}
}

// A workspace deleted since has no trail to write into; that is not a failure.
func TestARemovalForAGoneWorkspaceWritesNothing(t *testing.T) {
	sink := &auditSink{}
	record := RecordTrailRemoval{Workspaces: workspaceLookup{err: shared.ErrNotFound}, Audit: sink, Clock: clock.Fixed(now)}

	written, err := record.Execute(t.Context(), TrailRemoval{
		TenantID: tenantID, Partition: "audit_log_2025_05", Month: "2025-05", Entries: 1,
	})
	if err != nil || written || len(sink.entries) != 0 {
		t.Errorf("written %v, %d entries (%v)", written, len(sink.entries), err)
	}
}
