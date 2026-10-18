// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package platform

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

type log struct {
	events    []event.Envelope
	keys      map[string]bool
	forgotten int
}

func (l *log) Append(_ context.Context, envelope event.Envelope, key string) (bool, error) {
	if key != "" {
		if l.keys[key] {
			return false, nil
		}
		l.keys[key] = true
	}
	l.events = append(l.events, envelope)
	return true, nil
}

func (l *log) After(context.Context, int64, int) ([]adminrepo.PlatformEvent, error) { return nil, nil }
func (l *log) ForgetReference(context.Context) error                                { l.forgotten++; return nil }
func (l *log) Sweep(context.Context, time.Time, int) (int, error)                   { return 0, nil }

type row struct {
	record adminrepo.TenantRecord
	err    error
}

func (r row) Find(context.Context) (adminrepo.TenantRecord, error) { return r.record, r.err }

type ids struct{ n int }

func (i *ids) NewID() shared.ID {
	i.n++
	return shared.ID("0192f000-0000-7000-8000-" + string(rune('a'+i.n)) + "00000000000")
}

var (
	workspace = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	at        = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	maria     = adminrepo.TenantRecord{
		ID: workspace, Slug: "familie-berger", DisplayName: "Familie Berger", Status: identity.TenantSuspended,
		PlanName: "familie", ExternalReference: "cus_4711", Version: 7,
	}
)

func recorder(r row) (Recorder, *log) {
	l := &log{keys: map[string]bool{}}
	return Recorder{Events: l, Workspaces: r, IDs: &ids{}, Tenancy: env.TenancyMulti}, l
}

func TestAnEventCarriesTheRowAsTheChangeLeftIt(t *testing.T) {
	rec, l := recorder(row{record: maria})
	if err := rec.Record(t.Context(), event.WorkspaceSuspended, event.Actor{Kind: shared.ActorUser}, at, nil); err != nil {
		t.Fatal(err)
	}
	if len(l.events) != 1 {
		t.Fatalf("wrote %d events", len(l.events))
	}
	state := l.events[0].Payload["workspace"].(map[string]any)
	want := map[string]any{
		"id": workspace.String(), "external_reference": "cus_4711", "status": "SUSPENDED",
		"plan": "familie", "version": int64(7),
	}
	for key, value := range want {
		if state[key] != value {
			t.Errorf("%s = %v, want %v", key, state[key], value)
		}
	}
}

func TestAWriterThatRunsTwiceWritesOnce(t *testing.T) {
	rec, l := recorder(row{record: maria})
	for range 2 {
		if _, err := rec.RecordOnce(t.Context(), event.WorkspaceExportFinished, event.Actor{Kind: shared.ActorSystem},
			at, map[string]any{"export_id": "x"}, "export:x"); err != nil {
			t.Fatal(err)
		}
	}
	if len(l.events) != 1 {
		t.Errorf("a repeated writer wrote %d events", len(l.events))
	}
}

// The purge's event: DELETED, the version one higher, no reference - and the reference removed from
// the workspace's kept events in the same breath (multi-tenancy.md §4.3).
func TestThePurgeIsToldWithoutTheReference(t *testing.T) {
	rec, l := recorder(row{})
	if err := rec.RecordDeleted(t.Context(), maria, event.Actor{Kind: shared.ActorSystem}, at); err != nil {
		t.Fatal(err)
	}
	if l.forgotten != 1 || len(l.events) != 1 || l.events[0].Type != event.WorkspaceDeleted {
		t.Fatalf("forgot %d times, wrote %v", l.forgotten, l.events)
	}
	state := l.events[0].Payload["workspace"].(map[string]any)
	if state["status"] != event.WorkspaceDeletedStatus || state["version"] != int64(8) ||
		state["external_reference"] != nil {
		t.Errorf("deleted with %v", state)
	}
}

func TestNothingIsWrittenWithoutTheFeed(t *testing.T) {
	cases := map[string]Recorder{
		"the zero value": {},
		"single mode":    {Events: &log{}, Workspaces: row{record: maria}, IDs: &ids{}, Tenancy: env.TenancySingle},
	}
	for name, rec := range cases {
		t.Run(name, func(t *testing.T) {
			if err := rec.Record(t.Context(), event.WorkspaceResumed, event.Actor{}, at, nil); err != nil {
				t.Fatal(err)
			}
			if err := rec.RecordDeleted(t.Context(), maria, event.Actor{}, at); err != nil {
				t.Fatal(err)
			}
			if l, ok := rec.Events.(*log); ok && (len(l.events) != 0 || l.forgotten != 0) {
				t.Errorf("wrote %d events", len(l.events))
			}
		})
	}
}

func TestAWorkspaceThatCannotBeReadFailsTheChange(t *testing.T) {
	rec, _ := recorder(row{err: errors.New("away")})
	if err := rec.Record(t.Context(), event.WorkspaceResumed, event.Actor{}, at, nil); err == nil {
		t.Error("an unreadable workspace was told about")
	}
}
