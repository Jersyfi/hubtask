// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// workflowKeyIndex is the unique constraint a taken key violates (migration 0216).
const workflowKeyIndex = "workflow_key_uq"

// WorkflowRepository stores a workspace's workflows and scheme (UC-FLW-02 … 05), through the
// transaction the unit of work opened, so row level security decides whose rows it sees (rule 3).
type WorkflowRepository struct{}

func NewWorkflowRepository() WorkflowRepository { return WorkflowRepository{} }

var _ repository.Workflows = WorkflowRepository{}

// The shape's document: the application's WorkflowShape with the contract's field names.
type workflowShapeDocument struct {
	Statuses []workflowStatusDocument `json:"statuses"`
	Initial  string                   `json:"initial"`
	Steps    []workflowStepDocument   `json:"steps"`
}

type workflowStatusDocument struct {
	Key        string `json:"key"`
	Name       string `json:"name,omitempty"`
	Category   string `json:"category"`
	ColorToken string `json:"color_token"`
}

type workflowStepDocument struct {
	Key      string                  `json:"key"`
	Name     string                  `json:"name,omitempty"`
	From     []string                `json:"from"`
	To       string                  `json:"to"`
	Who      workflowWhoDocument     `json:"who"`
	Requires []string                `json:"requires,omitempty"`
	Asks     []string                `json:"asks,omitempty"`
	Checks   []workflowCheckDocument `json:"checks,omitempty"`
}

type workflowWhoDocument struct {
	Roles        []string `json:"roles,omitempty"`
	PersonFields []string `json:"person_fields,omitempty"`
	Groups       []string `json:"groups,omitempty"`
	Condition    string   `json:"condition,omitempty"`
}

type workflowCheckDocument struct {
	Key         string            `json:"key"`
	Holds       string            `json:"holds"`
	Message     string            `json:"message,omitempty"`
	Messages    map[string]string `json:"messages,omitempty"`
	MessageCode string            `json:"message_code,omitempty"`
}

func shapeColumn(shape work.WorkflowShape) ([]byte, error) {
	doc := workflowShapeDocument{Initial: shape.Initial, Statuses: []workflowStatusDocument{}, Steps: []workflowStepDocument{}}
	for _, status := range shape.Statuses {
		doc.Statuses = append(doc.Statuses, workflowStatusDocument{
			Key: status.Key, Name: status.Name, Category: string(status.Category), ColorToken: status.ColorToken,
		})
	}
	for _, step := range shape.Steps {
		checks := make([]workflowCheckDocument, 0, len(step.Checks))
		for _, check := range step.Checks {
			checks = append(checks, workflowCheckDocument(check))
		}
		doc.Steps = append(doc.Steps, workflowStepDocument{
			Key: step.Key, Name: step.Name, From: nonNil(step.From), To: step.To,
			Who:      workflowWhoDocument(step.Who),
			Requires: step.Requires, Asks: step.Asks, Checks: checks,
		})
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("postgres.encode_failed").WithCause(err)
	}
	return encoded, nil
}

func shapeFrom(raw []byte) (work.WorkflowShape, error) {
	var doc workflowShapeDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return work.WorkflowShape{}, shared.ErrInternal.WithDetail("postgres.decode_failed").WithCause(err)
	}
	shape := work.WorkflowShape{Initial: doc.Initial}
	for _, status := range doc.Statuses {
		shape.Statuses = append(shape.Statuses, work.WorkflowStatus{
			Key: status.Key, Name: status.Name, Category: work.StatusCategory(status.Category), ColorToken: status.ColorToken,
		})
	}
	for _, step := range doc.Steps {
		checks := make([]work.StepCheck, 0, len(step.Checks))
		for _, check := range step.Checks {
			checks = append(checks, work.StepCheck(check))
		}
		shape.Steps = append(shape.Steps, work.WorkflowStep{
			Key: step.Key, Name: step.Name, From: step.From, To: step.To, Who: work.StepWho(step.Who),
			Requires: step.Requires, Asks: step.Asks, Checks: checks,
		})
	}
	return shape, nil
}

func workflowColumns(workflow work.Workflow) (published, draft []byte, err error) {
	published, err = shapeColumn(workflow.Published)
	if err != nil || workflow.Draft == nil {
		return published, nil, err
	}
	draft, err = shapeColumn(*workflow.Draft)
	return published, draft, err
}

// List returns the workspace's rows by key.
func (r WorkflowRepository) List(ctx context.Context) ([]work.Workflow, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListWorkflows(ctx)
	if err != nil {
		return nil, workflowError(err, "listing the workflows")
	}
	out := make([]work.Workflow, 0, len(rows))
	for _, row := range rows {
		workflow, err := workflowFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, workflow)
	}
	return out, nil
}

// Insert writes a new row.
func (r WorkflowRepository) Insert(ctx context.Context, workflow work.Workflow) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(workflow.ID)
	if err != nil {
		return err
	}
	by, err := optionalUUID(workflow.PublishedBy)
	if err != nil {
		return err
	}
	published, draft, err := workflowColumns(workflow)
	if err != nil {
		return err
	}
	err = queries.InsertWorkflow(ctx, sqlc.InsertWorkflowParams{
		ID: id, Key: workflow.Key, System: workflow.System, Name: workflow.Name,
		Published: published, Draft: draft, PublishedAt: optionalTimestamp(workflow.PublishedAt),
		PublishedBy: by, CreatedAt: timestampOf(workflow.CreatedAt),
	})
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == workflowKeyIndex {
		return shared.ErrConflict.WithDetail("workflows.key_taken").WithParams(map[string]string{"key": workflow.Key})
	}
	return workflowError(err, "writing the workflow")
}

// Update writes a row whole under the optimistic lock.
func (r WorkflowRepository) Update(ctx context.Context, workflow work.Workflow, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	by, err := optionalUUID(workflow.PublishedBy)
	if err != nil {
		return err
	}
	published, draft, err := workflowColumns(workflow)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateWorkflow(ctx, sqlc.UpdateWorkflowParams{
		Name: workflow.Name, Published: published, Draft: draft,
		PublishedAt: optionalTimestamp(workflow.PublishedAt), PublishedBy: by,
		UpdatedAt: timestampOf(workflow.UpdatedAt), Key: workflow.Key,
		//nolint:gosec // G115: a version is a row counter
		ExpectedVersion: int32(expectedVersion),
	})
	if err != nil {
		return workflowError(err, "updating the workflow")
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("workflows.version_conflict").
			WithParams(map[string]string{"key": workflow.Key, "expected_version": strconv.Itoa(expectedVersion)})
	}
	return nil
}

// Delete removes a workflow no scheme names.
func (r WorkflowRepository) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteWorkflow(ctx, key)
	if err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrConflict.WithDetail("workflows.in_use")
		}
		return workflowError(err, "deleting the workflow")
	}
	if affected > 0 {
		return nil
	}
	schemes, err := r.Schemes(ctx)
	if err != nil {
		return err
	}
	for _, scheme := range schemes {
		if scheme.WorkflowKey == key {
			return shared.ErrConflict.WithDetail("workflows.in_use")
		}
	}
	return shared.ErrNotFound.WithDetail("workflows.not_found")
}

// Schemes returns every row of the scheme.
func (r WorkflowRepository) Schemes(ctx context.Context) ([]work.WorkflowScheme, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListWorkflowSchemes(ctx)
	if err != nil {
		return nil, workflowError(err, "listing the workflow scheme")
	}
	out := make([]work.WorkflowScheme, 0, len(rows))
	for _, row := range rows {
		container, err := idFrom(row.ContainerID)
		if err != nil {
			return nil, err
		}
		out = append(out, work.WorkflowScheme{ContainerID: container, EntryType: row.EntryType, WorkflowKey: row.WorkflowKey})
	}
	return out, nil
}

// SetScheme writes a container's row for a type.
func (r WorkflowRepository) SetScheme(ctx context.Context, scheme work.WorkflowScheme, now time.Time) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	container, err := uuidOf(scheme.ContainerID)
	if err != nil {
		return err
	}
	if err := queries.SetWorkflowScheme(ctx, sqlc.SetWorkflowSchemeParams{
		ContainerID: container, EntryType: scheme.EntryType, WorkflowKey: scheme.WorkflowKey,
		CreatedAt: timestampOf(now),
	}); err != nil {
		if isForeignKeyViolation(err) {
			return shared.ErrNotFound.WithDetail("workflows.not_found")
		}
		return workflowError(err, "writing the workflow scheme")
	}
	return nil
}

// RemoveScheme removes a container's row for a type.
func (r WorkflowRepository) RemoveScheme(ctx context.Context, containerID shared.ID, entryType string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	container, err := uuidOf(containerID)
	if err != nil {
		return err
	}
	if _, err := queries.DeleteWorkflowScheme(ctx, sqlc.DeleteWorkflowSchemeParams{
		ContainerID: container, EntryType: entryType,
	}); err != nil {
		return workflowError(err, "removing the workflow scheme")
	}
	return nil
}

// StatusGroups counts the entries of the collections by type, status and completion.
func (r WorkflowRepository) StatusGroups(ctx context.Context, collectionIDs []shared.ID) ([]work.StatusGroup, error) {
	if len(collectionIDs) == 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]pgtype.UUID, 0, len(collectionIDs))
	for _, id := range collectionIDs {
		converted, err := uuidOf(id)
		if err != nil {
			return nil, err
		}
		ids = append(ids, converted)
	}
	rows, err := queries.WorkflowStatusGroups(ctx, ids)
	if err != nil {
		return nil, workflowError(err, "counting the entries by status")
	}
	out := make([]work.StatusGroup, 0, len(rows))
	for _, row := range rows {
		collection, err := idFrom(row.CollectionID)
		if err != nil {
			return nil, err
		}
		out = append(out, work.StatusGroup{
			CollectionID: collection, EntryType: row.EntryType, Status: row.Status,
			Completed: row.IsCompleted, Count: int(row.Count),
		})
	}
	return out, nil
}

// MoveStatuses moves one group's entries.
func (r WorkflowRepository) MoveStatuses(ctx context.Context, move work.StatusMove, now time.Time) ([]repository.MovedEntry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	collection, err := uuidOf(move.CollectionID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.MoveWorkItemStatuses(ctx, sqlc.MoveWorkItemStatusesParams{
		ToStatus: optionalText(move.To), UpdatedAt: timestampOf(now), CollectionID: collection,
		EntryType: move.EntryType, FromStatus: optionalText(move.From), IsCompleted: move.Completed,
	})
	if err != nil {
		return nil, workflowError(err, "moving the entries' statuses")
	}
	out := make([]repository.MovedEntry, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, repository.MovedEntry{ID: id, Version: int(row.Version)})
	}
	return out, nil
}

func workflowFrom(row sqlc.Workflow) (work.Workflow, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return work.Workflow{}, err
	}
	tenantID, err := idFrom(row.TenantID)
	if err != nil {
		return work.Workflow{}, err
	}
	by, err := optionalID(row.PublishedBy)
	if err != nil {
		return work.Workflow{}, err
	}
	published, err := shapeFrom(row.Published)
	if err != nil {
		return work.Workflow{}, err
	}
	workflow := work.Workflow{
		ID: id, TenantID: tenantID, Key: row.Key, System: row.System, Name: row.Name,
		Published: published, PublishedAt: optionalTime(row.PublishedAt), PublishedBy: by,
		CreatedAt: timeFrom(row.CreatedAt), UpdatedAt: timeFrom(row.UpdatedAt), Version: int(row.Version),
	}
	if len(row.Draft) > 0 {
		draft, err := shapeFrom(row.Draft)
		if err != nil {
			return work.Workflow{}, err
		}
		workflow.Draft = &draft
	}
	return workflow, nil
}

func workflowError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}
