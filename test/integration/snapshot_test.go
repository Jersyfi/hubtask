// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The initial synchronisation's reader (N-02): every kind the change log records, paged by
// identifier across the whole workspace, live rows only - and a cross-tenant negative for every
// method (gate SG-3). The database is shared across files, so every assertion is by identity:
// the rows this test seeded are found in the walk, or they are not, whatever else is there.

func snapshotRepo() postgres.SnapshotRepository { return postgres.NewSnapshotRepository() }

// snapshotFixture is one of everything, in tenant A.
type snapshotFixture struct {
	hub, collection, task shared.ID
	bucket                work.Bucket
	label                 work.Label
	comment               work.Comment
	reminder              work.Reminder
	rule                  work.RecurrenceRule
	template              work.Template
	tag                   shared.HLC
}

func seedSnapshot(ctx context.Context, t *testing.T) snapshotFixture {
	t.Helper()
	seedContainerTenants(ctx, t)

	var f snapshotFixture
	f.hub, f.collection = hubWithCollection(ctx, t, tenantA, authorA)
	f.bucket = seedBucket(ctx, t, tenantA, f.collection, "a0")
	f.label = seedLabel(ctx, t, tenantA, f.collection)
	f.task = seedTask(ctx, t, tenantA, authorA, f.collection)

	tag, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	f.tag = tag
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, f.task, f.label.ID, tag)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	f.comment = seedComment(ctx, t, tenantA, f.task, authorA, "Looks right", created)
	f.reminder = seedReminder(ctx, t, tenantA, f.task, "ABS:2026-09-01T08:00:00Z", nil)

	due := created.Add(48 * time.Hour)
	dueDate, err := work.NewDueDate(&due, false, "Europe/Berlin")
	if err != nil {
		t.Fatalf("the due date was refused: %v", err)
	}
	item := findWorkItem(ctx, t, tenantA, f.task)
	item.Due = dueDate
	item.UpdatedAt = due
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetDueDate(ctx, item, item.Version)
	}); err != nil {
		t.Fatalf("setting the due date: %v", err)
	}
	rule, err := work.NewRecurrenceRule(work.NewRecurrenceRuleInput{
		ID: freshID(t), TenantID: tenantA, ItemID: f.task,
		Spec: work.RecurrenceSpec{
			RRULE: "FREQ=DAILY", TimeZone: "Europe/Berlin",
			Mode: string(work.RecurrenceOnSchedule), HorizonDays: 7,
		},
		Due: dueDate, Now: due,
	})
	if err != nil {
		t.Fatalf("the series was refused: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return recurrenceRepo().Insert(ctx, rule)
	}); err != nil {
		t.Fatalf("writing the series: %v", err)
	}
	f.rule = rule

	f.template = templateFor(t, tenantA, f.collection, freshName(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return templateRepo().Insert(ctx, f.template)
	}); err != nil {
		t.Fatalf("writing the template: %v", err)
	}
	return f
}

// snapshotWalk pages one kind to its end from the given tenant and reports whether the identifier was
// seen. A batch of two, so that the paging itself is exercised rather than a single read.
func snapshotWalk[T any](
	ctx context.Context, t *testing.T, tenant shared.ID,
	page func(ctx context.Context, after shared.ID, batch int) ([]T, error),
	idOf func(T) shared.ID, wanted shared.ID,
) (found bool, rows []T) {
	t.Helper()

	var after shared.ID
	for {
		var batch []T
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			batch, err = page(ctx, after, 2)
			return err
		}); err != nil {
			t.Fatalf("paging: %v", err)
		}
		for _, row := range batch {
			rows = append(rows, row)
			if idOf(row) == wanted {
				found = true
			}
			after = idOf(row)
		}
		if len(batch) < 2 {
			return found, rows
		}
	}
}

func TestTheSnapshotPagesEveryKindByIdentifier(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	repo := snapshotRepo()

	t.Run("containers", func(t *testing.T) {
		found, rows := snapshotWalk(ctx, t, tenantA, repo.Containers,
			func(c work.Container) shared.ID { return c.ID }, f.collection)
		if !found {
			t.Errorf("the collection is not in the walk")
		}
		for i := 1; i < len(rows); i++ {
			if rows[i].ID.String() <= rows[i-1].ID.String() {
				t.Fatalf("the walk is not by identifier: %s after %s", rows[i].ID, rows[i-1].ID)
			}
		}
	})
	t.Run("buckets", func(t *testing.T) {
		if found, _ := snapshotWalk(ctx, t, tenantA, repo.Buckets,
			func(b work.Bucket) shared.ID { return b.ID }, f.bucket.ID); !found {
			t.Errorf("the bucket is not in the walk")
		}
	})
	t.Run("labels", func(t *testing.T) {
		if found, _ := snapshotWalk(ctx, t, tenantA, repo.Labels,
			func(l work.Label) shared.ID { return l.ID }, f.label.ID); !found {
			t.Errorf("the label is not in the walk")
		}
	})
	t.Run("items", func(t *testing.T) {
		found, rows := snapshotWalk(ctx, t, tenantA, repo.Items,
			func(i work.WorkItem) shared.ID { return i.ID }, f.task)
		if !found {
			t.Errorf("the entry is not in the walk")
		}
		for _, row := range rows {
			if row.ID == f.task && row.Due == nil {
				t.Errorf("the entry came back without its due date: the row mapper is not the find's")
			}
		}
	})
	t.Run("set elements", func(t *testing.T) {
		var after repository.SetElementKey
		found := false
		for {
			var batch []repository.ItemSetElement
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				var err error
				batch, err = repo.SetElements(ctx, after, 2)
				return err
			}); err != nil {
				t.Fatalf("paging: %v", err)
			}
			for _, row := range batch {
				if row.ItemID == f.task && row.Set == work.SetLabels && row.Element.ElementID == f.label.ID {
					found = true
					if row.CollectionID != f.collection || row.Element.AddedAt.Compare(f.tag) != 0 {
						t.Errorf("the element came back as %+v", row)
					}
				}
				after = repository.SetElementKey{ItemID: row.ItemID, Set: row.Set, ElementID: row.Element.ElementID}
			}
			if len(batch) < 2 {
				break
			}
		}
		if !found {
			t.Errorf("the label's tag is not in the walk")
		}
	})
	t.Run("comments", func(t *testing.T) {
		found, rows := snapshotWalk(ctx, t, tenantA, repo.Comments,
			func(c repository.InCollection[work.Comment]) shared.ID { return c.Value.ID }, f.comment.ID)
		if !found {
			t.Errorf("the comment is not in the walk")
		}
		for _, row := range rows {
			if row.Value.ID == f.comment.ID && row.CollectionID != f.collection {
				t.Errorf("the comment names collection %s, want %s", row.CollectionID, f.collection)
			}
		}
	})
	t.Run("reminders", func(t *testing.T) {
		found, rows := snapshotWalk(ctx, t, tenantA, repo.Reminders,
			func(r repository.InCollection[work.Reminder]) shared.ID { return r.Value.ID }, f.reminder.ID)
		if !found {
			t.Errorf("the reminder is not in the walk")
		}
		for _, row := range rows {
			if row.Value.ID == f.reminder.ID && row.CollectionID != f.collection {
				t.Errorf("the reminder names collection %s, want %s", row.CollectionID, f.collection)
			}
		}
	})
	t.Run("recurrences", func(t *testing.T) {
		found, rows := snapshotWalk(ctx, t, tenantA, repo.Recurrences,
			func(r repository.InCollection[work.RecurrenceRule]) shared.ID { return r.Value.ID }, f.rule.ID)
		if !found {
			t.Errorf("the rule is not in the walk")
		}
		for _, row := range rows {
			if row.Value.ID == f.rule.ID && row.CollectionID != f.collection {
				t.Errorf("the rule names collection %s, want %s", row.CollectionID, f.collection)
			}
		}
	})
	t.Run("templates", func(t *testing.T) {
		if found, _ := snapshotWalk(ctx, t, tenantA, repo.Templates,
			func(tpl work.Template) shared.ID { return tpl.ID }, f.template.ID); !found {
			t.Errorf("the template is not in the walk")
		}
	})
}

// A trashed entry is a tombstone in the log, not state: it is not in the walk, and neither is
// what hangs off it.
func TestTheSnapshotLeavesTheTrashOut(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	stampColumn(ctx, t, f.task, "deleted_at")

	if found, _ := snapshotWalk(ctx, t, tenantA, snapshotRepo().Items,
		func(i work.WorkItem) shared.ID { return i.ID }, f.task); found {
		t.Errorf("a trashed entry is in the walk")
	}
	if found, _ := snapshotWalk(ctx, t, tenantA, snapshotRepo().Comments,
		func(c repository.InCollection[work.Comment]) shared.ID { return c.Value.ID }, f.comment.ID); found {
		t.Errorf("a trashed entry's comment is in the walk")
	}
}

// Gate SG-3: one negative per port method. The walk from tenant B never meets tenant A's rows.
func TestTheSnapshotIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	f := seedSnapshot(ctx, t)
	repo := snapshotRepo()

	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Containers,
		func(c work.Container) shared.ID { return c.ID }, f.collection); found {
		t.Errorf("containers: tenant B saw tenant A's collection")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Buckets,
		func(b work.Bucket) shared.ID { return b.ID }, f.bucket.ID); found {
		t.Errorf("buckets: tenant B saw tenant A's bucket")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Labels,
		func(l work.Label) shared.ID { return l.ID }, f.label.ID); found {
		t.Errorf("labels: tenant B saw tenant A's label")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Items,
		func(i work.WorkItem) shared.ID { return i.ID }, f.task); found {
		t.Errorf("items: tenant B saw tenant A's entry")
	}
	var elements []repository.ItemSetElement
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		elements, err = repo.SetElements(ctx, repository.SetElementKey{}, 1000)
		return err
	}); err != nil {
		t.Fatalf("paging set elements: %v", err)
	}
	for _, element := range elements {
		if element.ItemID == f.task {
			t.Errorf("set elements: tenant B saw tenant A's tag")
		}
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Comments,
		func(c repository.InCollection[work.Comment]) shared.ID { return c.Value.ID }, f.comment.ID); found {
		t.Errorf("comments: tenant B saw tenant A's comment")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Reminders,
		func(r repository.InCollection[work.Reminder]) shared.ID { return r.Value.ID }, f.reminder.ID); found {
		t.Errorf("reminders: tenant B saw tenant A's reminder")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Recurrences,
		func(r repository.InCollection[work.RecurrenceRule]) shared.ID { return r.Value.ID }, f.rule.ID); found {
		t.Errorf("recurrences: tenant B saw tenant A's rule")
	}
	if found, _ := snapshotWalk(ctx, t, tenantB, repo.Templates,
		func(tpl work.Template) shared.ID { return tpl.ID }, f.template.ID); found {
		t.Errorf("templates: tenant B saw tenant A's template")
	}
}
