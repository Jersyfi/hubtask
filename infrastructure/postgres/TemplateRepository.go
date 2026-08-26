// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// templateNameIndex is the partial unique index a taken name violates (migration 0030). Named
// here so that the one refusal a client can act on is told apart from a database that is unwell.
const templateNameIndex = "template_name_uq"

// TemplateRepository stores the trees somebody wrote down to stamp out again (D-06).
type TemplateRepository struct {
	cursors security.CursorCodec
}

func NewTemplateRepository(cursors security.CursorCodec) TemplateRepository {
	return TemplateRepository{cursors: cursors}
}

var _ repository.Templates = TemplateRepository{}

// Find returns the template, deleted or not.
func (r TemplateRepository) Find(ctx context.Context, id shared.ID) (work.Template, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return work.Template{}, err
	}
	templateID, err := uuidOf(id)
	if err != nil {
		return work.Template{}, err
	}

	row, err := queries.FindTemplate(ctx, templateID)
	if err != nil {
		if IsNoRows(err) {
			return work.Template{}, shared.ErrNotFound
		}
		return work.Template{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the template %s: %w", id, err))
	}
	return templateFrom(
		row.ID, row.TenantID, row.ScopeType, row.ScopeID, row.Name, row.Description,
		string(row.RootType), row.Nodes, row.CreatedAt, row.UpdatedAt, row.DeletedAt, row.Version,
	)
}

// ListInScopes returns one page of what is reachable from a container's path.
func (r TemplateRepository) ListInScopes(
	ctx context.Context, scopeIDs []shared.ID, page repository.Page,
) (repository.TemplatePage, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.TemplatePage{}, err
	}
	scopes, err := scopeUUIDs(scopeIDs)
	if err != nil {
		return repository.TemplatePage{}, err
	}
	boundary, err := templateCursor(r.cursors, page.Cursor)
	if err != nil {
		return repository.TemplatePage{}, err
	}

	rows, err := queries.ListTemplatesInScopes(ctx, sqlc.ListTemplatesInScopesParams{
		ScopeIds:        scopes,
		CursorCreatedAt: boundary.createdAt,
		CursorID:        boundary.id,
		PageSize:        pageProbe(page.Size),
	})
	if err != nil {
		return repository.TemplatePage{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the templates: %w", err))
	}

	templates := make([]work.Template, 0, len(rows))
	for _, row := range rows {
		template, err := templateFrom(
			row.ID, row.TenantID, row.ScopeType, row.ScopeID, row.Name, row.Description,
			string(row.RootType), row.Nodes, row.CreatedAt, row.UpdatedAt, row.DeletedAt,
			row.Version,
		)
		if err != nil {
			return repository.TemplatePage{}, err
		}
		templates = append(templates, template)
	}

	kept, info := pageOf(templates, page.Size, r.cursors, func(template work.Template) security.Position {
		return security.At(template.CreatedAt.UTC().Format(time.RFC3339Nano), template.ID)
	})
	return repository.TemplatePage{Templates: kept, Info: repository.PageInfo(info)}, nil
}

// Insert writes a new template.
func (r TemplateRepository) Insert(ctx context.Context, template work.Template) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(template.ID)
	if err != nil {
		return err
	}
	scopeID, err := optionalUUID(template.ScopeID)
	if err != nil {
		return err
	}
	nodes, err := encodeTemplateTree(template.Root)
	if err != nil {
		return err
	}

	err = queries.InsertTemplate(ctx, sqlc.InsertTemplateParams{
		ID:          id,
		ScopeType:   template.Scope.String(),
		ScopeID:     scopeID,
		Name:        template.Name,
		Description: optionalText(template.Description),
		RootType:    sqlc.ItemType(template.RootType),
		Nodes:       nodes,
		CreatedAt:   timestampOf(template.CreatedAt),
	})
	if err != nil {
		return insertTemplateError(err, template)
	}
	return nil
}

// Update writes the whole document under the optimistic lock. A deleted template is never matched:
// a change racing a deletion loses to it.
func (r TemplateRepository) Update(
	ctx context.Context, template work.Template, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(template.ID)
	if err != nil {
		return err
	}
	nodes, err := encodeTemplateTree(template.Root)
	if err != nil {
		return err
	}
	if template.UpdatedAt == nil {
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the change to template %s carries no stamp", template.ID))
	}

	affected, err := queries.UpdateTemplate(ctx, sqlc.UpdateTemplateParams{
		Name:        template.Name,
		Description: optionalText(template.Description),
		Nodes:       nodes,
		UpdatedAt:   timestampOf(*template.UpdatedAt),
		ID:          id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return insertTemplateError(err, template)
	}
	return templateConflictIfUntouched(affected, template.ID, expectedVersion)
}

// SetDeleted writes the soft deletion.
func (r TemplateRepository) SetDeleted(
	ctx context.Context, template work.Template, expectedVersion int,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(template.ID)
	if err != nil {
		return err
	}
	if template.DeletedAt == nil {
		return shared.ErrInternal.
			WithDetail("postgres.row_incoherent").
			WithCause(fmt.Errorf("the deletion of template %s carries no stamp", template.ID))
	}

	affected, err := queries.SetTemplateDeleted(ctx, sqlc.SetTemplateDeletedParams{
		DeletedAt: timestampOf(*template.DeletedAt),
		ID:        id,
		//nolint:gosec // G115: a version is a row counter, bounded by the number of updates a row has had
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("deleting the template %s: %w", template.ID, err))
	}
	return templateConflictIfUntouched(affected, template.ID, expectedVersion)
}

// insertTemplateError separates the one refusal a client can act on - the name is taken in this
// scope - from everything else.
func insertTemplateError(err error, template work.Template) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation &&
		pgErr.ConstraintName == templateNameIndex {
		return shared.ErrConflict.
			WithDetail("templates.name_taken").
			WithParams(map[string]string{"name": template.Name}).
			WithFields(shared.FieldError{Path: "/name", Code: "templates.name_taken"})
	}
	return shared.ErrUnavailable.
		WithDetail("postgres.query_failed").
		WithCause(fmt.Errorf("writing the template %s: %w", template.ID, err))
}

// templateConflictIfUntouched is the shared answer for a write that matched nothing: the row moved
// on, was deleted, or - through row level security - was never this tenant's to move. One answer
// for all three, deliberately (multi-tenancy.md §2).
func templateConflictIfUntouched(affected int64, id shared.ID, expectedVersion int) error {
	if affected != 0 {
		return nil
	}
	return shared.ErrVersionConflict.
		WithDetail("templates.version_conflict").
		WithParams(map[string]string{
			"template_id": id.String(), "expected_version": fmt.Sprint(expectedVersion),
		})
}

// storedNode is the tree as the column carries it. Its own shape rather than the domain's, for the
// reason every payload in this system has one: the row outlives the process that wrote it, and a
// type that changed shape in between would take the templates with it.
type storedNode struct {
	Type        string       `json:"type"`
	Title       string       `json:"title"`
	Notes       string       `json:"notes,omitempty"`
	DueOffset   string       `json:"due_offset,omitempty"`
	DueDateOnly bool         `json:"due_date_only,omitempty"`
	AssigneeID  string       `json:"assignee_id,omitempty"`
	Children    []storedNode `json:"children,omitempty"`
}

// encodeTemplateTree writes the tree as the document the column holds. The offset is stored as the
// ISO-8601 duration it was given rather than as a number of seconds: it is what somebody wrote,
// and a client reads it back to edit it.
func encodeTemplateTree(root work.TemplateNode) ([]byte, error) {
	document, err := json.Marshal(storeNode(root))
	if err != nil {
		return nil, shared.ErrInternal.
			WithDetail("templates.nodes_unencodable").
			WithCause(fmt.Errorf("encoding the template tree: %w", err))
	}
	return document, nil
}

func storeNode(node work.TemplateNode) storedNode {
	stored := storedNode{
		Type: string(node.Type), Title: node.Title, Notes: node.Notes,
		DueDateOnly: node.DueDateOnly, AssigneeID: node.AssigneeID.String(),
	}
	if node.DueOffset != nil {
		stored.DueOffset = work.SpellTemplateOffset(*node.DueOffset)
	}
	for _, child := range node.Children {
		stored.Children = append(stored.Children, storeNode(child))
	}
	return stored
}

// decodeTemplateTree reads the document back, and refuses one it cannot: a row this code cannot
// read is an internal fault rather than something a caller can fix.
func decodeTemplateTree(document []byte) (work.TemplateNode, error) {
	var stored storedNode
	if err := json.Unmarshal(document, &stored); err != nil {
		return work.TemplateNode{}, shared.ErrInternal.
			WithDetail("templates.nodes_undecodable").
			WithCause(fmt.Errorf("reading the template tree: %w", err))
	}
	return readNode(stored)
}

func readNode(stored storedNode) (work.TemplateNode, error) {
	node := work.TemplateNode{
		Type: work.ItemType(stored.Type), Title: stored.Title, Notes: stored.Notes,
		DueDateOnly: stored.DueDateOnly,
	}
	if stored.AssigneeID != "" {
		assignee, err := shared.ParseID(stored.AssigneeID)
		if err != nil {
			return work.TemplateNode{}, shared.ErrInternal.
				WithDetail("templates.nodes_undecodable").
				WithCause(fmt.Errorf("reading the assignee of a template node: %w", err))
		}
		node.AssigneeID = assignee
	}
	if stored.DueOffset != "" {
		offset, err := work.ParseTemplateOffset(stored.DueOffset)
		if err != nil {
			return work.TemplateNode{}, shared.ErrInternal.
				WithDetail("templates.nodes_undecodable").
				WithCause(fmt.Errorf("reading the offset of a template node: %w", err))
		}
		node.DueOffset = &offset
	}
	for _, child := range stored.Children {
		read, err := readNode(child)
		if err != nil {
			return work.TemplateNode{}, err
		}
		node.Children = append(node.Children, read)
	}
	return node, nil
}

// templateBoundary is a decoded template cursor, both fields absent for the first page.
type templateBoundary struct {
	createdAt pgtype.Timestamptz
	id        pgtype.UUID
}

func templateCursor(cursors security.CursorCodec, cursor string) (templateBoundary, error) {
	if cursor == "" {
		return templateBoundary{}, nil
	}

	position, err := cursors.Decode(cursor)
	if err != nil {
		return templateBoundary{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, position.SortKey())
	if err != nil {
		return templateBoundary{}, shared.ErrValidation.
			WithDetail("shared.cursor_invalid").WithCause(err)
	}
	id, err := uuidOf(position.ID)
	if err != nil {
		return templateBoundary{}, err
	}
	return templateBoundary{createdAt: timestampOf(createdAt), id: id}, nil
}

// scopeUUIDs maps the identifiers of a container's path onto the array the statement matches on.
func scopeUUIDs(ids []shared.ID) ([]pgtype.UUID, error) {
	scopes := make([]pgtype.UUID, 0, len(ids))
	for _, id := range ids {
		if id.IsZero() {
			continue
		}
		scope, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

// templateFrom maps a stored row onto the domain's template. One mapper for both selects, so the
// two cannot disagree about a field.
func templateFrom(
	id, tenantID pgtype.UUID, scopeType string, scopeID pgtype.UUID,
	name string, description *string, rootType string, nodes []byte,
	createdAt, updatedAt, deletedAt pgtype.Timestamptz, version int32,
) (work.Template, error) {
	templateID, err := idFrom(id)
	if err != nil {
		return work.Template{}, err
	}
	tenant, err := idFrom(tenantID)
	if err != nil {
		return work.Template{}, err
	}
	scope, err := optionalID(scopeID)
	if err != nil {
		return work.Template{}, err
	}
	root, err := decodeTemplateTree(nodes)
	if err != nil {
		return work.Template{}, err
	}
	if !createdAt.Valid {
		return work.Template{}, shared.ErrInternal.WithDetail("postgres.row_incoherent")
	}

	return work.Template{
		ID:          templateID,
		TenantID:    tenant,
		Scope:       work.TemplateScope(scopeType),
		ScopeID:     scope,
		Name:        name,
		Description: stringFrom(description),
		RootType:    work.ItemType(rootType),
		Root:        root,
		CreatedAt:   timeFrom(createdAt),
		UpdatedAt:   optionalTime(updatedAt),
		DeletedAt:   optionalTime(deletedAt),
		Version:     int(version),
	}, nil
}
