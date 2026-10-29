// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package config is the port of the configuration document's store: its versions, the agents'
// plans waiting for a person, and the workspace's templates with the ones it took (F-6).
package config

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Store keeps the configuration's history in the transaction's workspace.
type Store interface {
	// Latest answers the newest version, false where none was applied yet.
	Latest(ctx context.Context) (domain.Revision, bool, error)
	// Find answers one version with its document; not found is `config.version_unknown`.
	Find(ctx context.Context, version int) (domain.Revision, error)
	// List answers versions newest first, without their documents, before a version (0 for the
	// newest).
	List(ctx context.Context, before, limit int) ([]domain.Revision, error)
	// Insert writes the next version; a version taken by a concurrent apply is
	// `config.version_conflict`.
	Insert(ctx context.Context, revision domain.Revision) error

	InsertProposal(ctx context.Context, proposal domain.Proposal) error
	FindProposal(ctx context.Context, id shared.ID) (domain.Proposal, error)
	// ListProposals answers proposals newest first, in one state or every state for "".
	ListProposals(ctx context.Context, state domain.ProposalState, limit int) ([]domain.Proposal, error)
	// DecideProposal moves a waiting proposal on, and reports false where it was not waiting.
	DecideProposal(ctx context.Context, id shared.ID, state domain.ProposalState, by shared.ID, at time.Time) (bool, error)

	// InsertTemplate writes one template version; a version taken is `config.template_version_taken`.
	InsertTemplate(ctx context.Context, template domain.Template) error
	// ListTemplates answers every version of every template, newest version first within a key.
	ListTemplates(ctx context.Context) ([]domain.Template, error)
	FindTemplate(ctx context.Context, key string, version int) (domain.Template, error)
	// PutTemplateUse records the template version the workspace took.
	PutTemplateUse(ctx context.Context, use domain.TemplateUse) error
	TemplateUses(ctx context.Context) ([]domain.TemplateUse, error)
}
