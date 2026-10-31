// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// PageRepository stores the knowledge module's pages (migration 0231), through the unit of work's
// transaction (rule 3).
type PageRepository struct{}

// NewPageRepository answers the store.
func NewPageRepository() PageRepository { return PageRepository{} }

var _ repository.Pages = PageRepository{}

func pageFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// Insert writes a new page.
func (PageRepository) Insert(ctx context.Context, page knowledge.Page) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(page.ID, page.HubID, page.CreatedBy)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(page.ParentID)
	if err != nil {
		return err
	}
	if err := queries.InsertPage(ctx, sqlc.InsertPageParams{
		ID: ids[0], HubID: ids[1], ParentID: parent, Title: page.Title, Body: page.Body, Icon: page.Icon,
		OrderKey: page.OrderKey, Template: page.Template, ContentLanguage: optionalText(page.ContentLanguage),
		CreatedBy: ids[2], At: timestampOf(page.CreatedAt), DailyFor: pageDay(page.DailyFor),
	}); err != nil {
		return pageFailed("writing the page", err)
	}
	return nil
}

// Find answers a page, trashed ones included.
func (PageRepository) Find(ctx context.Context, id shared.ID) (knowledge.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Page{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return knowledge.Page{}, shared.ErrNotFound.WithDetail("pages.not_found")
	}
	row, err := queries.FindPage(ctx, key)
	if err != nil {
		return knowledge.Page{}, pageReadError(err)
	}
	return pageFrom(row)
}

// FindForUpdate answers a page and locks it.
func (PageRepository) FindForUpdate(ctx context.Context, id shared.ID) (knowledge.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Page{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return knowledge.Page{}, shared.ErrNotFound.WithDetail("pages.not_found")
	}
	row, err := queries.FindPageForUpdate(ctx, key)
	if err != nil {
		return knowledge.Page{}, pageReadError(err)
	}
	return pageFrom(sqlc.FindPageRow(row))
}

// Update writes a page at the version expected.
func (PageRepository) Update(ctx context.Context, page knowledge.Page, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(page.ID, page.UpdatedBy)
	if err != nil {
		return err
	}
	nullable, err := optionalUUIDs(page.ParentID, page.OwnerID, page.VerifiedBy)
	if err != nil {
		return err
	}
	rows, err := queries.UpdatePage(ctx, sqlc.UpdatePageParams{
		ParentID: nullable[0], Title: page.Title, Body: page.Body, Icon: page.Icon, OrderKey: page.OrderKey,
		Template: page.Template, ContentLanguage: optionalText(page.ContentLanguage), OwnerID: nullable[1],
		VerifiedUntil: optionalTimestamp(page.VerifiedUntil), VerifiedAt: optionalTimestamp(page.VerifiedAt),
		VerifiedBy: nullable[2], ArchivedAt: optionalTimestamp(page.ArchivedAt),
		DeletedAt: optionalTimestamp(page.DeletedAt), UpdatedBy: ids[1], UpdatedAt: timestampOf(page.UpdatedAt),
		ID: ids[0], ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	})
	if err != nil {
		return pageFailed("writing the page", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
	}
	return nil
}

// List answers a space's tree without bodies.
func (PageRepository) List(
	ctx context.Context, hubID shared.ID, includeArchived, templatesOnly bool, limit int,
) ([]knowledge.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListHubPages(ctx, sqlc.ListHubPagesParams{
		HubID: hub, IncludeArchived: includeArchived, TemplatesOnly: templatesOnly,
		Batch: int32(limit), //nolint:gosec // bounded by knowledge.MaxPagesPerHub
	})
	if err != nil {
		return nil, pageFailed("listing the pages", err)
	}
	return mapRows(rows, func(row sqlc.ListHubPagesRow) (knowledge.Page, error) {
		return pageFrom(bodiless(row))
	})
}

// Trashed answers a space's trashed pages.
func (PageRepository) Trashed(ctx context.Context, hubID shared.ID, limit int) ([]knowledge.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListTrashedHubPages(ctx, sqlc.ListTrashedHubPagesParams{
		HubID: hub, Batch: int32(limit), //nolint:gosec // bounded by knowledge.MaxPagesPerHub
	})
	if err != nil {
		return nil, pageFailed("listing the trashed pages", err)
	}
	return mapRows(rows, func(row sqlc.ListTrashedHubPagesRow) (knowledge.Page, error) {
		return pageFrom(bodiless(sqlc.ListHubPagesRow(row)))
	})
}

// Count answers how many pages a space holds.
func (PageRepository) Count(ctx context.Context, hubID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountHubPages(ctx, hub)
	if err != nil {
		return 0, pageFailed("counting the pages", err)
	}
	return int(count), nil
}

// Ancestors answers the pages above one, nearest first.
func (PageRepository) Ancestors(ctx context.Context, id shared.ID) ([]repository.Ancestor, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageAncestors(ctx, key)
	if err != nil {
		return nil, pageFailed("reading the ancestors", err)
	}
	return mapRows(rows, func(row sqlc.PageAncestorsRow) (repository.Ancestor, error) {
		ancestor, err := idFrom(row.ID)
		return repository.Ancestor{ID: ancestor, Title: row.Title}, err
	})
}

// Subtree answers a page and the pages under it.
func (PageRepository) Subtree(ctx context.Context, id shared.ID, includeTrashed bool, limit int) ([]knowledge.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageSubtree(ctx, sqlc.PageSubtreeParams{
		ID: key, IncludeTrashed: includeTrashed, Batch: int32(limit), //nolint:gosec // bounded by knowledge.MaxPagesPerHub
	})
	if err != nil {
		return nil, pageFailed("reading the subtree", err)
	}
	return mapRows(rows, func(row sqlc.PageSubtreeRow) (knowledge.Page, error) {
		return pageFrom(sqlc.FindPageRow(row))
	})
}

// LastOrderKey answers the last order key under a parent.
func (PageRepository) LastOrderKey(ctx context.Context, hubID, parentID shared.ID) (string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return "", err
	}
	parent, err := optionalUUID(parentID)
	if err != nil {
		return "", err
	}
	key, err := queries.LastSiblingOrderKey(ctx, sqlc.LastSiblingOrderKeyParams{HubID: hub, ParentID: parent})
	if err != nil {
		return "", pageFailed("reading the last place", err)
	}
	return key, nil
}

// Trash marks a page and the pages under it trashed.
func (PageRepository) Trash(ctx context.Context, id, by shared.ID, at time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	ids, err := pageUUIDs(id, by)
	if err != nil {
		return 0, err
	}
	rows, err := queries.TrashPageSubtree(ctx, sqlc.TrashPageSubtreeParams{ID: ids[0], By: ids[1], At: timestampOf(at)})
	if err != nil {
		return 0, pageFailed("trashing the pages", err)
	}
	return int(rows), nil
}

// Restore brings back the pages trashed with a page.
func (PageRepository) Restore(ctx context.Context, id, by shared.ID, trashedAt, at time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	ids, err := pageUUIDs(id, by)
	if err != nil {
		return 0, err
	}
	rows, err := queries.RestorePageSubtree(ctx, sqlc.RestorePageSubtreeParams{
		ID: ids[0], By: ids[1], TrashedAt: timestampOf(trashedAt), At: timestampOf(at),
	})
	if err != nil {
		return 0, pageFailed("restoring the pages", err)
	}
	return int(rows), nil
}

// Search answers the pages a query finds in the spaces named.
func (PageRepository) Search(ctx context.Context, query repository.PageQuery) ([]repository.Hit, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hubs, err := pageUUIDs(query.HubIDs...)
	if err != nil {
		return nil, err
	}
	author, err := optionalUUID(query.AuthorID)
	if err != nil {
		return nil, err
	}
	var label *string
	if query.Label != "" {
		label = &query.Label
	}
	rows, err := queries.SearchPages(ctx, sqlc.SearchPagesParams{
		Text: query.Text, HubIds: hubs, Label: label, AuthorID: author,
		UpdatedFrom: optionalTimestamp(query.UpdatedFrom), UpdatedTo: optionalTimestamp(query.UpdatedTo),
		Batch: int32(query.Limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("searching the pages", err)
	}
	return mapRows(rows, func(row sqlc.SearchPagesRow) (repository.Hit, error) {
		ids, err := pageIDs(row.ID, row.HubID)
		if err != nil {
			return repository.Hit{}, err
		}
		parent, err := optionalID(row.ParentID)
		return repository.Hit{
			ID: ids[0], HubID: ids[1], ParentID: parent, Title: row.Title, Icon: row.Icon,
			Excerpt: row.Excerpt, Rank: row.Rank, UpdatedAt: timeFrom(row.UpdatedAt),
		}, err
	})
}

// Labels answers the labels of each page among the identifiers, alphabetical.
func (PageRepository) Labels(ctx context.Context, ids []shared.ID) (map[shared.ID][]string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := pageUUIDs(ids...)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageLabels(ctx, keys)
	if err != nil {
		return nil, pageFailed("reading the page labels", err)
	}
	out := make(map[shared.ID][]string, len(ids))
	for _, row := range rows {
		id, err := idFrom(row.PageID)
		if err != nil {
			return nil, err
		}
		out[id] = append(out[id], row.Label)
	}
	return out, nil
}

// SetLabels replaces a page's labels.
func (PageRepository) SetLabels(ctx context.Context, pageID shared.ID, labels []string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID)
	if err != nil {
		return err
	}
	if _, err := queries.ClearPageLabels(ctx, ids[0]); err != nil {
		return pageFailed("clearing the page labels", err)
	}
	if len(labels) == 0 {
		return nil
	}
	if _, err := queries.AddPageLabels(ctx, sqlc.AddPageLabelsParams{PageID: ids[0], Labels: labels}); err != nil {
		return pageFailed("writing the page labels", err)
	}
	return nil
}

// SpaceLabels answers the labels of a space's live pages, by page.
func (PageRepository) SpaceLabels(ctx context.Context, hubID shared.ID, limit int) ([]repository.PageLabel, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hubs, err := pageUUIDs(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.SpaceLabels(ctx, sqlc.SpaceLabelsParams{
		HubID: hubs[0], Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("reading the space's labels", err)
	}
	return mapRows(rows, func(row sqlc.SpaceLabelsRow) (repository.PageLabel, error) {
		id, err := idFrom(row.PageID)
		return repository.PageLabel{PageID: id, Label: row.Label}, err
	})
}

// Summaries answers the pages among the identifiers that are not trashed.
func (PageRepository) Summaries(ctx context.Context, ids []shared.ID) ([]repository.Summary, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := pageUUIDs(ids...)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageSummaries(ctx, keys)
	if err != nil {
		return nil, pageFailed("reading the page summaries", err)
	}
	summaries, err := mapRows(rows, func(row sqlc.PageSummariesRow) (repository.Summary, error) {
		return summaryFrom(sqlc.PagesSharedWithRow(row))
	})
	if err != nil {
		return nil, err
	}
	return withReaders(ctx, summaries)
}

func summaryFrom(row sqlc.PagesSharedWithRow) (repository.Summary, error) {
	ids, err := pageIDs(row.ID, row.HubID)
	return repository.Summary{
		ID: ids[0], HubID: ids[1], Title: row.Title, Icon: row.Icon,
		Archived: row.ArchivedAt.Valid, VerifiedUntil: optionalTime(row.VerifiedUntil),
		Readers: knowledge.Readers{Restricted: row.Restricted},
	}, err
}

// withReaders fills the readers of the summaries whose page or one above it is restricted - all
// of them, since a page under a restricted one carries no flag of its own.
func withReaders(ctx context.Context, summaries []repository.Summary) ([]repository.Summary, error) {
	if len(summaries) == 0 {
		return summaries, nil
	}
	ids := make([]shared.ID, 0, len(summaries))
	for _, summary := range summaries {
		ids = append(ids, summary.ID)
	}
	readers, err := PageRepository{}.Readers(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range summaries {
		summaries[i].Readers = readers[summaries[i].ID]
	}
	return summaries, nil
}

// Backlinks answers the texts pointing at an object.
func (PageRepository) Backlinks(ctx context.Context, targetKind string, targetID shared.ID, limit int) ([]repository.Backlink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	target, err := uuidOf(targetID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageBacklinks(ctx, sqlc.PageBacklinksParams{
		TargetKind: targetKind, TargetID: target, Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("reading the backlinks", err)
	}
	return mapRows(rows, func(row sqlc.PageBacklinksRow) (repository.Backlink, error) {
		ids, err := pageIDs(row.SourceID, row.HubID, row.EntryID)
		if err != nil {
			return repository.Backlink{}, err
		}
		backlink := repository.Backlink{SourceKind: row.SourceKind, SourceID: ids[0], Strength: row.Strength}
		if row.SourceKind == "PAGE" {
			backlink.HubID = ids[1]
		} else {
			backlink.EntryID = ids[2]
		}
		return backlink, nil
	})
}

// LastVersion answers a page's newest version.
func (PageRepository) LastVersion(ctx context.Context, pageID shared.ID) (knowledge.Version, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Version{}, false, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return knowledge.Version{}, false, err
	}
	row, err := queries.LastPageVersion(ctx, key)
	if IsNoRows(err) {
		return knowledge.Version{}, false, nil
	}
	if err != nil {
		return knowledge.Version{}, false, pageFailed("reading the last version", err)
	}
	version, err := versionFrom(pageID, sqlc.FindPageVersionRow(row))
	return version, err == nil, err
}

// InsertVersion writes a new version.
func (PageRepository) InsertVersion(ctx context.Context, version knowledge.Version) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	page, err := uuidOf(version.PageID)
	if err != nil {
		return err
	}
	author, err := optionalUUID(version.AuthorID)
	if err != nil {
		return err
	}
	if err := queries.InsertPageVersion(ctx, sqlc.InsertPageVersionParams{
		PageID: page, Seq: int32(version.Seq), Title: version.Title, Body: version.Body, //nolint:gosec // a version's place
		AuthorID: author, Name: version.Name, Restored: int32(version.Restored), At: timestampOf(version.CreatedAt), //nolint:gosec // a version's place
	}); err != nil {
		return pageFailed("writing the version", err)
	}
	return nil
}

// FoldVersion rewrites a version's state in place.
func (PageRepository) FoldVersion(ctx context.Context, version knowledge.Version) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	page, err := uuidOf(version.PageID)
	if err != nil {
		return err
	}
	if err := queries.FoldPageVersion(ctx, sqlc.FoldPageVersionParams{
		PageID: page, Seq: int32(version.Seq), Title: version.Title, Body: version.Body, //nolint:gosec // a version's place
		At: timestampOf(version.CreatedAt),
	}); err != nil {
		return pageFailed("folding the version", err)
	}
	return nil
}

// NameVersion names a version, or takes its name away.
func (PageRepository) NameVersion(ctx context.Context, pageID shared.ID, seq int, name string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	page, err := uuidOf(pageID)
	if err != nil {
		return err
	}
	rows, err := queries.NamePageVersion(ctx, sqlc.NamePageVersionParams{
		PageID: page, Seq: int32(seq), Name: name, //nolint:gosec // a version's place
	})
	if err != nil {
		return pageFailed("naming the version", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("pages.version_not_found")
	}
	return nil
}

// Versions answers a page's history newest first, before a place (0 for the newest).
func (PageRepository) Versions(ctx context.Context, pageID shared.ID, before, limit int) ([]repository.VersionRow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	page, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPageVersions(ctx, sqlc.ListPageVersionsParams{
		PageID: page, Before: int32(before), Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("listing the versions", err)
	}
	return mapRows(rows, func(row sqlc.ListPageVersionsRow) (repository.VersionRow, error) {
		author, err := optionalID(row.AuthorID)
		return repository.VersionRow{
			Seq: int(row.Seq), Title: row.Title, AuthorID: author, Name: row.Name, Restored: int(row.Restored),
			Size: int(row.Size), CreatedAt: timeFrom(row.CreatedAt),
		}, err
	})
}

// Version answers one version with its body.
func (PageRepository) Version(ctx context.Context, pageID shared.ID, seq int) (knowledge.Version, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Version{}, err
	}
	page, err := uuidOf(pageID)
	if err != nil {
		return knowledge.Version{}, err
	}
	row, err := queries.FindPageVersion(ctx, sqlc.FindPageVersionParams{PageID: page, Seq: int32(seq)}) //nolint:gosec // a version's place
	if IsNoRows(err) {
		return knowledge.Version{}, shared.ErrNotFound.WithDetail("pages.version_not_found")
	}
	if err != nil {
		return knowledge.Version{}, pageFailed("reading the version", err)
	}
	return versionFrom(pageID, row)
}

// InsertComment writes a comment.
func (PageRepository) InsertComment(ctx context.Context, comment knowledge.Comment) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(comment.ID, comment.PageID, comment.AuthorID)
	if err != nil {
		return err
	}
	parent, err := optionalUUID(comment.ParentID)
	if err != nil {
		return err
	}
	if err := queries.InsertPageComment(ctx, sqlc.InsertPageCommentParams{
		ID: ids[0], PageID: ids[1], ParentID: parent, AuthorID: ids[2], Body: comment.Body,
		Anchor: comment.Anchor, Quote: comment.Quote, At: timestampOf(comment.CreatedAt),
	}); err != nil {
		return pageFailed("writing the comment", err)
	}
	return nil
}

// FindComment answers a comment.
func (PageRepository) FindComment(ctx context.Context, id shared.ID) (knowledge.Comment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return knowledge.Comment{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return knowledge.Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
	}
	row, err := queries.FindPageComment(ctx, key)
	if IsNoRows(err) {
		return knowledge.Comment{}, shared.ErrNotFound.WithDetail("pages.comment_not_found")
	}
	if err != nil {
		return knowledge.Comment{}, pageFailed("reading the comment", err)
	}
	return pageCommentFrom(row)
}

// Comments answers a page's comments, oldest first.
func (PageRepository) Comments(ctx context.Context, pageID shared.ID, limit int) ([]knowledge.Comment, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	page, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListPageComments(ctx, sqlc.ListPageCommentsParams{
		PageID: page, Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("listing the comments", err)
	}
	return mapRows(rows, func(row sqlc.ListPageCommentsRow) (knowledge.Comment, error) {
		return pageCommentFrom(sqlc.FindPageCommentRow(row))
	})
}

// UpdateComment writes a comment at the version expected.
func (PageRepository) UpdateComment(ctx context.Context, comment knowledge.Comment, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(comment.ID)
	if err != nil {
		return err
	}
	resolvedBy, err := optionalUUID(comment.ResolvedBy)
	if err != nil {
		return err
	}
	rows, err := queries.UpdatePageComment(ctx, sqlc.UpdatePageCommentParams{
		ID: id, Body: comment.Body, ResolvedAt: optionalTimestamp(comment.ResolvedAt), ResolvedBy: resolvedBy,
		EditedAt: optionalTimestamp(comment.EditedAt), DeletedAt: optionalTimestamp(comment.DeletedAt),
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	})
	if err != nil {
		return pageFailed("writing the comment", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("pages.comment_version_conflict")
	}
	return nil
}

func pageReadError(err error) error {
	if IsNoRows(err) {
		return shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return pageFailed("reading the page", err)
}

// bodiless is a tree row as a page row with an empty body: a tree is read without its bodies.
func bodiless(row sqlc.ListHubPagesRow) sqlc.FindPageRow {
	return sqlc.FindPageRow{
		ID: row.ID, HubID: row.HubID, ParentID: row.ParentID, Title: row.Title, Icon: row.Icon,
		OrderKey: row.OrderKey, Template: row.Template, ContentLanguage: row.ContentLanguage,
		OwnerID: row.OwnerID, VerifiedUntil: row.VerifiedUntil, VerifiedAt: row.VerifiedAt,
		VerifiedBy: row.VerifiedBy, ArchivedAt: row.ArchivedAt, DeletedAt: row.DeletedAt,
		CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, UpdatedBy: row.UpdatedBy,
		UpdatedAt: row.UpdatedAt, Version: row.Version, Restricted: row.Restricted, DailyFor: row.DailyFor,
	}
}

func pageFrom(row sqlc.FindPageRow) (knowledge.Page, error) {
	ids, err := pageIDs(row.ID, row.HubID, row.CreatedBy, row.UpdatedBy)
	if err != nil {
		return knowledge.Page{}, err
	}
	nullable := make([]shared.ID, 3)
	for i, value := range []pgtype.UUID{row.ParentID, row.OwnerID, row.VerifiedBy} {
		if nullable[i], err = optionalID(value); err != nil {
			return knowledge.Page{}, err
		}
	}
	page := knowledge.Page{
		ID: ids[0], HubID: ids[1], ParentID: nullable[0], Title: row.Title, Body: row.Body, Icon: row.Icon,
		OrderKey: row.OrderKey, Template: row.Template, OwnerID: nullable[1],
		VerifiedUntil: optionalTime(row.VerifiedUntil), VerifiedAt: optionalTime(row.VerifiedAt),
		VerifiedBy: nullable[2], ArchivedAt: optionalTime(row.ArchivedAt), DeletedAt: optionalTime(row.DeletedAt),
		CreatedBy: ids[2], CreatedAt: timeFrom(row.CreatedAt), UpdatedBy: ids[3], UpdatedAt: timeFrom(row.UpdatedAt),
		Version: int(row.Version), Restricted: row.Restricted,
	}
	if row.DailyFor.Valid {
		day := row.DailyFor.Time
		page.DailyFor = &day
	}
	if row.ContentLanguage != nil {
		page.ContentLanguage = *row.ContentLanguage
	}
	return page, nil
}

func versionFrom(pageID shared.ID, row sqlc.FindPageVersionRow) (knowledge.Version, error) {
	author, err := optionalID(row.AuthorID)
	return knowledge.Version{
		PageID: pageID, Seq: int(row.Seq), Title: row.Title, Body: row.Body, AuthorID: author,
		Name: row.Name, Restored: int(row.Restored), CreatedAt: timeFrom(row.CreatedAt),
	}, err
}

func pageCommentFrom(row sqlc.FindPageCommentRow) (knowledge.Comment, error) {
	ids, err := pageIDs(row.ID, row.PageID)
	if err != nil {
		return knowledge.Comment{}, err
	}
	nullable := make([]shared.ID, 3)
	for i, value := range []pgtype.UUID{row.ParentID, row.AuthorID, row.ResolvedBy} {
		if nullable[i], err = optionalID(value); err != nil {
			return knowledge.Comment{}, err
		}
	}
	return knowledge.Comment{
		ID: ids[0], PageID: ids[1], ParentID: nullable[0], AuthorID: nullable[1], Body: row.Body,
		Anchor: row.Anchor, Quote: row.Quote, ResolvedAt: optionalTime(row.ResolvedAt), ResolvedBy: nullable[2],
		CreatedAt: timeFrom(row.CreatedAt), EditedAt: optionalTime(row.EditedAt),
		DeletedAt: optionalTime(row.DeletedAt), Version: int(row.Version),
	}, nil
}

func pageUUIDs(ids ...shared.ID) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		key, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		out[i] = key
	}
	return out, nil
}

func optionalUUIDs(ids ...shared.ID) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, len(ids))
	for i, id := range ids {
		key, err := optionalUUID(id)
		if err != nil {
			return nil, err
		}
		out[i] = key
	}
	return out, nil
}

func pageIDs(values ...pgtype.UUID) ([]shared.ID, error) {
	out := make([]shared.ID, len(values))
	for i, value := range values {
		id, err := idFrom(value)
		if err != nil {
			return nil, err
		}
		out[i] = id
	}
	return out, nil
}

func pageDay(day *time.Time) pgtype.Date {
	if day == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *day, Valid: true}
}

// Readers answers who of each page's hub readers may read it.
func (PageRepository) Readers(ctx context.Context, ids []shared.ID) (map[shared.ID]knowledge.Readers, error) {
	out := map[shared.ID]knowledge.Readers{}
	if len(ids) == 0 {
		return out, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	keys, err := pageUUIDs(ids...)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageChainRestrictions(ctx, keys)
	if err != nil {
		return nil, pageFailed("reading the page restrictions", err)
	}
	// Per page, one set per restricted page above it, in the order the rows come.
	sets := map[shared.ID]map[shared.ID][]shared.ID{}
	for _, row := range rows {
		ids, err := pageIDs(row.PageID, row.AboveID)
		if err != nil {
			return nil, err
		}
		if sets[ids[0]] == nil {
			sets[ids[0]] = map[shared.ID][]shared.ID{}
		}
		set := sets[ids[0]][ids[1]]
		if set == nil {
			set = []shared.ID{}
		}
		if account, err := optionalID(row.AccountID); err != nil {
			return nil, err
		} else if !account.IsZero() {
			set = append(set, account)
		}
		sets[ids[0]][ids[1]] = set
	}
	for page, above := range sets {
		combined := make([][]shared.ID, 0, len(above))
		for _, set := range above {
			combined = append(combined, set)
		}
		out[page] = knowledge.CombineRestrictions(combined)
	}
	return out, nil
}

// RestrictedTo answers the people a page's own restriction names.
func (PageRepository) RestrictedTo(ctx context.Context, pageID shared.ID) ([]shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageRestrictedTo(ctx, key)
	if err != nil {
		return nil, pageFailed("reading the page restriction", err)
	}
	return pageIDs(rows...)
}

// Restrict writes a page's own restriction at the version expected.
func (PageRepository) Restrict(ctx context.Context, page knowledge.Page, accounts []shared.ID, expected int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(page.ID, page.UpdatedBy)
	if err != nil {
		return err
	}
	rows, err := queries.SetPageRestricted(ctx, sqlc.SetPageRestrictedParams{
		Restricted: accounts != nil, UpdatedBy: ids[1], UpdatedAt: timestampOf(page.UpdatedAt), ID: ids[0],
		ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	})
	if err != nil {
		return pageFailed("writing the page restriction", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
	}
	if err := queries.DropPageRestriction(ctx, ids[0]); err != nil {
		return pageFailed("writing the page restriction", err)
	}
	if len(accounts) == 0 {
		return nil
	}
	keys, err := pageUUIDs(accounts...)
	if err != nil {
		return err
	}
	if err := queries.InsertPageRestriction(ctx, sqlc.InsertPageRestrictionParams{
		PageID: ids[0], AccountIds: keys, GrantedBy: ids[1], At: timestampOf(page.UpdatedAt),
	}); isForeignKeyViolation(err) {
		return shared.ErrValidation.WithDetail("pages.restriction_account_unknown").
			WithFields(shared.FieldError{Path: "/account_ids", Code: "pages.restriction_account_unknown"})
	} else if err != nil {
		return pageFailed("writing the page restriction", err)
	}
	return nil
}

// InsertShare writes a share.
func (PageRepository) InsertShare(ctx context.Context, share knowledge.Share) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(share.PageID, share.AccountID, share.GrantedBy)
	if err != nil {
		return false, err
	}
	rows, err := queries.InsertPageShare(ctx, sqlc.InsertPageShareParams{
		PageID: ids[0], AccountID: ids[1], GrantedBy: ids[2], At: timestampOf(share.GrantedAt),
	})
	if isForeignKeyViolation(err) {
		return false, shared.ErrValidation.WithDetail("pages.share_account_unknown").
			WithFields(shared.FieldError{Path: "/account_id", Code: "pages.share_account_unknown"})
	}
	if err != nil {
		return false, pageFailed("writing the share", err)
	}
	return rows > 0, nil
}

// DeleteShare takes a share away.
func (PageRepository) DeleteShare(ctx context.Context, pageID, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(pageID, accountID)
	if err != nil {
		return false, err
	}
	rows, err := queries.DeletePageShare(ctx, sqlc.DeletePageShareParams{PageID: ids[0], AccountID: ids[1]})
	if err != nil {
		return false, pageFailed("removing the share", err)
	}
	return rows > 0, nil
}

// Shares answers the people a page is shared with.
func (PageRepository) Shares(ctx context.Context, pageID shared.ID, limit int) ([]knowledge.Share, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(pageID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PageShares(ctx, sqlc.PageSharesParams{
		PageID: key, Batch: int32(limit), //nolint:gosec // bounded by knowledge.MaxSharesPerPage
	})
	if err != nil {
		return nil, pageFailed("reading the shares", err)
	}
	return mapRows(rows, func(row sqlc.PageSharesRow) (knowledge.Share, error) {
		ids, err := pageIDs(row.PageID, row.AccountID)
		if err != nil {
			return knowledge.Share{}, err
		}
		by, err := optionalID(row.GrantedBy)
		return knowledge.Share{PageID: ids[0], AccountID: ids[1], GrantedBy: by, GrantedAt: timeFrom(row.GrantedAt)}, err
	})
}

// SharedWith answers whether a page is shared with a person.
func (PageRepository) SharedWith(ctx context.Context, pageID, accountID shared.ID) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	ids, err := pageUUIDs(pageID, accountID)
	if err != nil {
		return false, nil
	}
	held, err := queries.IsPageSharedWith(ctx, sqlc.IsPageSharedWithParams{PageID: ids[0], AccountID: ids[1]})
	if err != nil {
		return false, pageFailed("reading the share", err)
	}
	return held, nil
}

// PagesSharedWith answers the living pages shared with a person.
func (PageRepository) PagesSharedWith(ctx context.Context, accountID shared.ID, limit int) ([]repository.Summary, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.PagesSharedWith(ctx, sqlc.PagesSharedWithParams{
		AccountID: key, Batch: int32(limit), //nolint:gosec // bounded by the use case
	})
	if err != nil {
		return nil, pageFailed("reading the shared pages", err)
	}
	summaries, err := mapRows(rows, summaryFrom)
	if err != nil {
		return nil, err
	}
	return withReaders(ctx, summaries)
}

// PersonalSpace answers a person's own notes' hub.
func (PageRepository) PersonalSpace(ctx context.Context, accountID shared.ID) (shared.ID, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", false, err
	}
	key, err := uuidOf(accountID)
	if err != nil {
		return "", false, err
	}
	hub, err := queries.FindPersonalSpace(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, pageFailed("reading the personal space", err)
	}
	id, err := optionalID(hub)
	return id, err == nil, err
}

// ClaimPersonalSpace names a hub a person's own notes.
func (r PageRepository) ClaimPersonalSpace(ctx context.Context, accountID, hubID shared.ID, at time.Time) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	ids, err := pageUUIDs(accountID, hubID)
	if err != nil {
		return "", err
	}
	hub, err := queries.ClaimPersonalSpace(ctx, sqlc.ClaimPersonalSpaceParams{AccountID: ids[0], HubID: ids[1], At: timestampOf(at)})
	if errors.Is(err, pgx.ErrNoRows) {
		held, found, err := r.PersonalSpace(ctx, accountID)
		if err == nil && !found {
			err = shared.ErrVersionConflict.WithDetail("pages.personal_space_conflict")
		}
		return held, err
	}
	if err != nil {
		return "", pageFailed("writing the personal space", err)
	}
	return optionalID(hub)
}

// DailyNote answers a space's daily note for a day.
func (PageRepository) DailyNote(ctx context.Context, hubID shared.ID, day time.Time) (shared.ID, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", false, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return "", false, err
	}
	id, err := queries.FindDailyNote(ctx, sqlc.FindDailyNoteParams{HubID: key, Day: pgtype.Date{Time: day, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, pageFailed("reading the daily note", err)
	}
	page, err := optionalID(id)
	return page, err == nil, err
}

// AttachMedia makes a media object one of a page's files; holding it twice is once.
func (PageRepository) AttachMedia(ctx context.Context, pageID, mediaID shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := pageUUIDs(pageID, mediaID)
	if err != nil {
		return err
	}
	if err := queries.AttachPageMedia(ctx, sqlc.AttachPageMediaParams{PageID: ids[0], MediaID: ids[1]}); err != nil {
		return pageFailed("holding a file", err)
	}
	return nil
}

// CountMedia answers how many media objects a page holds.
func (PageRepository) CountMedia(ctx context.Context, pageID shared.ID) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	id, err := uuidOf(pageID)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountPageMedia(ctx, id)
	if err != nil {
		return 0, pageFailed("counting the files", err)
	}
	return int(count), nil
}
