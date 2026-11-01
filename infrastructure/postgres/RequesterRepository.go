// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// RequesterRepository is the record of requesters without an account (migration 0283): written by
// the mail intake, removed by REQUESTER's period and by an erasure case by address.
type RequesterRepository struct {
	IDs interface{ NewID() shared.ID }
}

var _ privacyrepo.AddressMail = RequesterRepository{}

func requesterFailed(what string, err error) error {
	return shared.ErrUnavailable.WithDetail("postgres.query_failed").WithCause(fmt.Errorf("%s: %w", what, err))
}

// RecordContact records that the address wrote on the entry at that moment.
func (r RequesterRepository) RecordContact(
	ctx context.Context, address, name string, itemID shared.ID, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return err
	}
	id, err := uuidOf(r.IDs.NewID())
	if err != nil {
		return err
	}
	if err := queries.RecordRequesterContact(ctx, sqlc.RecordRequesterContactParams{
		ID: id, Address: strings.ToLower(strings.TrimSpace(address)), DisplayName: cutRunes(strings.TrimSpace(name), 200),
		ItemID: item, At: timestampOf(at),
	}); err != nil {
		return requesterFailed("recording a requester's contact", err)
	}
	return nil
}

// ContactsSince counts the entries the address was recorded on since the moment.
func (RequesterRepository) ContactsSince(ctx context.Context, address string, since time.Time) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountRequesterContactsSince(ctx, sqlc.CountRequesterContactsSinceParams{
		Address: strings.ToLower(strings.TrimSpace(address)), Since: timestampOf(since),
	})
	if err != nil {
		return 0, requesterFailed("counting a requester's contacts", err)
	}
	return int(n), nil
}

// AddressesOn answers who the entry was filed for by mail, by address (UC-SVC-12).
func (RequesterRepository) AddressesOn(ctx context.Context, itemID shared.ID) ([]servicedesk.RequesterAddress, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	item, err := uuidOf(itemID)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRequesterAddressesOn(ctx, item)
	if err != nil {
		return nil, requesterFailed("reading an entry's requesters", err)
	}
	out := make([]servicedesk.RequesterAddress, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, servicedesk.RequesterAddress{ID: id, Address: row.Address, Name: row.DisplayName})
	}
	return out, nil
}

// DeleteExpired removes a batch of requesters past REQUESTER's period.
func (RequesterRepository) DeleteExpired(ctx context.Context, cutoff time.Time, batch int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredRequesters(ctx, sqlc.DeleteExpiredRequestersParams{Cutoff: timestampOf(cutoff), Batch: boundedInt32(batch)})
	if err != nil {
		return 0, requesterFailed("removing expired requesters", err)
	}
	return int(n), nil
}

// DeleteExpiredAmong removes the requesters past REQUESTER's period among these addresses.
func (RequesterRepository) DeleteExpiredAmong(ctx context.Context, cutoff time.Time, addresses []string) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.DeleteExpiredRequestersAmong(ctx, sqlc.DeleteExpiredRequestersAmongParams{
		Cutoff: timestampOf(cutoff), Addresses: addresses,
	})
	if err != nil {
		return 0, requesterFailed("removing expired requesters", err)
	}
	return int(n), nil
}

// CountExpired counts the requesters past the period, up to ceiling.
func (RequesterRepository) CountExpired(ctx context.Context, cutoff time.Time, ceiling int) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountExpiredRequesters(ctx, sqlc.CountExpiredRequestersParams{Cutoff: timestampOf(cutoff), Ceiling: boundedInt32(ceiling)})
	if err != nil {
		return 0, requesterFailed("counting expired requesters", err)
	}
	return int(n), nil
}

// CountAddress counts what the workspace holds of an address: the requester record, the jumble
// entries and the mail receipts it sent.
func (RequesterRepository) CountAddress(ctx context.Context, address string) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := queries.CountAddressHoldings(ctx, address)
	if err != nil {
		return 0, requesterFailed("counting what an address sent", err)
	}
	return int(n), nil
}

// EraseAddress removes the requester record and the mail receipts of an address, and its jumble
// entries in a full deletion - in the other mode they keep their text without the address.
func (RequesterRepository) EraseAddress(ctx context.Context, address string, full bool) (int, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	requesters, err := queries.DeleteRequesterByAddress(ctx, address)
	if err != nil {
		return 0, requesterFailed("removing a requester", err)
	}
	receipts, err := queries.DeleteMailReceiptsFrom(ctx, address)
	if err != nil {
		return 0, requesterFailed("removing an address's mail receipts", err)
	}
	var intake int64
	if full {
		intake, err = queries.DiscardIntakeFrom(ctx, address)
	} else {
		intake, err = queries.ReleaseIntakeFrom(ctx, address)
	}
	if err != nil {
		return 0, requesterFailed("erasing an address's intake", err)
	}
	return int(requesters + receipts + intake), nil
}

// IntakeLinesFrom answers the entries and comments whose first line names one of the addresses.
func (RequesterRepository) IntakeLinesFrom(ctx context.Context, addresses []string) ([]privacyrepo.IntakeLine, error) {
	if len(addresses) == 0 {
		return nil, nil
	}
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListIntakeLinesFrom(ctx, addresses)
	if err != nil {
		return nil, requesterFailed("reading the intake's lines of an address", err)
	}
	lines := make([]privacyrepo.IntakeLine, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		itemID, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		writtenBy, err := idFrom(row.WrittenBy)
		if err != nil {
			return nil, err
		}
		lines = append(lines, privacyrepo.IntakeLine{
			Comment: row.Kind == "COMMENT", ID: id, ItemID: itemID, WrittenBy: writtenBy, Text: row.Text,
		})
	}
	return lines, nil
}

// MailReceiptsFrom answers the mail intake's log of the address, oldest first.
func (RequesterRepository) MailReceiptsFrom(ctx context.Context, address string) ([]privacyrepo.MailReceipt, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListMailReceiptsFrom(ctx, address)
	if err != nil {
		return nil, requesterFailed("reading the mail receipts of an address", err)
	}
	receipts := make([]privacyrepo.MailReceipt, 0, len(rows))
	for _, row := range rows {
		id, err := idFrom(row.ID)
		if err != nil {
			return nil, err
		}
		itemID, err := optionalID(row.ItemID)
		if err != nil {
			return nil, err
		}
		entryID, err := optionalID(row.EntryID)
		if err != nil {
			return nil, err
		}
		receipts = append(receipts, privacyrepo.MailReceipt{
			ID: id, MessageID: row.MessageID, ReceivedAt: timeFrom(row.ReceivedAt), Sender: row.Sender,
			Subject: row.Subject, Outcome: row.Outcome, Reason: row.Reason, ItemID: itemID, EntryID: entryID,
			Door: row.Door,
		})
	}
	return receipts, nil
}

// RatingLinksTo answers the rating links mailed to the address's requester, without their tokens.
func (RequesterRepository) RatingLinksTo(ctx context.Context, address string) ([]privacyrepo.RatingLink, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := queries.ListRatingLinksTo(ctx, address)
	if err != nil {
		return nil, requesterFailed("reading the rating links of an address", err)
	}
	links := make([]privacyrepo.RatingLink, 0, len(rows))
	for _, row := range rows {
		itemID, err := idFrom(row.ItemID)
		if err != nil {
			return nil, err
		}
		links = append(links, privacyrepo.RatingLink{
			ItemID: itemID, CreatedAt: timeFrom(row.CreatedAt), ExpiresAt: timeFrom(row.ExpiresAt),
			UsedAt: optionalTime(row.UsedAt),
		})
	}
	return links, nil
}

// ExpiredAddresses answers a page of the expired requesters' addresses, oldest first.
func (RequesterRepository) ExpiredAddresses(ctx context.Context, cutoff time.Time, skip, batch int) ([]string, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	addresses, err := queries.ListExpiredRequesterAddresses(ctx, sqlc.ListExpiredRequesterAddressesParams{
		Cutoff: timestampOf(cutoff), Skip: boundedInt32(skip), Batch: boundedInt32(batch),
	})
	if err != nil {
		return nil, requesterFailed("reading the expired requesters' addresses", err)
	}
	return addresses, nil
}

// cutRunes keeps a name within the column's bound rather than refusing the whole contact over it.
func cutRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
