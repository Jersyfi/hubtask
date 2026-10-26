// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// MailIntakeRepository is a workspace's mail intake (ADR-0094): settings, receipts, and the
// lookups that route a message.
type MailIntakeRepository struct{}

// NewMailIntakeRepository builds the repository.
func NewMailIntakeRepository() MailIntakeRepository { return MailIntakeRepository{} }

var (
	_ repository.Settings = MailIntakeRepository{}
	_ repository.Receipts = MailIntakeRepository{}
	_ repository.Places   = MailIntakeRepository{}
)

// storedRoute is a sender route as the jsonb column holds it.
type storedRoute struct {
	Match        string `json:"match"`
	CollectionID string `json:"collection_id"`
}

func (MailIntakeRepository) Find(ctx context.Context) (domain.Settings, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Settings{}, err
	}
	row, err := queries.FindMailSetting(ctx)
	if IsNoRows(err) {
		return domain.DefaultSettings(), nil
	}
	if err != nil {
		return domain.Settings{}, mailQueryFailed("reading the mail settings", err)
	}
	settings := domain.Settings{
		Enabled: row.Enabled,
		Name:    stringFrom(row.Name),
		Senders: domain.Senders{Level: domain.Level(row.SenderLevel), Allowed: row.Allowed, Blocked: row.Blocked},
	}
	if settings.DefaultCollectionID, err = optionalID(row.DefaultCollectionID); err != nil {
		return domain.Settings{}, err
	}
	if settings.OpenedBy, err = optionalID(row.OpenedBy); err != nil {
		return domain.Settings{}, err
	}
	var routes []storedRoute
	if err := json.Unmarshal([]byte(row.SenderRoutes), &routes); err != nil {
		return domain.Settings{}, shared.ErrInternal.WithDetail("mail.routes_unreadable").WithCause(err)
	}
	for _, route := range routes {
		id, err := shared.ParseID(route.CollectionID)
		if err != nil {
			return domain.Settings{}, shared.ErrInternal.WithDetail("mail.routes_unreadable").WithCause(err)
		}
		settings.SenderRoutes = append(settings.SenderRoutes, domain.SenderRoute{Match: route.Match, CollectionID: id})
	}
	return settings, nil
}

func (MailIntakeRepository) Save(
	ctx context.Context, settings domain.Settings, by shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	if settings.Name != "" {
		taken, err := queries.MailWorkspaceNameTaken(ctx, settings.Name)
		if err != nil {
			return mailQueryFailed("checking the mail name", err)
		}
		if taken {
			return mailNameTaken(settings.Name)
		}
	}
	defaultID, err := optionalUUID(settings.DefaultCollectionID)
	if err != nil {
		return err
	}
	openedBy, err := optionalUUID(settings.OpenedBy)
	if err != nil {
		return err
	}
	byID, err := optionalUUID(by)
	if err != nil {
		return err
	}
	routes := make([]storedRoute, 0, len(settings.SenderRoutes))
	for _, route := range settings.SenderRoutes {
		routes = append(routes, storedRoute{Match: route.Match, CollectionID: route.CollectionID.String()})
	}
	document, err := json.Marshal(routes)
	if err != nil {
		return shared.ErrInternal.WithCause(err)
	}
	err = queries.UpsertMailSetting(ctx, sqlc.UpsertMailSettingParams{
		Enabled: settings.Enabled, Name: optionalText(settings.Name), DefaultCollectionID: defaultID,
		SenderLevel: string(settings.Senders.Level),
		Allowed:     nonNil(settings.Senders.Allowed), Blocked: nonNil(settings.Senders.Blocked),
		SenderRoutes: document, OpenedBy: openedBy, UpdatedAt: timestampOf(at), UpdatedBy: byID,
	})
	if isUniqueViolation(err) {
		// The check above and the write are two statements; the index is the guarantee.
		return mailNameTaken(settings.Name)
	}
	if err != nil {
		return mailQueryFailed("storing the mail settings", err)
	}
	return nil
}

func (MailIntakeRepository) WorkspaceNamed(ctx context.Context, name string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	tenant, err := queries.ResolveMailWorkspace(ctx, name)
	if err != nil {
		return "", mailQueryFailed("resolving the mail name", err)
	}
	if !tenant.Valid {
		return "", shared.ErrNotFound.WithDetail("mail.not_found")
	}
	return idFrom(tenant)
}

func (MailIntakeRepository) Seen(ctx context.Context, messageID string) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	seen, err := queries.MailReceiptExists(ctx, messageID)
	if err != nil {
		return false, mailQueryFailed("reading the mail receipts", err)
	}
	return seen, nil
}

func (MailIntakeRepository) Record(ctx context.Context, receipt domain.Receipt) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}
	id, err := uuidOf(receipt.ID)
	if err != nil {
		return false, err
	}
	item, err := optionalUUID(receipt.ItemID)
	if err != nil {
		return false, err
	}
	entry, err := optionalUUID(receipt.EntryID)
	if err != nil {
		return false, err
	}
	rows, err := queries.InsertMailReceipt(ctx, sqlc.InsertMailReceiptParams{
		ID: id, MessageID: receipt.MessageID, ReceivedAt: timestampOf(receipt.ReceivedAt),
		Sender: receipt.Sender, Subject: receipt.Subject, Outcome: string(receipt.Outcome),
		Reason: string(receipt.Reason), ItemID: item, EntryID: entry, Door: receipt.Door,
	})
	if err != nil {
		return false, mailQueryFailed("recording the mail receipt", err)
	}
	return rows == 1, nil
}

func (MailIntakeRepository) CountFrom(ctx context.Context, sender string, since time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	count, err := queries.CountMailReceiptsFrom(ctx, sqlc.CountMailReceiptsFromParams{
		Sender: sender, Since: timestampOf(since),
	})
	if err != nil {
		return 0, mailQueryFailed("counting a sender's mail", err)
	}
	return int(count), nil
}

func (MailIntakeRepository) Recent(ctx context.Context, limit int) ([]domain.Receipt, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.RecentMailReceipts(ctx, boundedInt32(min(max(limit, 1), 500)))
	if err != nil {
		return nil, mailQueryFailed("reading the mail receipts", err)
	}
	out := make([]domain.Receipt, 0, len(rows))
	for _, row := range rows {
		receipt := domain.Receipt{
			MessageID: row.MessageID, ReceivedAt: timeFrom(row.ReceivedAt), Sender: row.Sender,
			Subject: row.Subject, Outcome: domain.Outcome(row.Outcome), Reason: domain.Reason(row.Reason),
			Door: row.Door,
		}
		if receipt.ID, err = idFrom(row.ID); err != nil {
			return nil, err
		}
		if receipt.ItemID, err = optionalID(row.ItemID); err != nil {
			return nil, err
		}
		if receipt.EntryID, err = optionalID(row.EntryID); err != nil {
			return nil, err
		}
		out = append(out, receipt)
	}
	return out, nil
}

func (MailIntakeRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	removed, err := queries.DeleteExpiredMailReceipts(ctx, sqlc.DeleteExpiredMailReceiptsParams{
		Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch),
	})
	if err != nil {
		return 0, mailQueryFailed("removing expired mail receipts", err)
	}
	return int(removed), nil
}

func (MailIntakeRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	due, err := queries.CountExpiredMailReceipts(ctx, sqlc.CountExpiredMailReceiptsParams{
		Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling),
	})
	if err != nil {
		return 0, mailQueryFailed("counting expired mail receipts", err)
	}
	return int(due), nil
}

func (MailIntakeRepository) CollectionByHandle(ctx context.Context, handle string) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := queries.CollectionByHandle(ctx, &handle)
	if IsNoRows(err) {
		return "", shared.ErrNotFound.WithDetail("mail.place_not_found")
	}
	if err != nil {
		return "", mailQueryFailed("resolving a handle", err)
	}
	return idFrom(id)
}

func (MailIntakeRepository) ItemByKey(ctx context.Context, key domain.Key) (shared.ID, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := queries.ItemByKey(ctx, sqlc.ItemByKeyParams{Handle: &key.Handle, Number: key.Number})
	if IsNoRows(err) {
		return "", shared.ErrNotFound.WithDetail("mail.place_not_found")
	}
	if err != nil {
		return "", mailQueryFailed("resolving a ticket key", err)
	}
	return idFrom(id)
}

func (MailIntakeRepository) KeyOf(ctx context.Context, itemID shared.ID) (domain.Key, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Key{}, err
	}
	id, err := uuidOf(itemID)
	if err != nil {
		return domain.Key{}, err
	}
	row, err := queries.ItemKey(ctx, id)
	if IsNoRows(err) || (err == nil && (row.Handle == nil || row.Number == nil)) {
		return domain.Key{}, shared.ErrNotFound.WithDetail("mail.place_not_found")
	}
	if err != nil {
		return domain.Key{}, mailQueryFailed("reading a ticket key", err)
	}
	return domain.Key{Handle: *row.Handle, Number: *row.Number}, nil
}

func (MailIntakeRepository) SetHandle(ctx context.Context, collectionID shared.ID, handle string) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(collectionID)
	if err != nil {
		return err
	}
	rows, err := queries.SetCollectionHandle(ctx, sqlc.SetCollectionHandleParams{ID: id, Handle: &handle})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("mail.handle_taken").
			WithFields(shared.FieldError{Path: "/handle", Code: "mail.handle_taken"}).
			WithParams(map[string]string{"handle": handle})
	}
	if err != nil {
		return mailQueryFailed("setting a handle", err)
	}
	if rows == 0 {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func mailNameTaken(name string) error {
	return shared.ErrConflict.WithDetail("mail.name_taken").
		WithFields(shared.FieldError{Path: "/name", Code: "mail.name_taken"}).
		WithParams(map[string]string{"name": name})
}

func mailQueryFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
