// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	changed   = created.Add(time.Hour)
	archived  = created.Add(-time.Hour)
	otherHub  = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	collectID = shared.MustParseID("0192f000-0000-7000-8000-00000000000f")
)

// collection is a stored collection, as a read would hand it up.
func collection() work.Container {
	return work.Container{
		ID: collectID, TenantID: tenant, Type: work.ContainerCollection, ParentID: hubID,
		Name: "Shopping", OrderKey: "m", CompletionPolicy: work.CompletionManual,
		CreatedBy: actorID, CreatedAt: created, UpdatedAt: created, Version: 3,
	}
}

func hub() work.Container {
	return work.Container{
		ID: hubID, TenantID: tenant, Type: work.ContainerHub, Name: "Private", OrderKey: "m",
		CompletionPolicy: work.CompletionManual, CreatedBy: actorID, CreatedAt: created,
		UpdatedAt: created, Version: 1,
	}
}

func text(value string) *string { return &value }

// I-C3, both halves: a container is read-only when it carries the stamp, and when the hub above it
// does. The two facts stay separate on the row, which is what lets a client be told which of them
// to lift.
func TestEffectiveArchiving(t *testing.T) {
	cases := []struct {
		name        string
		own, parent *time.Time
		effective   bool
		root        shared.ID
	}{
		{name: "neither", effective: false, root: ""},
		{name: "its own", own: &archived, effective: true, root: collectID},
		{name: "inherited from the hub", parent: &archived, effective: true, root: hubID},
		{name: "both, its own is the more specific", own: &archived, parent: &archived, effective: true, root: collectID},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			container := collection()
			container.ArchivedAt = c.own
			container.ParentArchivedAt = c.parent

			if got := container.IsEffectivelyArchived(); got != c.effective {
				t.Errorf("effectively archived %v, want %v", got, c.effective)
			}
			if got := container.IsArchived(); got != (c.own != nil) {
				t.Errorf("IsArchived reports the inherited stamp: %v", got)
			}
			if got := container.ArchivedRootID(); got != c.root {
				t.Errorf("archived root %q, want %q", got, c.root)
			}
			if c.effective && container.EffectiveArchivedAt() == nil {
				t.Error("effectively archived and no timestamp saying since when")
			}
		})
	}
}

// A collection whose hub is archived refuses items without carrying a stamp of its own. Without
// this the invariant would hold only for the container a client happened to archive directly.
func TestAnArchivedHubMakesItsCollectionReadOnly(t *testing.T) {
	container := collection()
	container.ParentArchivedAt = &archived

	err := container.EnsureAcceptsItems()
	assertDetail(t, err, shared.ErrConflict, "items.collection_archived")
	if got := shared.AsError(err).Params["archived_id"]; got != hubID.String() {
		t.Errorf("the refusal names %q as the archived thing, want the hub", got)
	}
	assertDetail(t, container.EnsureEditable(), shared.ErrConflict, "containers.archived")
}

func TestRenamedReportsOnlyWhatMoved(t *testing.T) {
	cases := []struct {
		name       string
		attributes work.ContainerAttributes
		want       map[string]string
	}{
		{
			name:       "the name alone",
			attributes: work.ContainerAttributes{Name: text("Groceries")},
			want:       map[string]string{"name": "Groceries"},
		},
		{
			name:       "trimmed, and the trimmed form is what is compared",
			attributes: work.ContainerAttributes{Name: text("  Shopping  ")},
			want:       map[string]string{},
		},
		{
			name:       "a description arrives",
			attributes: work.ContainerAttributes{Description: text("Weekly")},
			want:       map[string]string{"description": "Weekly"},
		},
		{
			name:       "sending what is already stored moves nothing",
			attributes: work.ContainerAttributes{Name: text("Shopping"), Icon: text("")},
			want:       map[string]string{},
		},
		{
			name: "three fields at once",
			attributes: work.ContainerAttributes{
				Name: text("Groceries"), Icon: text("basket"), ColorToken: text("green"),
			},
			want: map[string]string{"name": "Groceries", "icon": "basket", "color_token": "green"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			updated, changes, err := collection().Renamed(c.attributes, changed)
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if len(changes) != len(c.want) {
				t.Fatalf("changes %+v, want %d of them", changes, len(c.want))
			}
			for _, change := range changes {
				if want, ok := c.want[change.Field]; !ok || change.To != want {
					t.Errorf("change %+v is not among the expected %+v", change, c.want)
				}
			}
			if len(changes) == 0 && updated.UpdatedAt != created {
				t.Error("a request that changed nothing spent an update timestamp")
			}
			if len(changes) > 0 && updated.UpdatedAt != changed {
				t.Errorf("updated at %v, want the clock's %v", updated.UpdatedAt, changed)
			}
		})
	}
}

// Clearing a field is a request in its own right, and it has to survive the round trip as one.
func TestRenamedClearsAFieldThatIsSentEmpty(t *testing.T) {
	container := collection()
	container.Description = "Weekly"

	updated, changes, err := container.Renamed(work.ContainerAttributes{Description: text("")}, changed)
	if err != nil {
		t.Fatalf("clearing the description was refused: %v", err)
	}
	if updated.Description != "" {
		t.Errorf("description %q, want it cleared", updated.Description)
	}
	if len(changes) != 1 || changes[0].From != "Weekly" || changes[0].To != "" {
		t.Errorf("the change does not describe the clearing: %+v", changes)
	}
}

func TestRenamedChecksEveryValueItStores(t *testing.T) {
	cases := []struct {
		name       string
		attributes work.ContainerAttributes
		detailCode string
		path       string
	}{
		{
			name:       "an empty name",
			attributes: work.ContainerAttributes{Name: text("   ")},
			detailCode: "containers.name_empty", path: "/name",
		},
		{
			name:       "a name over the limit",
			attributes: work.ContainerAttributes{Name: text(strings.Repeat("a", 201))},
			detailCode: "containers.name_too_long", path: "/name",
		},
		{
			name:       "a newline in the name",
			attributes: work.ContainerAttributes{Name: text("Shop\nping")},
			detailCode: "containers.name_malformed", path: "/name",
		},
		{
			name:       "a newline in the icon",
			attributes: work.ContainerAttributes{Icon: text("bas\nket")},
			detailCode: "containers.field_malformed", path: "/icon",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := collection().Renamed(c.attributes, changed)
			assertDetail(t, err, shared.ErrValidation, c.detailCode)
			if fields := shared.AsError(err).Fields; len(fields) != 1 || fields[0].Path != c.path {
				t.Errorf("the finding does not point at %s: %+v", c.path, fields)
			}
		})
	}
}

// I-C3 as a write sees it: the read-only state refuses the rename whether it is this container's
// own or the hub's.
func TestRenamedRefusesAReadOnlyContainer(t *testing.T) {
	for _, c := range []struct {
		name       string
		prepare    func(*work.Container)
		detailCode string
	}{
		{
			name:       "archived itself",
			prepare:    func(c *work.Container) { c.ArchivedAt = &archived },
			detailCode: "containers.archived",
		},
		{
			name:       "in an archived hub",
			prepare:    func(c *work.Container) { c.ParentArchivedAt = &archived },
			detailCode: "containers.archived",
		},
		{
			name:       "in the trash",
			prepare:    func(c *work.Container) { c.DeletedAt = &archived },
			detailCode: "containers.trashed",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			container := collection()
			c.prepare(&container)

			_, _, err := container.Renamed(work.ContainerAttributes{Name: text("Groceries")}, changed)
			assertDetail(t, err, shared.ErrConflict, c.detailCode)
		})
	}
}

func TestWithPolicies(t *testing.T) {
	t.Run("the policy moves", func(t *testing.T) {
		updated, changes, err := collection().WithPolicies(
			work.ContainerPolicies{CompletionPolicy: work.CompletionRollup}, changed)
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if updated.CompletionPolicy != work.CompletionRollup {
			t.Errorf("policy %q, want ROLLUP", updated.CompletionPolicy)
		}
		if len(changes) != 1 || changes[0].Field != "completion_policy" || changes[0].From != "MANUAL" {
			t.Errorf("the change does not describe the move: %+v", changes)
		}
	})

	// A PUT replaces. A key that is not sent is the default, not whatever happens to be stored -
	// otherwise the document would partly remember what it replaced.
	t.Run("an absent key is the default, not the stored value", func(t *testing.T) {
		container := collection()
		container.CompletionPolicy = work.CompletionRollup

		updated, changes, err := container.WithPolicies(work.ContainerPolicies{}, changed)
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if updated.CompletionPolicy != work.CompletionManual || len(changes) != 1 {
			t.Errorf("policy %q with changes %+v, want the default back", updated.CompletionPolicy, changes)
		}
	})

	t.Run("writing what is already stored changes nothing", func(t *testing.T) {
		updated, changes, err := collection().WithPolicies(
			work.ContainerPolicies{CompletionPolicy: work.CompletionManual}, changed)
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if len(changes) != 0 || updated.UpdatedAt != created {
			t.Errorf("a repeat spent a version: %+v at %v", changes, updated.UpdatedAt)
		}
	})

	t.Run("an unknown policy", func(t *testing.T) {
		_, _, err := collection().WithPolicies(work.ContainerPolicies{CompletionPolicy: "AUTO"}, changed)
		assertDetail(t, err, shared.ErrValidation, "containers.completion_policy_unknown")
	})

	// A hub holds collections and no items, so a completion policy on one would decide nothing. It
	// is refused rather than stored, because a setting that never takes effect is worse than none.
	t.Run("a hub carries no policies", func(t *testing.T) {
		_, _, err := hub().WithPolicies(
			work.ContainerPolicies{CompletionPolicy: work.CompletionRollup}, changed)
		assertDetail(t, err, shared.ErrValidation, "containers.policies_not_supported")
	})

	t.Run("an archived collection is read-only", func(t *testing.T) {
		container := collection()
		container.ArchivedAt = &archived

		_, _, err := container.WithPolicies(
			work.ContainerPolicies{CompletionPolicy: work.CompletionRollup}, changed)
		assertDetail(t, err, shared.ErrConflict, "containers.archived")
	})
}

func TestArchivedAndUnarchived(t *testing.T) {
	t.Run("archiving stamps it", func(t *testing.T) {
		updated, moved, err := collection().Archived(changed)
		if err != nil || len(moved) != 1 {
			t.Fatalf("archiving was refused: moved=%+v err=%v", moved, err)
		}
		if updated.ArchivedAt == nil || !updated.ArchivedAt.Equal(changed) {
			t.Errorf("archived at %v, want the clock's %v", updated.ArchivedAt, changed)
		}
		// The change set says what an offline client has to be told, and it says it in the one
		// spelling every channel of this system uses.
		if moved[0].Field != "archived_at" || moved[0].To != changed.UTC().Format(time.RFC3339Nano) {
			t.Errorf("the change does not describe the stamp: %+v", moved[0])
		}
	})

	// Idempotent, which is what makes a retry after a lost response harmless.
	t.Run("archiving an archived container writes nothing", func(t *testing.T) {
		container := collection()
		container.ArchivedAt = &archived

		updated, moved, err := container.Archived(changed)
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if len(moved) != 0 || !updated.ArchivedAt.Equal(archived) || updated.UpdatedAt != created {
			t.Errorf("a repeat moved the stamp: %+v", updated)
		}
	})

	t.Run("unarchiving lifts its own stamp", func(t *testing.T) {
		container := collection()
		container.ArchivedAt = &archived

		updated, moved, err := container.Unarchived(changed)
		if err != nil || len(moved) != 1 {
			t.Fatalf("unarchiving was refused: moved=%+v err=%v", moved, err)
		}
		if updated.ArchivedAt != nil {
			t.Errorf("still archived: %+v", updated.ArchivedAt)
		}
		// The cleared field travels as the empty string, exactly as a cleared description does.
		if moved[0].To != "" || moved[0].From == "" {
			t.Errorf("the change does not describe the clearing: %+v", moved[0])
		}
	})

	t.Run("unarchiving an open container writes nothing", func(t *testing.T) {
		_, moved, err := collection().Unarchived(changed)
		if err != nil || len(moved) != 0 {
			t.Fatalf("moved=%+v err=%v, want a silent no-op", moved, err)
		}
	})

	// A hub is unarchivable while it is archived - which is the reason these two verbs read the
	// inherited stamp rather than the effective one.
	t.Run("an archived hub can still be unarchived", func(t *testing.T) {
		container := hub()
		container.ArchivedAt = &archived

		if _, moved, err := container.Unarchived(changed); err != nil || len(moved) != 1 {
			t.Fatalf("an archived hub refused to be unarchived: moved=%+v err=%v", moved, err)
		}
	})

	// Inside somebody else's archived subtree, both verbs are writes into an archived subtree.
	t.Run("a collection in an archived hub is refused both ways", func(t *testing.T) {
		container := collection()
		container.ParentArchivedAt = &archived

		_, _, err := container.Archived(changed)
		assertDetail(t, err, shared.ErrConflict, "containers.archived")
		if got := shared.AsError(err).Params["archived_id"]; got != hubID.String() {
			t.Errorf("the refusal names %q, want the hub to unarchive", got)
		}

		_, _, err = container.Unarchived(changed)
		assertDetail(t, err, shared.ErrConflict, "containers.archived")
	})

	t.Run("a trashed container has no archive to change", func(t *testing.T) {
		container := collection()
		container.DeletedAt = &archived

		_, _, err := container.Archived(changed)
		assertDetail(t, err, shared.ErrConflict, "containers.trashed")
	})
}

func TestMovedInto(t *testing.T) {
	destination := hub()
	destination.ID = otherHub

	t.Run("into another hub", func(t *testing.T) {
		updated, moved, err := collection().MovedInto(destination, "q", changed)
		if err != nil || len(moved) != 2 {
			t.Fatalf("the move was refused: moved=%+v err=%v", moved, err)
		}
		if updated.ParentID != otherHub || updated.OrderKey != "q" {
			t.Errorf("unexpected placement: parent %q rank %q", updated.ParentID, updated.OrderKey)
		}
		// Both fields are announced, because an offline client merges them separately: the hub is
		// last writer wins, the rank is a fractional index that merges by itself.
		if moved[0].Field != "parent_id" || moved[1].Field != "order_key" {
			t.Errorf("the change set does not name both fields: %+v", moved)
		}
	})

	// The destination has just been checked as accepting children, so it is not archived. What the
	// collection inherited from the hub it left does not travel with it.
	t.Run("the inherited stamp does not travel", func(t *testing.T) {
		container := collection()
		container.ParentArchivedAt = nil

		updated, _, err := container.MovedInto(destination, "q", changed)
		if err != nil {
			t.Fatalf("refused: %v", err)
		}
		if updated.ParentArchivedAt != nil {
			t.Errorf("the collection took the old hub's archive with it: %v", updated.ParentArchivedAt)
		}
	})

	t.Run("naming the hub it already sits in at the rank it already has moves nothing", func(t *testing.T) {
		_, moved, err := collection().MovedInto(hub(), "m", changed)
		if err != nil || len(moved) != 0 {
			t.Fatalf("moved=%+v err=%v, want a silent no-op", moved, err)
		}
	})

	t.Run("a hub sits in nothing and cannot be moved", func(t *testing.T) {
		_, _, err := hub().MovedInto(destination, "q", changed)
		assertDetail(t, err, shared.ErrValidation, "containers.hub_not_movable")
	})

	t.Run("into itself", func(t *testing.T) {
		container := collection()
		_, _, err := container.MovedInto(container, "q", changed)
		assertDetail(t, err, shared.ErrValidation, "containers.parent_is_self")
	})

	t.Run("into a collection", func(t *testing.T) {
		target := collection()
		target.ID = otherHub

		_, _, err := collection().MovedInto(target, "q", changed)
		assertDetail(t, err, shared.ErrValidation, "containers.parent_type_invalid")
	})

	t.Run("into an archived hub", func(t *testing.T) {
		target := destination
		target.ArchivedAt = &archived

		_, _, err := collection().MovedInto(target, "q", changed)
		assertDetail(t, err, shared.ErrConflict, "containers.parent_archived")
	})

	t.Run("an archived collection stays where it is", func(t *testing.T) {
		container := collection()
		container.ArchivedAt = &archived

		_, _, err := container.MovedInto(destination, "q", changed)
		assertDetail(t, err, shared.ErrConflict, "containers.archived")
	})
}
