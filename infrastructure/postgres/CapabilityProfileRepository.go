// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CapabilityProfileRepository reads the profile in force per item type.
type CapabilityProfileRepository struct{}

func NewCapabilityProfileRepository() CapabilityProfileRepository {
	return CapabilityProfileRepository{}
}

var _ repository.CapabilityProfiles = CapabilityProfileRepository{}

// List returns one profile per item type. Which one - the tenant's override or the system default
// - is decided by the query; what is visible at all is decided by row level security and by the
// scope the caller opened (ADR-0010).
func (r CapabilityProfileRepository) List(ctx context.Context) ([]work.CapabilityProfile, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListCapabilityProfiles(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the capability profiles: %w", err))
	}

	profiles := make([]work.CapabilityProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, profileFrom(
			row.Type, row.Capabilities, row.AllowedChildTypes, row.MaxDepth))
	}
	return profiles, nil
}

// ListSystem returns the defaults, ignoring whatever this tenant has overridden. They bound what
// a narrowing may do, and the hierarchy reads the topology off them for that reason.
func (r CapabilityProfileRepository) ListSystem(ctx context.Context) ([]work.CapabilityProfile, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := queries.ListSystemCapabilityProfiles(ctx)
	if err != nil {
		return nil, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the system capability profiles: %w", err))
	}

	profiles := make([]work.CapabilityProfile, 0, len(rows))
	for _, row := range rows {
		profiles = append(profiles, profileFrom(
			row.Type, row.Capabilities, row.AllowedChildTypes, row.MaxDepth))
	}
	return profiles, nil
}

func profileFrom(
	itemType sqlc.ItemType, rawCapabilities []string, rawChildren []sqlc.ItemType, maxDepth int32,
) work.CapabilityProfile {
	capabilities := make([]work.Capability, 0, len(rawCapabilities))
	for _, capability := range rawCapabilities {
		capabilities = append(capabilities, work.Capability(capability))
	}
	children := make([]work.ItemType, 0, len(rawChildren))
	for _, child := range rawChildren {
		children = append(children, work.ItemType(child))
	}

	return work.CapabilityProfile{
		Type:              work.ItemType(itemType),
		Capabilities:      capabilities,
		AllowedChildTypes: children,
		MaxDepth:          int(maxDepth),
	}
}
