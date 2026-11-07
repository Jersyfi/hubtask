// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Assurance keeps what vouches for an inventory: the CIs entries name as affected, certification
// campaigns and their attestations, a hub's CI groups, the CIs' asset records and the workspace's
// software aliases. Another workspace's rows are not found.
type Assurance interface {
	// AddAffected names a CI as affected by an entry; it answers false when it was named already.
	AddAffected(ctx context.Context, a cmdb.Affected) (bool, error)
	// RemoveAffected answers the row removed; not found when the entry did not name the CI.
	RemoveAffected(ctx context.Context, itemID, ciID shared.ID) (cmdb.Affected, error)
	AffectedOf(ctx context.Context, itemID shared.ID) ([]cmdb.Affected, error)
	// AffectingAny answers the rows naming any of the CIs, the latest first, up to limit.
	AffectingAny(ctx context.Context, ciIDs []shared.ID, limit int) ([]cmdb.Affected, error)

	// InsertCampaign writes a campaign with one pending attestation per CI asked.
	InsertCampaign(ctx context.Context, c cmdb.Campaign, asked []cmdb.Attestation) error
	FindCampaign(ctx context.Context, id shared.ID) (cmdb.Campaign, error)
	Campaigns(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Campaign, error)
	// CloseCampaign closes an open campaign at the version expected.
	CloseCampaign(ctx context.Context, id shared.ID, at time.Time, by shared.ID, expected int) error
	Attestations(ctx context.Context, campaignID shared.ID) ([]cmdb.Attestation, error)
	// Tallies counts each campaign's answers by verdict.
	Tallies(ctx context.Context, campaignIDs []shared.ID) (map[shared.ID]map[cmdb.Verdict]int, error)
	// FindAttestation holds the row for the transaction; not found when the CI was not asked.
	FindAttestation(ctx context.Context, campaignID, ciID shared.ID) (cmdb.Attestation, error)
	Answer(ctx context.Context, a cmdb.Attestation) error

	// PutGroup writes a group at the version expected; zero expects a new key.
	PutGroup(ctx context.Context, g cmdb.Group, expected int) error
	Groups(ctx context.Context, hubID shared.ID) ([]cmdb.Group, error)
	FindGroup(ctx context.Context, id shared.ID) (cmdb.Group, error)

	// PutAsset writes a CI's asset record at the version expected; zero expects none.
	PutAsset(ctx context.Context, a cmdb.Asset, expected int) error
	FindAsset(ctx context.Context, ciID shared.ID) (cmdb.Asset, error)
	Assets(ctx context.Context, hubID, after shared.ID, limit int) ([]cmdb.Asset, error)

	// PutAlias writes an alias at the version expected; zero expects a new match.
	PutAlias(ctx context.Context, a cmdb.SoftwareAlias, expected int) error
	Aliases(ctx context.Context) ([]cmdb.SoftwareAlias, error)
	RemoveAlias(ctx context.Context, id shared.ID) error
	// Installations counts the hosts each software CI of a hub is installed on.
	Installations(ctx context.Context, hubID shared.ID) (map[shared.ID]int, error)
}
