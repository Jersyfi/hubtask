// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MailboxRepository is a workspace's connected mailboxes (ADR-0094).
type MailboxRepository struct{}

// NewMailboxRepository builds the repository.
func NewMailboxRepository() MailboxRepository { return MailboxRepository{} }

var _ repository.Mailboxes = MailboxRepository{}

func (MailboxRepository) List(ctx context.Context) ([]domain.Mailbox, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListMailboxes(ctx)
	if err != nil {
		return nil, mailQueryFailed("reading the mailboxes", err)
	}
	out := make([]domain.Mailbox, 0, len(rows))
	for _, row := range rows {
		box, err := mailboxFrom(sqlc.FindMailboxRow{
			ID: row.ID, Protocol: row.Protocol, Address: row.Address, Host: row.Host, Port: row.Port,
			SessionUrl: row.SessionUrl, Username: row.Username, Folder: row.Folder, DoneFolder: row.DoneFolder,
			Preset: row.Preset, Enabled: row.Enabled, CheckedAt: row.CheckedAt, Failure: row.Failure,
			FailingSince: row.FailingSince, CreatedAt: row.CreatedAt, Auth: row.Auth,
			OauthProvider: row.OauthProvider, OauthAuthorizeUrl: row.OauthAuthorizeUrl,
			OauthTokenUrl: row.OauthTokenUrl, OauthScopes: row.OauthScopes, OauthClientID: row.OauthClientID,
			OauthInstallation: row.OauthInstallation,
		})
		if err != nil {
			return nil, err
		}
		out = append(out, box)
	}
	return out, nil
}

func (MailboxRepository) Find(ctx context.Context, id shared.ID) (domain.Mailbox, crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	row, err := queries.FindMailbox(ctx, key)
	if IsNoRows(err) {
		return domain.Mailbox{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("mail.mailbox_not_found")
	}
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, mailQueryFailed("reading a mailbox", err)
	}
	box, err := mailboxFrom(row)
	if err != nil {
		return domain.Mailbox{}, crypto.Sealed{}, err
	}
	return box, crypto.Sealed{KeyID: row.SecretKeyID, Ciphertext: row.SecretEnc}, nil
}

func (MailboxRepository) Insert(ctx context.Context, box domain.Mailbox, sealed crypto.Sealed, by shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(box.ID)
	if err != nil {
		return err
	}
	byID, err := optionalUUID(by)
	if err != nil {
		return err
	}
	params := sqlc.InsertMailboxParams{
		ID: id, Protocol: string(box.Protocol), Address: box.Address, Username: box.Username,
		Folder: box.Folder, DoneFolder: box.DoneFolder, Preset: optionalText(box.Preset), Enabled: box.Enabled,
		SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, CreatedAt: timestampOf(box.CreatedAt), CreatedBy: byID,
		Auth: string(box.Auth),
	}
	if params.Auth == "" {
		params.Auth = string(domain.AuthPassword)
	}
	if client := box.OAuth; client != nil {
		params.Auth = string(domain.AuthOAuth)
		params.OauthProvider, params.OauthAuthorizeUrl = &client.Provider, &client.AuthorizeURL
		params.OauthTokenUrl, params.OauthScopes = &client.TokenURL, client.Scopes
		params.OauthClientID, params.OauthInstallation = &client.ClientID, client.Installation
	}
	if box.Protocol == domain.ProtocolIMAP {
		port := int32(box.Port) //nolint:gosec // validated to 1-65535 by NewMailbox
		params.Host, params.Port = &box.Host, &port
	} else {
		params.SessionUrl = &box.SessionURL
	}
	err = queries.InsertMailbox(ctx, params)
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("mail.mailbox_taken").
			WithFields(shared.FieldError{Path: "/address", Code: "mail.mailbox_taken"})
	}
	if err != nil {
		return mailQueryFailed("storing a mailbox", err)
	}
	return nil
}

func (MailboxRepository) RecordCheck(ctx context.Context, box domain.Mailbox) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(box.ID)
	if err != nil {
		return err
	}
	_, err = queries.SetMailboxChecked(ctx, sqlc.SetMailboxCheckedParams{
		ID: id, CheckedAt: optionalTimestamp(box.CheckedAt), Failure: optionalText(box.Failure),
		FailingSince: optionalTimestamp(box.FailingSince),
	})
	if err != nil {
		return mailQueryFailed("recording a mailbox read", err)
	}
	return nil
}

func (MailboxRepository) Reseal(ctx context.Context, id shared.ID, sealed crypto.Sealed) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	rows, err := queries.ResealMailbox(ctx, sqlc.ResealMailboxParams{ID: key, SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID})
	if err != nil {
		return mailQueryFailed("resealing a mailbox", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("mail.mailbox_not_found")
	}
	return nil
}

func (MailboxRepository) Delete(ctx context.Context, id shared.ID) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return err
	}
	rows, err := queries.DeleteMailbox(ctx, key)
	if err != nil {
		return mailQueryFailed("removing a mailbox", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("mail.mailbox_not_found")
	}
	return nil
}

func (MailboxRepository) Count(ctx context.Context) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountMailboxes(ctx)
	if err != nil {
		return 0, mailQueryFailed("counting the mailboxes", err)
	}
	return int(count), nil
}

func mailboxFrom(row sqlc.FindMailboxRow) (domain.Mailbox, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Mailbox{}, err
	}
	box := domain.Mailbox{
		ID: id, Protocol: domain.Protocol(row.Protocol), Address: row.Address, Host: stringFrom(row.Host),
		SessionURL: stringFrom(row.SessionUrl), Username: row.Username, Folder: row.Folder,
		DoneFolder: row.DoneFolder, Preset: stringFrom(row.Preset), Enabled: row.Enabled,
		CheckedAt: optionalTime(row.CheckedAt), Failure: stringFrom(row.Failure),
		FailingSince: optionalTime(row.FailingSince), CreatedAt: timeFrom(row.CreatedAt),
	}
	if row.Port != nil {
		box.Port = int(*row.Port)
	}
	box.Auth = domain.Auth(row.Auth)
	if box.Auth == domain.AuthOAuth {
		box.OAuth = &domain.OAuthClient{
			Provider: stringFrom(row.OauthProvider), AuthorizeURL: stringFrom(row.OauthAuthorizeUrl),
			TokenURL: stringFrom(row.OauthTokenUrl), Scopes: row.OauthScopes,
			ClientID: stringFrom(row.OauthClientID), Installation: row.OauthInstallation,
		}
	}
	return box, nil
}

// MailboxAuthorizationRepository is the OAuth connections waiting for a provider's answer.
type MailboxAuthorizationRepository struct{}

// NewMailboxAuthorizationRepository builds the repository.
func NewMailboxAuthorizationRepository() MailboxAuthorizationRepository {
	return MailboxAuthorizationRepository{}
}

var _ repository.Authorizations = MailboxAuthorizationRepository{}

func (MailboxAuthorizationRepository) Insert(ctx context.Context, pending domain.PendingAuthorization, stateHash []byte, verifier crypto.Sealed, clientSecret *crypto.Sealed) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(pending.ID)
	if err != nil {
		return err
	}
	account, err := uuidOf(pending.AccountID)
	if err != nil {
		return err
	}
	params := sqlc.InsertMailboxAuthorizationParams{
		ID: id, AccountID: account, StateHash: stateHash, Provider: pending.Client.Provider,
		AuthorizeUrl: pending.Client.AuthorizeURL, TokenUrl: pending.Client.TokenURL, Scopes: pending.Client.Scopes,
		ClientID: pending.Client.ClientID, Installation: pending.Client.Installation, Host: pending.Host,
		Port:      int32(pending.Port), //nolint:gosec // validated to 1-65535 by NewOAuthClient
		LoginHint: optionalText(pending.LoginHint), VerifierEnc: verifier.Ciphertext, VerifierKeyID: verifier.KeyID,
		ExpiresAt: timestampOf(pending.ExpiresAt),
	}
	if clientSecret != nil {
		params.ClientSecretEnc, params.ClientSecretKeyID = clientSecret.Ciphertext, &clientSecret.KeyID
	}
	if err := queries.InsertMailboxAuthorization(ctx, params); err != nil {
		return mailQueryFailed("storing a mailbox authorization", err)
	}
	return nil
}

func (MailboxAuthorizationRepository) Take(ctx context.Context, stateHash []byte) (domain.PendingAuthorization, crypto.Sealed, *crypto.Sealed, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, err
	}
	row, err := queries.TakeMailboxAuthorization(ctx, stateHash)
	if IsNoRows(err) {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, shared.ErrNotFound.WithDetail("mail.oauth_state_unknown")
	}
	if err != nil {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, mailQueryFailed("taking a mailbox authorization", err)
	}
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, err
	}
	account, err := idFrom(row.AccountID)
	if err != nil {
		return domain.PendingAuthorization{}, crypto.Sealed{}, nil, err
	}
	pending := domain.PendingAuthorization{
		ID: id, AccountID: account, Host: row.Host, Port: int(row.Port), LoginHint: stringFrom(row.LoginHint),
		ExpiresAt: timeFrom(row.ExpiresAt),
		Client: domain.OAuthClient{
			Provider: row.Provider, AuthorizeURL: row.AuthorizeUrl, TokenURL: row.TokenUrl, Scopes: row.Scopes,
			ClientID: row.ClientID, Installation: row.Installation,
		},
	}
	var clientSecret *crypto.Sealed
	if row.ClientSecretEnc != nil && row.ClientSecretKeyID != nil {
		clientSecret = &crypto.Sealed{KeyID: *row.ClientSecretKeyID, Ciphertext: row.ClientSecretEnc}
	}
	return pending, crypto.Sealed{KeyID: row.VerifierKeyID, Ciphertext: row.VerifierEnc}, clientSecret, nil
}

func (MailboxAuthorizationRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	rows, err := queries.DeleteExpiredMailboxAuthorizations(ctx, timestampOf(now))
	if err != nil {
		return 0, mailQueryFailed("forgetting expired mailbox authorizations", err)
	}
	return int(rows), nil
}
