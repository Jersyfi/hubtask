// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CmdbConnectorRepository stores the connectors, their sealed credentials and their alerts
// (migration 0238) through the unit of work's transaction (rule 3).
type CmdbConnectorRepository struct{}

// NewCmdbConnectorRepository answers the store.
func NewCmdbConnectorRepository() CmdbConnectorRepository { return CmdbConnectorRepository{} }

var _ repository.Connectors = CmdbConnectorRepository{}

func connectorOf(r sqlc.ListCmdbConnectorsRow) (cmdb.Connector, error) {
	settings := map[string]string{}
	if len(r.Settings) > 0 {
		if err := json.Unmarshal(r.Settings, &settings); err != nil {
			return cmdb.Connector{}, cmdbFailed("decoding a connector", err)
		}
	}
	var k cmdbIDs
	c := cmdb.Connector{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Key: r.Key, Kind: cmdb.ConnectorKind(r.Kind), Source: r.SourceKey,
		BaseURL: r.BaseUrl, Settings: settings, AllowedHosts: r.AllowedHosts, Every: int(r.EveryMinutes),
		Enabled: r.Enabled, Collection: k.of(r.CollectionID), RunAs: k.of(r.RunAs), HasCredential: r.HasCredential,
		Cursor: r.Cursor, NextRunAt: optionalTime(r.NextRunAt), LastRunAt: optionalTime(r.LastRunAt),
		LastRun: k.of(r.LastRunID), LastOutcome: cmdb.ConnectorOutcome(r.LastOutcome), LastError: r.LastError,
		Failures: int(r.Failures), CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt),
		Version: int(r.Version),
	}
	return c, k.err
}

// ListConnectors answers a hub's connectors, or the workspace's for a zero hub.
func (CmdbConnectorRepository) ListConnectors(ctx context.Context, hubID shared.ID) ([]cmdb.Connector, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	var k cmdbKeys
	hub := k.opt(hubID)
	if k.err != nil {
		return nil, k.err
	}
	rows, err := q.ListCmdbConnectors(ctx, hub)
	if err != nil {
		return nil, cmdbFailed("listing connectors", err)
	}
	out := make([]cmdb.Connector, 0, len(rows))
	for _, r := range rows {
		c, err := connectorOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// FindConnector answers one connector, its row held when asked.
func (CmdbConnectorRepository) FindConnector(ctx context.Context, id shared.ID, lock bool) (cmdb.Connector, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Connector{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return cmdb.Connector{}, shared.ErrNotFound.WithDetail("cmdb.connector_not_found")
	}
	var row sqlc.ListCmdbConnectorsRow
	if lock {
		r, err := q.LockCmdbConnector(ctx, key)
		if err != nil {
			return cmdb.Connector{}, cmdbRead(err, "cmdb.connector_not_found")
		}
		row = sqlc.ListCmdbConnectorsRow(r)
	} else {
		r, err := q.FindCmdbConnector(ctx, key)
		if err != nil {
			return cmdb.Connector{}, cmdbRead(err, "cmdb.connector_not_found")
		}
		row = sqlc.ListCmdbConnectorsRow(r)
	}
	return connectorOf(row)
}

// CountConnectors answers how many connectors a hub has.
func (CmdbConnectorRepository) CountConnectors(ctx context.Context, hubID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	key, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountCmdbConnectors(ctx, key)
	if err != nil {
		return 0, cmdbFailed("counting connectors", err)
	}
	return int(n), nil
}

// PutConnector writes a connector at the version expected.
func (CmdbConnectorRepository) PutConnector(ctx context.Context, c cmdb.Connector, expected int) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	settings, err := json.Marshal(c.Settings)
	if err != nil {
		return cmdbFailed("encoding a connector", err)
	}
	var k cmdbKeys
	params := sqlc.UpsertCmdbConnectorParams{
		ID: k.of(c.ID), HubID: k.of(c.HubID), Key: c.Key, Kind: string(c.Kind), SourceKey: c.Source,
		BaseUrl: c.BaseURL, Settings: settings, AllowedHosts: append([]string{}, c.AllowedHosts...),
		EveryMinutes: int32(c.Every), Enabled: c.Enabled, CollectionID: k.opt(c.Collection), //nolint:gosec // bounded by the domain
		RunAs: k.of(c.RunAs), NextRunAt: optionalTimestamp(c.NextRunAt), Failures: int32(c.Failures), //nolint:gosec // bounded by the domain
		At: timestampOf(c.UpdatedAt), ExpectedVersion: int32(expected), //nolint:gosec // a row's version
	}
	if k.err != nil {
		return k.err
	}
	rows, err := q.UpsertCmdbConnector(ctx, params)
	if err != nil {
		return cmdbFailed("writing a connector", err)
	}
	if rows == 0 {
		return shared.ErrVersionConflict.WithDetail("cmdb.version_conflict")
	}
	return nil
}

// SetCredential replaces a connector's sealed credential.
func (CmdbConnectorRepository) SetCredential(ctx context.Context, id shared.ID, ciphertext []byte, keyID string, at time.Time) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	rows, err := q.SetCmdbConnectorCredential(ctx, sqlc.SetCmdbConnectorCredentialParams{
		CredentialEnc: ciphertext, CredentialKeyID: &keyID, At: timestampOf(at), ID: key,
	})
	if err != nil {
		return cmdbFailed("sealing a credential", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("cmdb.connector_not_found")
	}
	return nil
}

// Credential answers a connector's sealed credential.
func (CmdbConnectorRepository) Credential(ctx context.Context, id shared.ID) (repository.Credential, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return repository.Credential{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return repository.Credential{}, err
	}
	r, err := q.ReadCmdbConnectorCredential(ctx, key)
	if err != nil {
		return repository.Credential{}, cmdbRead(err, "cmdb.connector_credential_missing")
	}
	out := repository.Credential{ConnectorID: id, Ciphertext: r.CredentialEnc}
	if r.CredentialKeyID != nil {
		out.KeyID = *r.CredentialKeyID
	}
	return out, nil
}

// RecordRun writes what a run changed.
func (CmdbConnectorRepository) RecordRun(ctx context.Context, c cmdb.Connector) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.RecordCmdbConnectorRunParams{
		Cursor: c.Cursor, NextRunAt: optionalTimestamp(c.NextRunAt), LastRunAt: optionalTimestamp(c.LastRunAt),
		LastRunID: k.opt(c.LastRun), LastOutcome: string(c.LastOutcome), LastError: c.LastError,
		Failures: int32(c.Failures), Enabled: c.Enabled, ID: k.of(c.ID), //nolint:gosec // bounded by the domain
	}
	if k.err != nil {
		return k.err
	}
	if err := q.RecordCmdbConnectorRun(ctx, params); err != nil {
		return cmdbFailed("recording a connector's run", err)
	}
	return nil
}

// SealedNotUnder answers the credentials sealed under another key.
func (CmdbConnectorRepository) SealedNotUnder(ctx context.Context, keyID string) ([]repository.Credential, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListSealedCmdbConnectorCredentials(ctx, &keyID)
	if err != nil {
		return nil, cmdbFailed("listing sealed credentials", err)
	}
	out := make([]repository.Credential, 0, len(rows))
	for _, r := range rows {
		var k cmdbIDs
		c := repository.Credential{ConnectorID: k.of(r.ID), Ciphertext: r.CredentialEnc}
		if r.CredentialKeyID != nil {
			c.KeyID = *r.CredentialKeyID
		}
		if k.err != nil {
			return nil, k.err
		}
		out = append(out, c)
	}
	return out, nil
}

// Reseal writes a credential sealed again.
func (CmdbConnectorRepository) Reseal(ctx context.Context, c repository.Credential) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(c.ConnectorID)
	if err != nil {
		return err
	}
	keyID := c.KeyID
	if err := q.ResealCmdbConnectorCredential(ctx, sqlc.ResealCmdbConnectorCredentialParams{
		CredentialEnc: c.Ciphertext, CredentialKeyID: &keyID, ID: key,
	}); err != nil {
		return cmdbFailed("resealing a credential", err)
	}
	return nil
}

func alertOf(r sqlc.ListCmdbAlertsRow) (cmdb.Alert, error) {
	var k cmdbIDs
	a := cmdb.Alert{
		ID: k.of(r.ID), HubID: k.of(r.HubID), Connector: k.of(r.ConnectorID), Key: r.DedupKey,
		Status: cmdb.AlertStatus(r.Status), Severity: r.Severity, Title: r.Title, CI: k.of(r.CiID),
		Entry: k.of(r.EntryID), FirstAt: timeFrom(r.FirstAt), LastAt: timeFrom(r.LastAt),
		ResolvedAt: optionalTime(r.ResolvedAt), Count: int(r.Count),
	}
	return a, k.err
}

// FindAlert answers the alert of a connector's key, held for the transaction.
func (CmdbConnectorRepository) FindAlert(ctx context.Context, connectorID shared.ID, key string) (cmdb.Alert, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return cmdb.Alert{}, err
	}
	id, err := uuidOf(connectorID)
	if err != nil {
		return cmdb.Alert{}, err
	}
	r, err := q.FindCmdbAlert(ctx, sqlc.FindCmdbAlertParams{ConnectorID: id, DedupKey: key})
	if err != nil {
		return cmdb.Alert{}, cmdbRead(err, "cmdb.alert_not_found")
	}
	return alertOf(sqlc.ListCmdbAlertsRow(r))
}

// PutAlert writes an alert as it now stands.
func (CmdbConnectorRepository) PutAlert(ctx context.Context, a cmdb.Alert) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var k cmdbKeys
	params := sqlc.PutCmdbAlertParams{
		ID: k.of(a.ID), HubID: k.of(a.HubID), ConnectorID: k.of(a.Connector), DedupKey: a.Key,
		Status: string(a.Status), Severity: a.Severity, Title: a.Title, CiID: k.opt(a.CI), EntryID: k.opt(a.Entry),
		FirstAt: timestampOf(a.FirstAt), LastAt: timestampOf(a.LastAt), ResolvedAt: optionalTimestamp(a.ResolvedAt),
		Count: int32(a.Count), //nolint:gosec // a count of deliveries
	}
	if k.err != nil {
		return k.err
	}
	if err := q.PutCmdbAlert(ctx, params); err != nil {
		return cmdbFailed("writing an alert", err)
	}
	return nil
}

// ListAlerts answers a hub's alerts, the latest first.
func (CmdbConnectorRepository) ListAlerts(ctx context.Context, hubID shared.ID, status cmdb.AlertStatus, limit int) ([]cmdb.Alert, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	var narrow *string
	if status != "" {
		s := string(status)
		narrow = &s
	}
	rows, err := q.ListCmdbAlerts(ctx, sqlc.ListCmdbAlertsParams{HubID: hub, Status: narrow, MaxRows: int32(limit)}) //nolint:gosec // a page size
	if err != nil {
		return nil, cmdbFailed("listing alerts", err)
	}
	out := make([]cmdb.Alert, 0, len(rows))
	for _, r := range rows {
		a, err := alertOf(r)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

// SeeDigest records a signed delivery's digest and forgets the expired ones.
func (CmdbConnectorRepository) SeeDigest(ctx context.Context, connectorID shared.ID, digest []byte, at, before time.Time) (bool, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(connectorID)
	if err != nil {
		return false, err
	}
	if err := q.ForgetCmdbIntakeDigests(ctx, sqlc.ForgetCmdbIntakeDigestsParams{ConnectorID: id, Before: timestampOf(before)}); err != nil {
		return false, cmdbFailed("forgetting signatures", err)
	}
	rows, err := q.SeeCmdbIntakeDigest(ctx, sqlc.SeeCmdbIntakeDigestParams{ConnectorID: id, Digest: digest, At: timestampOf(at)})
	if err != nil {
		return false, cmdbFailed("remembering a signature", err)
	}
	return rows == 1, nil
}
