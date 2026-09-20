// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AutomationReferenceRepository answers the check's one question (ADR-0060): does something a
// rule names still exist. Seven statements for seven kinds, each under the tenant context the
// transaction wrapper set - so another workspace's object answers "no", which is what SG-3 asks
// of a lookup by identifier.
type AutomationReferenceRepository struct{}

// NewAutomationReferenceRepository returns the resolver.
func NewAutomationReferenceRepository() AutomationReferenceRepository {
	return AutomationReferenceRepository{}
}

var _ repository.References = AutomationReferenceRepository{}

// Exists answers for one kind. A kind this resolver does not know is an internal error rather
// than "no": answering false would make every parameter of an unclassified kind a finding, which
// is the check inventing things wrong with a rule.
func (r AutomationReferenceRepository) Exists(
	ctx context.Context, kind repository.ReferenceKind, id shared.ID,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return false, err
	}

	var exists bool
	switch kind {
	case repository.ReferenceLabel:
		exists, err = queries.LabelExists(ctx, key)
	case repository.ReferenceBucket:
		exists, err = queries.BucketExists(ctx, key)
	case repository.ReferenceContainer:
		exists, err = queries.ContainerExists(ctx, key)
	case repository.ReferenceTemplate:
		exists, err = queries.TemplateExists(ctx, key)
	case repository.ReferenceSubscription:
		exists, err = queries.WebhookSubscriptionExists(ctx, key)
	case repository.ReferenceGroup:
		exists, err = queries.AccountGroupExists(ctx, key)
	case repository.ReferenceAccount:
		exists, err = queries.ActingAccountExists(ctx, key)
	default:
		return false, shared.Internalf("automation: no resolver for references of kind %q", kind)
	}
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("resolving %s %s: %w", kind, id, err))
	}
	return exists, nil
}
