// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"

	repository "github.com/Jersyfi/hubtask/core/application/repository/meta"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
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
		capabilities := make([]work.Capability, 0, len(row.Capabilities))
		for _, capability := range row.Capabilities {
			capabilities = append(capabilities, work.Capability(capability))
		}
		children := make([]work.ItemType, 0, len(row.AllowedChildTypes))
		for _, child := range row.AllowedChildTypes {
			children = append(children, work.ItemType(child))
		}

		profiles = append(profiles, work.CapabilityProfile{
			Type:              work.ItemType(row.Type),
			Capabilities:      capabilities,
			AllowedChildTypes: children,
			MaxDepth:          int(row.MaxDepth),
		})
	}
	return profiles, nil
}
