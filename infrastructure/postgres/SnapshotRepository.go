// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SnapshotRepository reads the current state for an initial synchronisation (N-02): every kind
// the change log records, in pages by identifier, live rows only.
//
// Every statement runs inside the caller's transaction and therefore under `SET LOCAL
// app.tenant_id` (rule 3): row level security is what keeps one workspace's walk out of another's
// rows, and the explicit tenant predicate beside it is the index's, not the boundary's.
type SnapshotRepository struct{}

func NewSnapshotRepository() SnapshotRepository { return SnapshotRepository{} }

var _ repository.Snapshot = SnapshotRepository{}

// Containers pages the live hubs and collections.
func (SnapshotRepository) Containers(
	ctx context.Context, after shared.ID, batch int,
) ([]work.Container, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotContainers(ctx, sqlc.SnapshotContainersParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("containers", err)
	}
	out := make([]work.Container, 0, len(rows))
	for _, row := range rows {
		container, err := containerFrom(sqlc.FindContainerRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, container)
	}
	return out, nil
}

// Buckets pages the live columns.
func (SnapshotRepository) Buckets(
	ctx context.Context, after shared.ID, batch int,
) ([]work.Bucket, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotBuckets(ctx, sqlc.SnapshotBucketsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("buckets", err)
	}
	out := make([]work.Bucket, 0, len(rows))
	for _, row := range rows {
		bucket, err := bucketFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, bucket)
	}
	return out, nil
}

// Labels pages the live labels.
func (SnapshotRepository) Labels(
	ctx context.Context, after shared.ID, batch int,
) ([]work.Label, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotLabels(ctx, sqlc.SnapshotLabelsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("labels", err)
	}
	out := make([]work.Label, 0, len(rows))
	for _, row := range rows {
		label, err := labelFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, label)
	}
	return out, nil
}

// Items pages the live entries, archived ones included.
func (SnapshotRepository) Items(
	ctx context.Context, after shared.ID, batch int,
) ([]work.WorkItem, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotWorkItems(ctx, sqlc.SnapshotWorkItemsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("entries", err)
	}
	out := make([]work.WorkItem, 0, len(rows))
	for _, row := range rows {
		item, err := itemFrom(sqlc.FindWorkItemRow(row))
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

// SetElements pages every tag row of every live entry, in the order of the row's key.
func (SnapshotRepository) SetElements(
	ctx context.Context, after repository.SetElementKey, batch int,
) ([]repository.ItemSetElement, error) {
	queries, itemKey, err := snapshotPage(ctx, after.ItemID)
	if err != nil {
		return nil, err
	}
	elementKey, err := pageKey(after.ElementID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotSetElements(ctx, sqlc.SnapshotSetElementsParams{
		AfterItemID: itemKey, AfterSetName: string(after.Set), AfterElementID: elementKey,
		Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("set elements", err)
	}
	out := make([]repository.ItemSetElement, 0, len(rows))
	for _, row := range rows {
		itemID, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		elementID, err := idFrom(row.ElementID)
		if err != nil {
			return nil, err
		}
		added, err := tagFrom(row.AddTag)
		if err != nil {
			return nil, err
		}
		removed, err := tagFrom(row.RemoveTag)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.ItemSetElement{
			ItemID: itemID, CollectionID: collectionID, Set: work.SetName(row.SetName),
			Element: work.SetElement{ElementID: elementID, AddedAt: added, RemovedAt: removed},
		})
	}
	return out, nil
}

// Comments pages the live comments of live entries.
func (SnapshotRepository) Comments(
	ctx context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.Comment], error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotComments(ctx, sqlc.SnapshotCommentsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("comments", err)
	}
	out := make([]repository.InCollection[work.Comment], 0, len(rows))
	for _, row := range rows {
		comment, err := commentFrom(
			row.ID, row.TenantID, row.ItemID, row.AuthorID, row.ParentCommentID,
			row.Body, row.CreatedAt, row.EditedAt, row.DeletedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.InCollection[work.Comment]{Value: comment, CollectionID: collectionID})
	}
	return out, nil
}

// Reminders pages the reminders of live entries.
func (SnapshotRepository) Reminders(
	ctx context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.Reminder], error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotReminders(ctx, sqlc.SnapshotRemindersParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("reminders", err)
	}
	out := make([]repository.InCollection[work.Reminder], 0, len(rows))
	for _, row := range rows {
		reminder, err := reminderFrom(
			row.ID, row.TenantID, row.ItemID, row.OffsetSpec, row.Channels, row.Recipients,
			row.State, row.FireAt, row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.InCollection[work.Reminder]{Value: reminder, CollectionID: collectionID})
	}
	return out, nil
}

// Recurrences pages the recurrence rules of live entries.
func (SnapshotRepository) Recurrences(
	ctx context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.RecurrenceRule], error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotRecurrenceRules(ctx, sqlc.SnapshotRecurrenceRulesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("recurrence rules", err)
	}
	out := make([]repository.InCollection[work.RecurrenceRule], 0, len(rows))
	for _, row := range rows {
		rule, err := recurrenceFrom(
			row.ID, row.TenantID, row.SourceItemID, row.Rrule, row.TimeZone, row.Mode,
			row.HorizonDays, row.EndsAt, row.MaxCount, row.LastMaterializedAt,
			row.CreatedAt, row.UpdatedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		collectionID, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.InCollection[work.RecurrenceRule]{Value: rule, CollectionID: collectionID})
	}
	return out, nil
}

// Templates pages the live templates.
func (SnapshotRepository) Templates(
	ctx context.Context, after shared.ID, batch int,
) ([]work.Template, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotTemplates(ctx, sqlc.SnapshotTemplatesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("templates", err)
	}
	out := make([]work.Template, 0, len(rows))
	for _, row := range rows {
		template, err := templateFrom(
			row.ID, row.TenantID, row.ScopeType, row.ScopeID, row.Name, row.Description,
			string(row.RootType), row.Nodes, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.Version,
		)
		if err != nil {
			return nil, err
		}
		out = append(out, template)
	}
	return out, nil
}

// snapshotPage is the opening every method shares: the transaction's queries and the page key.
func snapshotPage(ctx context.Context, after shared.ID) (*sqlc.Queries, pgtype.UUID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	key, err := pageKey(after)
	if err != nil {
		return nil, pgtype.UUID{}, err
	}
	return queries, key, nil
}

// pageKey is the identifier a page resumes after. The walk starts with none, and "after nothing"
// is the nil UUID: every identifier sorts after it, and it is a value rather than a NULL the
// comparison would swallow.
func pageKey(after shared.ID) (pgtype.UUID, error) {
	if after.IsZero() {
		return pgtype.UUID{Valid: true}, nil
	}
	return uuidOf(after)
}

func snapshotFailed(kind string, err error) error {
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("reading the %s for a synchronisation: %w", kind, err))
}
