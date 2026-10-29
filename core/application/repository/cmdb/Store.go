// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package cmdb is the storage port of the assets and configuration module (M6). Every method runs
// inside the caller's transaction, under the tenant it names (rule 3); another workspace's rows
// are not found.
package cmdb

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Reached is a CI an impact walk reached, at its nearest depth.
type Reached struct {
	CI    shared.ID
	Depth int
}

// Store keeps the workspace's CMDB configuration and a hub's CIs. A put with an expected version
// answers shared.ErrConflict when the row changed meanwhile; zero expects a new row.
type Store interface {
	Classes(ctx context.Context) ([]cmdb.Class, error)
	PutClass(ctx context.Context, class cmdb.Class, expected int) error
	RelationTypes(ctx context.Context) ([]cmdb.RelationType, error)
	PutRelationType(ctx context.Context, t cmdb.RelationType, expected int) error
	Sources(ctx context.Context) ([]cmdb.Source, error)
	PutSource(ctx context.Context, s cmdb.Source, expected int) error
	TouchSource(ctx context.Context, key string, at time.Time) error

	InsertCI(ctx context.Context, ci cmdb.CI) error
	UpdateCI(ctx context.Context, ci cmdb.CI, expected int) error
	FindCI(ctx context.Context, id shared.ID, lock bool) (cmdb.CI, error)
	// ListCIs answers a hub's CIs in identifier order after a cursor; classes narrow when given.
	ListCIs(ctx context.Context, hubID shared.ID, classes []string, after shared.ID, limit int) ([]cmdb.CI, error)
	CIsByID(ctx context.Context, ids []shared.ID) ([]cmdb.CI, error)
	CountCIs(ctx context.Context, hubID shared.ID) (int, error)

	// PutIdentifiers replaces a CI's identifiers; a source's own key, once set, stays.
	PutIdentifiers(ctx context.Context, ci cmdb.CI, identifiers map[string]string) error
	// FindIdentifiers answers the index rows of a hub's unmerged CIs carrying any of the pairs.
	FindIdentifiers(ctx context.Context, hubID shared.ID, identifiers map[string]string) ([]cmdb.IdentifierRow, error)
	// MergeInto moves a duplicate's source keys and relations to the survivor and drops what is
	// left of them on the duplicate.
	MergeInto(ctx context.Context, survivor, duplicate shared.ID) error

	// PutRelation writes a relation or sees it again; it answers the stored identifier and
	// whether it is new.
	PutRelation(ctx context.Context, r cmdb.Relation) (shared.ID, bool, error)
	FindRelation(ctx context.Context, id shared.ID) (cmdb.Relation, error)
	DeleteRelation(ctx context.Context, id shared.ID) error
	RelationsOf(ctx context.Context, ciID shared.ID) ([]cmdb.Relation, error)
	HubRelations(ctx context.Context, hubID shared.ID) ([]cmdb.Relation, error)
	Walk(ctx context.Context, ciID shared.ID, downstream bool, sourceSuffers, targetSuffers []string, depth int) ([]Reached, error)

	InsertRun(ctx context.Context, run cmdb.Run) error
	Runs(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Run, error)
	FindRun(ctx context.Context, id shared.ID) (cmdb.Run, error)
	InsertHold(ctx context.Context, hold cmdb.Hold) error
	Holds(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Hold, error)
	FindHold(ctx context.Context, id shared.ID) (cmdb.Hold, error)
	ResolveHold(ctx context.Context, id shared.ID, at time.Time, by shared.ID) error
	// OpenDuplicate opens a duplicate unless one is open for the same CIs; it answers whether.
	OpenDuplicate(ctx context.Context, d cmdb.Duplicate) (bool, error)
	Duplicates(ctx context.Context, hubID shared.ID, limit int) ([]cmdb.Duplicate, error)
	FindDuplicate(ctx context.Context, id shared.ID) (cmdb.Duplicate, error)
	ResolveDuplicate(ctx context.Context, id shared.ID, at time.Time, by, survivor shared.ID) error
}

// Credential is a connector's sealed credential as stored.
type Credential struct {
	ConnectorID shared.ID
	Ciphertext  []byte
	KeyID       string
}

// Connectors keeps a workspace's connectors, their sealed credentials and the alerts they took
// in. Another workspace's rows are not found.
type Connectors interface {
	// ListConnectors answers the connectors of a hub, or of the workspace when hubID is zero.
	ListConnectors(ctx context.Context, hubID shared.ID) ([]cmdb.Connector, error)
	// FindConnector answers one; lock holds its row for the transaction (a write's transaction).
	FindConnector(ctx context.Context, id shared.ID, lock bool) (cmdb.Connector, error)
	CountConnectors(ctx context.Context, hubID shared.ID) (int, error)
	// PutConnector writes a connector at the version expected; zero expects a new key.
	PutConnector(ctx context.Context, c cmdb.Connector, expected int) error
	SetCredential(ctx context.Context, id shared.ID, ciphertext []byte, keyID string, at time.Time) error
	// Credential answers the sealed credential; not found when none was set.
	Credential(ctx context.Context, id shared.ID) (Credential, error)
	// RecordRun writes what a run changed: cursor, schedule, outcome, failures, switch.
	RecordRun(ctx context.Context, c cmdb.Connector) error
	// SealedNotUnder answers the credentials sealed under another key than the current one.
	SealedNotUnder(ctx context.Context, keyID string) ([]Credential, error)
	Reseal(ctx context.Context, c Credential) error

	// FindAlert answers the alert of a connector's key, held for the transaction; not found when
	// there is none.
	FindAlert(ctx context.Context, connectorID shared.ID, key string) (cmdb.Alert, error)
	PutAlert(ctx context.Context, a cmdb.Alert) error
	// ListAlerts answers a hub's alerts, the latest first, narrowed to a status when given.
	ListAlerts(ctx context.Context, hubID shared.ID, status cmdb.AlertStatus, limit int) ([]cmdb.Alert, error)
	// SeeDigest records a signed delivery's digest and answers false when it was seen already;
	// digests older than before are forgotten.
	SeeDigest(ctx context.Context, connectorID shared.ID, digest []byte, at, before time.Time) (bool, error)
}
