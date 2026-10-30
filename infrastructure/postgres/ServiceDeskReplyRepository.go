// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

var _ repository.ReplyTemplates = ServiceDeskReplies{}

// ServiceDeskReplies stores the workspace's canned replies (migration 0242).
type ServiceDeskReplies struct{}

func (ServiceDeskRepository) Replies() ServiceDeskReplies { return ServiceDeskReplies{} }

func (ServiceDeskReplies) Insert(ctx context.Context, template servicedesk.ReplyTemplate) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(template.ID)
	if err != nil {
		return err
	}
	tenant, err := uuidOf(template.TenantID)
	if err != nil {
		return err
	}
	if err := queries.InsertReplyTemplate(ctx, sqlc.InsertReplyTemplateParams{
		ID: id, TenantID: tenant, Key: template.Key, Name: template.Name, Body: template.Body,
		Now: timestampOf(template.CreatedAt),
	}); err != nil {
		if isUniqueViolation(err) {
			return shared.ErrConflict.WithDetail("reply_templates.key_taken").WithParams(map[string]string{"key": template.Key})
		}
		return queryFailed("writing a reply template", err)
	}
	return nil
}

func (ServiceDeskReplies) Update(ctx context.Context, template servicedesk.ReplyTemplate, expectedVersion int) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.UpdateReplyTemplate(ctx, sqlc.UpdateReplyTemplateParams{
		Name: template.Name, Body: template.Body, Now: timestampOf(template.UpdatedAt), Key: template.Key,
		ExpectedVersion: int32(expectedVersion), //nolint:gosec // a version is far below 2^31
	})
	if err != nil {
		return queryFailed("updating a reply template", err)
	}
	if affected == 0 {
		return shared.ErrVersionConflict.WithDetail("reply_templates.version_conflict")
	}
	return nil
}

func replyTemplateFrom(row sqlc.ReplyTemplate) (servicedesk.ReplyTemplate, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return servicedesk.ReplyTemplate{}, err
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return servicedesk.ReplyTemplate{}, err
	}
	return servicedesk.ReplyTemplate{
		ID: id, TenantID: tenant, Key: row.Key, Name: row.Name, Body: row.Body,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time, Version: int(row.Version),
	}, nil
}

func (ServiceDeskReplies) Find(ctx context.Context, key string) (servicedesk.ReplyTemplate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return servicedesk.ReplyTemplate{}, err
	}
	row, err := queries.FindReplyTemplate(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return servicedesk.ReplyTemplate{}, shared.ErrNotFound.WithDetail("reply_templates.not_found")
		}
		return servicedesk.ReplyTemplate{}, queryFailed("reading a reply template", err)
	}
	return replyTemplateFrom(row)
}

func (ServiceDeskReplies) List(ctx context.Context) ([]servicedesk.ReplyTemplate, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListReplyTemplates(ctx)
	if err != nil {
		return nil, queryFailed("listing reply templates", err)
	}
	out := make([]servicedesk.ReplyTemplate, 0, len(rows))
	for _, row := range rows {
		template, err := replyTemplateFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, template)
	}
	return out, nil
}

func (ServiceDeskReplies) Delete(ctx context.Context, key string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	affected, err := queries.DeleteReplyTemplate(ctx, key)
	if err != nil {
		return queryFailed("deleting a reply template", err)
	}
	if affected == 0 {
		return shared.ErrNotFound.WithDetail("reply_templates.not_found")
	}
	return nil
}
