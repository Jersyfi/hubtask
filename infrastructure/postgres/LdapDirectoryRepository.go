// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// LdapDirectoryRepository is a workspace's LDAP directory (UC-ID-31, migration 0285).
type LdapDirectoryRepository struct{}

// NewLdapDirectoryRepository builds the repository.
func NewLdapDirectoryRepository() LdapDirectoryRepository { return LdapDirectoryRepository{} }

var _ repository.LdapDirectories = LdapDirectoryRepository{}

// ldapCounts is a run's counts as the row keeps them.
type ldapCounts struct {
	Seen          int `json:"seen"`
	Provisioned   int `json:"provisioned"`
	Linked        int `json:"linked"`
	Renamed       int `json:"renamed"`
	SwitchedOff   int `json:"switched_off"`
	SwitchedOn    int `json:"switched_on"`
	Kept          int `json:"kept"`
	Skipped       int `json:"skipped"`
	Failed        int `json:"failed"`
	GroupsCreated int `json:"groups_created"`
	GroupsChanged int `json:"groups_changed"`
}

func (LdapDirectoryRepository) Find(ctx context.Context) (domain.LdapDirectory, crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.LdapDirectory{}, crypto.Sealed{}, err
	}
	row, err := queries.FindLdapDirectory(ctx)
	if IsNoRows(err) {
		return domain.LdapDirectory{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	if err != nil {
		return domain.LdapDirectory{}, crypto.Sealed{}, queryFailed("reading the LDAP directory", err)
	}
	runAs, err := idFrom(row.RunAs)
	if err != nil {
		return domain.LdapDirectory{}, crypto.Sealed{}, err
	}
	d := domain.LdapDirectory{
		URL: row.Url, BindDN: row.BindDn, UserBase: row.UserBase, UserFilter: row.UserFilter,
		GroupFilter: row.GroupFilter, IDAttribute: row.IDAttribute, MailAttribute: row.MailAttribute,
		NameAttribute: row.NameAttribute, GroupNameAttribute: row.GroupNameAttribute,
		MemberAttribute: row.MemberAttribute, RunAs: runAs, EveryMinutes: int(row.EveryMinutes),
		Enabled: row.Enabled, NextRunAt: optionalTime(row.NextRunAt), LastRunAt: optionalTime(row.LastRunAt),
		Failures: int(row.Failures), CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}
	if row.GroupBase != nil {
		d.GroupBase = *row.GroupBase
	}
	if row.LastOutcome != nil {
		d.LastOutcome = domain.LdapOutcome(*row.LastOutcome)
	}
	if row.LastError != nil {
		d.LastError = *row.LastError
	}
	if len(row.LastCounts) > 0 {
		var kept ldapCounts
		if err := json.Unmarshal(row.LastCounts, &kept); err == nil {
			counts := domain.LdapCounts(kept)
			d.LastCounts = &counts
		}
	}
	return d, crypto.Sealed{KeyID: row.PasswordKeyID, Ciphertext: row.PasswordEnc}, nil
}

func (LdapDirectoryRepository) Save(ctx context.Context, d domain.LdapDirectory, sealed crypto.Sealed, by shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	runAs, err := uuidOf(d.RunAs)
	if err != nil {
		return err
	}
	byID, err := optionalUUID(by)
	if err != nil {
		return err
	}
	every := int32(d.EveryMinutes) //nolint:gosec // bounded to 15-1440 by NewLdapDirectory
	err = queries.SaveLdapDirectory(ctx, sqlc.SaveLdapDirectoryParams{
		Url: d.URL, BindDn: d.BindDN, PasswordEnc: sealed.Ciphertext, PasswordKeyID: sealed.KeyID,
		UserBase: d.UserBase, UserFilter: d.UserFilter, GroupBase: optionalText(d.GroupBase),
		GroupFilter: d.GroupFilter, IDAttribute: d.IDAttribute, MailAttribute: d.MailAttribute,
		NameAttribute: d.NameAttribute, GroupNameAttribute: d.GroupNameAttribute,
		MemberAttribute: d.MemberAttribute, RunAs: runAs, EveryMinutes: every, Enabled: d.Enabled,
		NextRunAt: optionalTimestamp(d.NextRunAt), Now: timestampOf(d.UpdatedAt), UpdatedBy: byID,
	})
	if err != nil {
		return queryFailed("storing the LDAP directory", err)
	}
	return nil
}

func (LdapDirectoryRepository) RecordRun(ctx context.Context, d domain.LdapDirectory) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var counts []byte
	if d.LastCounts != nil {
		if counts, err = json.Marshal(ldapCounts(*d.LastCounts)); err != nil {
			return shared.ErrInternal.WithDetail("postgres.encode_failed").WithCause(err)
		}
	}
	failures := int32(min(d.Failures, 1<<30)) //nolint:gosec // bounded just above
	var lastRun = timestampOf(d.UpdatedAt)
	if d.LastRunAt != nil {
		lastRun = timestampOf(*d.LastRunAt)
	}
	moved, err := queries.RecordLdapRun(ctx, sqlc.RecordLdapRunParams{
		LastRunAt: lastRun, LastOutcome: optionalText(string(d.LastOutcome)), LastError: optionalText(d.LastError),
		LastCounts: counts, Failures: failures, NextRunAt: optionalTimestamp(d.NextRunAt),
	})
	if err != nil {
		return queryFailed("recording an LDAP run", err)
	}
	if moved == 0 {
		return shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	return nil
}

func (LdapDirectoryRepository) Delete(ctx context.Context) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	removed, err := queries.DeleteLdapDirectory(ctx)
	if err != nil {
		return queryFailed("removing the LDAP directory", err)
	}
	if removed == 0 {
		return shared.ErrNotFound.WithDetail("directory.ldap_not_configured")
	}
	return nil
}

func (LdapDirectoryRepository) Held(ctx context.Context) ([]domain.LdapHeld, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListLdapHeldAccounts(ctx)
	if err != nil {
		return nil, queryFailed("reading whom the LDAP directory holds", err)
	}
	out := make([]domain.LdapHeld, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.LdapHeld{
			AccountID: id, ExternalID: row.ExternalID, Name: row.DisplayName, Status: domain.AccountStatus(row.Status),
		})
	}
	return out, nil
}

func (LdapDirectoryRepository) HeldGroups(ctx context.Context) ([]domain.LdapHeldGroup, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListLdapHeldGroups(ctx)
	if err != nil {
		return nil, queryFailed("reading the groups the LDAP directory holds", err)
	}
	out := make([]domain.LdapHeldGroup, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.LdapHeldGroup{GroupID: id, ExternalID: row.ExternalID, Name: row.Name})
	}
	return out, nil
}
