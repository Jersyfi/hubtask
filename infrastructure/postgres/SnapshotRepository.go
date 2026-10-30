// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// SnapshotRepository reads the current state for an initial synchronisation: every kind
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
			row.Kind, row.SystemCode, row.SystemParams,
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

// EntryTypes pages the workspace's type rows.
func (SnapshotRepository) EntryTypes(ctx context.Context, after shared.ID, batch int) ([]work.EntryType, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotEntryTypes(ctx, sqlc.SnapshotEntryTypesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("entry types", err)
	}
	out := make([]work.EntryType, 0, len(rows))
	for _, row := range rows {
		entryType, err := entryTypeFrom(sqlc.EntryType(row))
		if err != nil {
			return nil, err
		}
		out = append(out, entryType)
	}
	return out, nil
}

// Workflows pages the workspace's workflow rows.
func (SnapshotRepository) Workflows(ctx context.Context, after shared.ID, batch int) ([]work.Workflow, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotWorkflows(ctx, sqlc.SnapshotWorkflowsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("workflows", err)
	}
	return mapRows(rows, workflowFrom)
}

// LinkTypes pages the workspace's own link type rows.
func (SnapshotRepository) LinkTypes(ctx context.Context, after shared.ID, batch int) ([]work.LinkType, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotLinkTypes(ctx, sqlc.SnapshotLinkTypesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("link types", err)
	}
	return mapRows(rows, linkTypeFrom)
}

// Pages pages the workspace's living pages with their bodies.
func (SnapshotRepository) Pages(ctx context.Context, after shared.ID, batch int) ([]knowledge.Page, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotPages(ctx, sqlc.SnapshotPagesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("pages", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotPagesRow) (knowledge.Page, error) {
		return pageFrom(sqlc.FindPageRow(row))
	})
}

// CIs pages the workspace's configuration items that are not merged away.
func (SnapshotRepository) CIs(ctx context.Context, after shared.ID, batch int) ([]cmdb.CI, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCmdbCis(ctx, sqlc.SnapshotCmdbCisParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("configuration items", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotCmdbCisRow) (cmdb.CI, error) {
		return cmdbCIFrom(sqlc.FindCmdbCiRow(row))
	})
}

// CmdbCatalogue reads the workspace's own classes and relation types.
func (SnapshotRepository) CmdbCatalogue(ctx context.Context) ([]cmdb.Class, []cmdb.RelationType, error) {
	repo := NewCmdbRepository()
	classes, err := repo.Classes(ctx)
	if err != nil {
		return nil, nil, err
	}
	types, err := repo.RelationTypes(ctx)
	if err != nil {
		return nil, nil, err
	}
	return classes, types, nil
}

// CIRelations pages the relations between configuration items.
func (SnapshotRepository) CIRelations(ctx context.Context, after shared.ID, batch int) ([]cmdb.Relation, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCmdbRelations(ctx, sqlc.SnapshotCmdbRelationsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("configuration item relations", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotCmdbRelationsRow) (cmdb.Relation, error) {
		return cmdbRelationFrom(sqlc.FindCmdbRelationRow(row))
	})
}

// CIAffected pages the CIs entries name as affected, each with its CI's hub (migration 0267).
func (SnapshotRepository) CIAffected(ctx context.Context, after shared.ID, batch int) ([]repository.AffectedRow, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCmdbAffected(ctx, sqlc.SnapshotCmdbAffectedParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("affected configuration items", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotCmdbAffectedRow) (repository.AffectedRow, error) {
		a, err := affectedFrom(row.ID, row.ItemID, row.CiID, row.AddedBy, row.AddedAt)
		if err != nil {
			return repository.AffectedRow{}, err
		}
		hub, err := idFrom(row.HubID)
		return repository.AffectedRow{Affected: a, HubID: hub}, err
	})
}

// CIGroups pages the workspace's CI groups.
func (SnapshotRepository) CIGroups(ctx context.Context, after shared.ID, batch int) ([]cmdb.Group, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCmdbGroups(ctx, sqlc.SnapshotCmdbGroupsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("configuration item groups", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotCmdbGroupsRow) (cmdb.Group, error) {
		return groupFrom(sqlc.FindCmdbGroupRow(row))
	})
}

// CIAssets pages the CIs' asset records.
func (SnapshotRepository) CIAssets(ctx context.Context, after shared.ID, batch int) ([]cmdb.Asset, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCmdbAssets(ctx, sqlc.SnapshotCmdbAssetsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("asset records", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotCmdbAssetsRow) (cmdb.Asset, error) {
		return assetFrom(sqlc.FindCmdbAssetRow(row))
	})
}

// Whiteboards pages the workspace's boards outside the trash with their scenes.
func (SnapshotRepository) Whiteboards(ctx context.Context, after shared.ID, batch int) ([]whiteboard.Board, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotWhiteboards(ctx, sqlc.SnapshotWhiteboardsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("whiteboards", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotWhiteboardsRow) (whiteboard.Board, error) {
		return boardFrom(sqlc.FindWhiteboardRow(row))
	})
}

// EntryLinks pages every link between two live entries, with the collection of each end.
func (SnapshotRepository) EntryLinks(ctx context.Context, after shared.ID, batch int) ([]repository.LinkBetween, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotEntryLinks(ctx, sqlc.SnapshotEntryLinksParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("entry links", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotEntryLinksRow) (repository.LinkBetween, error) {
		link, err := entryLinkFrom(sqlc.FindEntryLinkRow{
			ID: row.ID, TenantID: row.TenantID, LinkType: row.LinkType, SourceID: row.SourceID,
			TargetID: row.TargetID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt,
			DependencyKind: row.DependencyKind, LagDays: row.LagDays,
		})
		if err != nil {
			return repository.LinkBetween{}, err
		}
		source, err := idFrom(row.SourceCollectionID)
		if err != nil {
			return repository.LinkBetween{}, err
		}
		target, err := idFrom(row.TargetCollectionID)
		if err != nil {
			return repository.LinkBetween{}, err
		}
		return repository.LinkBetween{Link: link, SourceCollectionID: source, TargetCollectionID: target}, nil
	})
}

// EntrySchedules pages the live entries' stored scheduling modes.
func (SnapshotRepository) EntrySchedules(ctx context.Context, after shared.ID, batch int) ([]repository.ScheduleOf, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotEntrySchedules(ctx, sqlc.SnapshotEntrySchedulesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("entry schedules", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotEntrySchedulesRow) (repository.ScheduleOf, error) {
		entry, err := idFrom(row.EntryID)
		if err != nil {
			return repository.ScheduleOf{}, err
		}
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return repository.ScheduleOf{}, err
		}
		return repository.ScheduleOf{
			EntryID: entry, CollectionID: collection, Mode: row.Mode, Milestone: row.Milestone, Version: int(row.Version),
		}, nil
	})
}

// CustomRoles pages the workspace's own roles.
func (SnapshotRepository) CustomRoles(ctx context.Context, after shared.ID, batch int) ([]identityrepo.CustomRoleRow, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotCustomRoles(ctx, sqlc.SnapshotCustomRolesParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("custom roles", err)
	}
	return mapRows(rows, customRoleFrom)
}

// OwnRoleGrants pages the grants of custom roles the account holds itself or through a group.
func (SnapshotRepository) OwnRoleGrants(
	ctx context.Context, accountID, after shared.ID, batch int,
) ([]identityrepo.CustomGrantRow, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotOwnRoleGrants(ctx, sqlc.SnapshotOwnRoleGrantsParams{
		After: key, AccountID: account, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("role grants", err)
	}
	return mapRows(rows, func(row sqlc.SnapshotOwnRoleGrantsRow) (identityrepo.CustomGrantRow, error) {
		return customGrantFrom(sqlc.ListCustomRoleGrantsRow(row))
	})
}

// InGroup reports whether the account is a member of the group.
func (SnapshotRepository) InGroup(ctx context.Context, accountID, groupID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return false, err
	}
	group, err := uuidOf(groupID)
	if err != nil {
		return false, err
	}
	member, err := queries.IsAccountGroupMember(ctx, sqlc.IsAccountGroupMemberParams{GroupID: group, AccountID: account})
	if err != nil {
		return false, snapshotFailed("group membership", err)
	}
	return member, nil
}

// mapRows turns a page of rows into the port's values, failing on the first that does not read.
func mapRows[R, T any](rows []R, from func(R) (T, error)) ([]T, error) {
	out := make([]T, 0, len(rows))
	for _, row := range rows {
		value, err := from(row)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
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

// GoalPeriods pages the workspace's goal periods.
func (SnapshotRepository) GoalPeriods(ctx context.Context, after shared.ID, batch int) ([]goal.Period, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotGoalPeriods(ctx, sqlc.SnapshotGoalPeriodsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("goal periods", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotGoalPeriodsRow) (goal.Period, error) {
		id, err := idFrom(r.ID)
		if err != nil {
			return goal.Period{}, err
		}
		return goal.Period{
			ID: id, Name: r.Name, StartOn: planDay(r.StartOn), EndOn: planDay(r.EndOn),
			State: goal.PeriodState(r.State), Version: int(r.Version),
		}, nil
	})
}

// Goals pages the workspace's goals.
func (SnapshotRepository) Goals(ctx context.Context, after shared.ID, batch int) ([]goal.Goal, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotGoals(ctx, sqlc.SnapshotGoalsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("goals", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotGoalsRow) (goal.Goal, error) { return goalOf(sqlc.ListGoalsRow(r)) })
}

// GoalCheckIns pages the goals' check-ins with their goals' hubs.
func (SnapshotRepository) GoalCheckIns(ctx context.Context, after shared.ID, batch int) ([]repository.GoalCheckIn, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotGoalCheckIns(ctx, sqlc.SnapshotGoalCheckInsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("goal check-ins", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotGoalCheckInsRow) (repository.GoalCheckIn, error) {
		found, err := ids(r.ID, r.GoalID, r.AuthorID, r.HubID)
		if err != nil {
			return repository.GoalCheckIn{}, err
		}
		values, err := decodeValues(r.Values)
		if err != nil {
			return repository.GoalCheckIn{}, err
		}
		return repository.GoalCheckIn{HubID: found[3], CheckIn: goal.CheckIn{
			ID: found[0], GoalID: found[1], AuthorID: found[2], Status: goal.Status(r.Status),
			Confidence: int(r.Confidence), Note: r.Note, Values: values, At: r.CreatedAt.Time,
		}}, nil
	})
}

// Dashboards pages the workspace's dashboards.
func (SnapshotRepository) Dashboards(ctx context.Context, after shared.ID, batch int) ([]goal.Dashboard, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotDashboards(ctx, sqlc.SnapshotDashboardsParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("dashboards", err)
	}
	return mapRows(rows, func(r sqlc.SnapshotDashboardsRow) (goal.Dashboard, error) {
		return dashboardOf(sqlc.ListDashboardsRow(r))
	})
}

// Portfolios pages the workspace's portfolios with their projects.
func (SnapshotRepository) Portfolios(ctx context.Context, after shared.ID, batch int) ([]goal.Portfolio, error) {
	queries, key, err := snapshotPage(ctx, after)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SnapshotPortfolios(ctx, sqlc.SnapshotPortfoliosParams{
		After: key, Batch: int32(batch), //nolint:gosec // bounded by the contract's page maximum
	})
	if err != nil {
		return nil, snapshotFailed("portfolios", err)
	}
	out, err := mapRows(rows, func(r sqlc.SnapshotPortfoliosRow) (goal.Portfolio, error) {
		return portfolioOf(sqlc.ListPortfoliosRow(r))
	})
	if err != nil {
		return nil, err
	}
	return out, withItems(ctx, queries, out)
}
