// SPDX-License-Identifier: Apache-2.0
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

	repository "github.com/Jersyfi/hubtask/core/application/repository/config"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ConfigRepository stores the configuration document's versions, proposals and templates
// (migration 0221), through the unit of work's transaction (rule 3).
type ConfigRepository struct{}

func NewConfigRepository() ConfigRepository { return ConfigRepository{} }

var _ repository.Store = ConfigRepository{}

func configError(err error, what string) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func documentBytes(d domain.Document) ([]byte, error) {
	if d == nil {
		d = domain.Document{}
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("config.document_unwritable").WithCause(err)
	}
	return raw, nil
}

// documentFrom reads a stored document back into its sections; a block stored as null (a
// proposal's removal) stays nil.
func documentFrom(raw []byte) (domain.Document, error) {
	var sections map[string]map[string]map[string]any
	if err := json.Unmarshal(raw, &sections); err != nil {
		return nil, shared.ErrInternal.WithDetail("config.document_unreadable").WithCause(err)
	}
	out := make(domain.Document, len(sections))
	for name, section := range sections {
		blocks := make(domain.Section, len(section))
		for key, block := range section {
			if block == nil {
				blocks[key] = nil
				continue
			}
			blocks[key] = domain.Block(block)
		}
		out[name] = blocks
	}
	return out, nil
}

func textOf(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (ConfigRepository) Latest(ctx context.Context) (domain.Revision, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Revision{}, false, err
	}
	row, err := queries.LatestConfigRevision(ctx)
	if err != nil {
		if IsNoRows(err) {
			return domain.Revision{}, false, nil
		}
		return domain.Revision{}, false, configError(err, "reading the latest configuration version")
	}
	revision, err := revisionFrom(row)
	return revision, err == nil, err
}

func (ConfigRepository) Find(ctx context.Context, version int) (domain.Revision, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Revision{}, err
	}
	row, err := queries.FindConfigRevision(ctx, boundedInt32(version))
	if err != nil {
		if IsNoRows(err) {
			return domain.Revision{}, shared.ErrNotFound.WithDetail("config.version_unknown").
				WithParams(map[string]string{"version": fmt.Sprint(version)})
		}
		return domain.Revision{}, configError(err, "reading a configuration version")
	}
	return revisionFrom(row)
}

func revisionFrom(row sqlc.ConfigRevision) (domain.Revision, error) {
	document, err := documentFrom(row.Document)
	if err != nil {
		return domain.Revision{}, err
	}
	revision, err := revisionHead(row.TenantID, row.Version, row.Hash, row.PlanHash, row.ActorKind, row.ActorID,
		row.ProposedBy, row.RolledBackTo, row.Template, row.Added, row.Changed, row.Removed, row.CreatedAt, row.HubID)
	revision.Document = document
	return revision, err
}

func revisionHead(
	tenantID pgtype.UUID, version int32, hash, planHash, actorKind string, actorID, proposedBy pgtype.UUID,
	rolledBackTo *int32, template *string, added, changed, removed int32, at pgtype.Timestamptz, hubID pgtype.UUID,
) (domain.Revision, error) {
	tenant, err := idFrom(tenantID)
	if err != nil {
		return domain.Revision{}, err
	}
	hub, err := optionalID(hubID)
	if err != nil {
		return domain.Revision{}, err
	}
	actor, err := optionalID(actorID)
	if err != nil {
		return domain.Revision{}, err
	}
	proposer, err := optionalID(proposedBy)
	if err != nil {
		return domain.Revision{}, err
	}
	revision := domain.Revision{
		TenantID: tenant, Version: int(version), Hash: hash, PlanHash: planHash,
		ActorKind: shared.ActorKind(actorKind), ActorID: actor, ProposedBy: proposer,
		Template: textOf(template), At: timeFrom(at), HubID: hub,
		Counts: domain.Counts{Added: int(added), Changed: int(changed), Removed: int(removed)},
	}
	if rolledBackTo != nil {
		revision.RolledBackTo = int(*rolledBackTo)
	}
	return revision, nil
}

func (ConfigRepository) List(ctx context.Context, hub shared.ID, before, limit int) ([]domain.Revision, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hubID, err := optionalUUID(hub)
	if err != nil {
		return nil, err
	}
	params := sqlc.ListConfigRevisionsParams{MaxRows: boundedInt32(limit), HubID: hubID}
	if before > 0 {
		b := boundedInt32(before)
		params.Before = &b
	}
	rows, err := queries.ListConfigRevisions(ctx, params)
	if err != nil {
		return nil, configError(err, "listing the configuration versions")
	}
	out := make([]domain.Revision, 0, len(rows))
	for _, row := range rows {
		revision, err := revisionHead(row.TenantID, row.Version, row.Hash, row.PlanHash, row.ActorKind, row.ActorID,
			row.ProposedBy, row.RolledBackTo, row.Template, row.Added, row.Changed, row.Removed, row.CreatedAt, row.HubID)
		if err != nil {
			return nil, err
		}
		out = append(out, revision)
	}
	return out, nil
}

func (ConfigRepository) Insert(ctx context.Context, revision domain.Revision) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := documentBytes(revision.Document)
	if err != nil {
		return err
	}
	actor, err := optionalUUID(revision.ActorID)
	if err != nil {
		return err
	}
	proposer, err := optionalUUID(revision.ProposedBy)
	if err != nil {
		return err
	}
	hub, err := optionalUUID(revision.HubID)
	if err != nil {
		return err
	}
	params := sqlc.InsertConfigRevisionParams{
		HubID:   hub,
		Version: boundedInt32(revision.Version), Document: document, Hash: revision.Hash, PlanHash: revision.PlanHash,
		ActorKind: string(revision.ActorKind), ActorID: actor, ProposedBy: proposer,
		Template: optionalText(revision.Template), Added: boundedInt32(revision.Counts.Added),
		Changed: boundedInt32(revision.Counts.Changed), Removed: boundedInt32(revision.Counts.Removed),
		At: timestampOf(revision.At),
	}
	if revision.RolledBackTo > 0 {
		to := boundedInt32(revision.RolledBackTo)
		params.RolledBackTo = &to
	}
	err = queries.InsertConfigRevision(ctx, params)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return domain.ErrVersionConflict()
	}
	if err != nil {
		return configError(err, "writing a configuration version")
	}
	return nil
}

func (ConfigRepository) InsertProposal(ctx context.Context, p domain.Proposal) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := documentBytes(p.Document)
	if err != nil {
		return err
	}
	id, err := uuidOf(p.ID)
	if err != nil {
		return err
	}
	proposer, err := optionalUUID(p.ProposedBy)
	if err != nil {
		return err
	}
	hub, err := optionalUUID(p.HubID)
	if err != nil {
		return err
	}
	err = queries.InsertConfigProposal(ctx, sqlc.InsertConfigProposalParams{
		ID: id, HubID: hub, Document: document, Mode: string(p.Mode), BaseHash: p.BaseHash, PlanHash: p.PlanHash,
		Template: optionalText(p.Template), Added: boundedInt32(p.Counts.Added), Changed: boundedInt32(p.Counts.Changed),
		Removed: boundedInt32(p.Counts.Removed), HidesData: p.Flags.HidesData,
		ChangesPermissions: p.Flags.ChangesPermissions, ProposedBy: proposer, At: timestampOf(p.CreatedAt),
	})
	if err != nil {
		return configError(err, "writing a configuration proposal")
	}
	return nil
}

func (ConfigRepository) FindProposal(ctx context.Context, id shared.ID) (domain.Proposal, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Proposal{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Proposal{}, err
	}
	row, err := queries.FindConfigProposal(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Proposal{}, shared.ErrNotFound.WithDetail("config.proposal_unknown")
		}
		return domain.Proposal{}, configError(err, "reading a configuration proposal")
	}
	return proposalFrom(row)
}

func proposalFrom(row sqlc.ConfigProposal) (domain.Proposal, error) {
	document, err := documentFrom(row.Document)
	if err != nil {
		return domain.Proposal{}, err
	}
	ids := make([]shared.ID, 5)
	for i, value := range []pgtype.UUID{row.ID, row.TenantID, row.ProposedBy, row.DecidedBy, row.HubID} {
		id, err := optionalID(value)
		if err != nil {
			return domain.Proposal{}, err
		}
		ids[i] = id
	}
	return domain.Proposal{
		ID: ids[0], TenantID: ids[1], State: domain.ProposalState(row.State), Document: document,
		Mode: domain.Mode(row.Mode), BaseHash: row.BaseHash, PlanHash: row.PlanHash, Template: textOf(row.Template),
		Counts:     domain.Counts{Added: int(row.Added), Changed: int(row.Changed), Removed: int(row.Removed)},
		Flags:      domain.Flags{HidesData: row.HidesData, ChangesPermissions: row.ChangesPermissions},
		ProposedBy: ids[2], DecidedBy: ids[3], HubID: ids[4], CreatedAt: timeFrom(row.CreatedAt), DecidedAt: optionalTime(row.DecidedAt),
	}, nil
}

func (ConfigRepository) ListProposals(ctx context.Context, state domain.ProposalState, limit int) ([]domain.Proposal, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	params := sqlc.ListConfigProposalsParams{MaxRows: boundedInt32(limit)}
	if state != "" {
		s := string(state)
		params.State = &s
	}
	rows, err := queries.ListConfigProposals(ctx, params)
	if err != nil {
		return nil, configError(err, "listing the configuration proposals")
	}
	out := make([]domain.Proposal, 0, len(rows))
	for _, row := range rows {
		proposal, err := proposalFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, proposal)
	}
	return out, nil
}

func (ConfigRepository) DecideProposal(
	ctx context.Context, id shared.ID, state domain.ProposalState, by shared.ID, at time.Time,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}
	decider, err := optionalUUID(by)
	if err != nil {
		return false, err
	}
	rows, err := queries.DecideConfigProposal(ctx, sqlc.DecideConfigProposalParams{
		State: string(state), DecidedBy: decider, At: timestampOf(at), ID: key,
	})
	if err != nil {
		return false, configError(err, "deciding a configuration proposal")
	}
	return rows == 1, nil
}

func (ConfigRepository) InsertTemplate(ctx context.Context, t domain.Template) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := documentBytes(t.Document)
	if err != nil {
		return err
	}
	id, err := uuidOf(t.ID)
	if err != nil {
		return err
	}
	creator, err := optionalUUID(t.CreatedBy)
	if err != nil {
		return err
	}
	var hub []byte
	if t.ForHubs() {
		if hub, err = documentBytes(t.Hub); err != nil {
			return err
		}
	}
	err = queries.InsertConfigTemplate(ctx, sqlc.InsertConfigTemplateParams{
		ID: id, Key: t.Key, Version: boundedInt32(t.Version), Name: t.Name, Origin: string(t.Origin),
		Document: document, CreatedBy: creator, At: timestampOf(t.CreatedAt), HubDocument: hub,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return shared.ErrConflict.WithDetail("config.template_version_taken").
			WithParams(map[string]string{"template": t.Key})
	}
	if err != nil {
		return configError(err, "writing a template version")
	}
	return nil
}

func configTemplateFrom(row sqlc.ConfigTemplate) (domain.Template, error) {
	document, err := documentFrom(row.Document)
	if err != nil {
		return domain.Template{}, err
	}
	ids := make([]shared.ID, 3)
	for i, value := range []pgtype.UUID{row.ID, row.TenantID, row.CreatedBy} {
		id, err := optionalID(value)
		if err != nil {
			return domain.Template{}, err
		}
		ids[i] = id
	}
	var hub domain.Document
	if row.HubDocument != nil {
		if hub, err = documentFrom(row.HubDocument); err != nil {
			return domain.Template{}, err
		}
		if hub == nil {
			hub = domain.Document{}
		}
	}
	return domain.Template{
		ID: ids[0], TenantID: ids[1], Key: row.Key, Version: int(row.Version), Name: row.Name,
		Origin: domain.TemplateOrigin(row.Origin), Document: document, Hub: hub, CreatedBy: ids[2],
		CreatedAt: timeFrom(row.CreatedAt),
	}, nil
}

func (ConfigRepository) ListTemplates(ctx context.Context) ([]domain.Template, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListConfigTemplates(ctx)
	if err != nil {
		return nil, configError(err, "listing the templates")
	}
	out := make([]domain.Template, 0, len(rows))
	for _, row := range rows {
		t, err := configTemplateFrom(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (ConfigRepository) FindTemplate(ctx context.Context, key string, version int) (domain.Template, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Template{}, err
	}
	row, err := queries.FindConfigTemplate(ctx, sqlc.FindConfigTemplateParams{Key: key, Version: boundedInt32(version)})
	if err != nil {
		if IsNoRows(err) {
			return domain.Template{}, domain.ErrTemplateUnknown(key)
		}
		return domain.Template{}, configError(err, "reading a template version")
	}
	return configTemplateFrom(row)
}

func (ConfigRepository) PutTemplateUse(ctx context.Context, use domain.TemplateUse) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := documentBytes(use.Document)
	if err != nil {
		return err
	}
	if !use.HubID.IsZero() {
		hub, err := uuidOf(use.HubID)
		if err != nil {
			return err
		}
		if err := queries.UpsertConfigHubTemplateUse(ctx, sqlc.UpsertConfigHubTemplateUseParams{
			HubID: hub, TemplateKey: use.TemplateKey, Version: boundedInt32(use.Version), Document: document, At: timestampOf(use.At),
		}); err != nil {
			return configError(err, "recording a template a hub took")
		}
		return nil
	}
	if err := queries.UpsertConfigTemplateUse(ctx, sqlc.UpsertConfigTemplateUseParams{
		TemplateKey: use.TemplateKey, Version: boundedInt32(use.Version), Document: document, At: timestampOf(use.At),
	}); err != nil {
		return configError(err, "recording a template taken")
	}
	return nil
}

func (ConfigRepository) TemplateUses(ctx context.Context, hub shared.ID) ([]domain.TemplateUse, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	if !hub.IsZero() {
		return hubTemplateUses(ctx, queries, hub)
	}
	rows, err := queries.ListConfigTemplateUses(ctx)
	if err != nil {
		return nil, configError(err, "listing the templates taken")
	}
	out := make([]domain.TemplateUse, 0, len(rows))
	for _, row := range rows {
		document, err := documentFrom(row.Document)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.TemplateUse{
			TemplateKey: row.TemplateKey, Version: int(row.Version), Document: document, At: timeFrom(row.AppliedAt),
		})
	}
	return out, nil
}

func hubTemplateUses(ctx context.Context, queries *sqlc.Queries, hub shared.ID) ([]domain.TemplateUse, error) {
	id, err := uuidOf(hub)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListConfigHubTemplateUses(ctx, id)
	if err != nil {
		return nil, configError(err, "listing the templates a hub took")
	}
	out := make([]domain.TemplateUse, 0, len(rows))
	for _, row := range rows {
		document, err := documentFrom(row.Document)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.TemplateUse{
			HubID: hub, TemplateKey: row.TemplateKey, Version: int(row.Version), Document: document, At: timeFrom(row.AppliedAt),
		})
	}
	return out, nil
}

func (ConfigRepository) Sandbox(ctx context.Context) (domain.Sandbox, bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Sandbox{}, false, err
	}
	row, err := queries.FindConfigSandbox(ctx)
	if err != nil {
		if IsNoRows(err) {
			return domain.Sandbox{}, false, nil
		}
		return domain.Sandbox{}, false, configError(err, "reading the configuration sandbox")
	}
	tenant, err := idFrom(row.TenantID)
	if err != nil {
		return domain.Sandbox{}, false, err
	}
	document, err := documentFrom(row.Document)
	if err != nil {
		return domain.Sandbox{}, false, err
	}
	base, err := documentFrom(row.BaseDocument)
	if err != nil {
		return domain.Sandbox{}, false, err
	}
	return domain.Sandbox{
		TenantID: tenant, Document: document, Base: base, BaseHash: row.BaseHash,
		BaseVersion: int(row.BaseVersion), Writes: int(row.Writes), UpdatedAt: timeFrom(row.UpdatedAt),
	}, true, nil
}

func (ConfigRepository) PutSandbox(ctx context.Context, sandbox domain.Sandbox) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	document, err := documentBytes(sandbox.Document)
	if err != nil {
		return err
	}
	base, err := documentBytes(sandbox.Base)
	if err != nil {
		return err
	}
	if err := queries.PutConfigSandbox(ctx, sqlc.PutConfigSandboxParams{
		Document: document, BaseDocument: base, BaseHash: sandbox.BaseHash,
		BaseVersion: boundedInt32(sandbox.BaseVersion), Writes: boundedInt32(sandbox.Writes), At: timestampOf(sandbox.UpdatedAt),
	}); err != nil {
		return configError(err, "writing the configuration sandbox")
	}
	return nil
}

func (ConfigRepository) DropSandbox(ctx context.Context) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	rows, err := queries.DropConfigSandbox(ctx)
	if err != nil {
		return false, configError(err, "discarding the configuration sandbox")
	}
	return rows == 1, nil
}
