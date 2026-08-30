// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// JumbleRepository is the inbox's table (G-10). Like every repository it is called inside a unit
// of work and never opens a transaction of its own; row level security bounds every statement to
// the tenant of the running transaction (ADR-0010).
type JumbleRepository struct {
	cursors security.CursorCodec
}

func NewJumbleRepository(cursors security.CursorCodec) JumbleRepository {
	return JumbleRepository{cursors: cursors}
}

var _ repository.Entries = JumbleRepository{}

// maxJumblePage bounds a listing, api-guidelines.md §4's numbers.
const (
	defaultJumblePage = 50
	maxJumblePage     = 200
)

func (r JumbleRepository) Insert(ctx context.Context, entry domain.Entry) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	id, err := uuidOf(entry.ID)
	if err != nil {
		return err
	}
	attachments, err := uuidsOf(entry.Attachments)
	if err != nil {
		return err
	}
	if attachments == nil {
		// The column is NOT NULL with an empty-array default, and uuidsOf answers nil for an
		// empty set because ListWorkItems reads nil as "no restriction". Here nil would be NULL.
		attachments = []pgtype.UUID{}
	}

	if err := queries.InsertJumbleEntry(ctx, sqlc.InsertJumbleEntryParams{
		ID:          id,
		Channel:     entry.Channel.String(),
		Sender:      optionalText(entry.Sender),
		RawSubject:  optionalText(entry.RawSubject),
		RawBody:     optionalText(entry.RawBody),
		Attachments: attachments,
		Status:      string(entry.Status),
		ReceivedAt:  timestampOf(entry.ReceivedAt),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing jumble entry %s: %w", entry.ID, err))
	}
	return nil
}

func (r JumbleRepository) Find(ctx context.Context, id shared.ID) (domain.Entry, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return domain.Entry{}, err
	}

	key, err := uuidOf(id)
	if err != nil {
		return domain.Entry{}, err
	}

	row, err := queries.FindJumbleEntry(ctx, key)
	if err != nil {
		if IsNoRows(err) {
			return domain.Entry{}, shared.ErrNotFound.
				WithDetail("jumble.entry_not_found").
				WithParams(map[string]string{"entry_id": id.String()})
		}
		return domain.Entry{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading jumble entry %s: %w", id, err))
	}
	return entryFrom(sqlc.ListJumbleEntriesRow(row))
}

func (r JumbleRepository) List(
	ctx context.Context, query repository.Query,
) (repository.Page, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return repository.Page{}, err
	}

	after := pgtype.UUID{}
	if query.Cursor != "" {
		position, err := r.cursors.Decode(query.Cursor)
		if err != nil {
			return repository.Page{}, err
		}
		if after, err = uuidOf(position.ID); err != nil {
			return repository.Page{}, err
		}
	}

	size := query.Size
	if size <= 0 {
		size = defaultJumblePage
	}
	if size > maxJumblePage {
		size = maxJumblePage
	}

	rows, err := queries.ListJumbleEntries(ctx, sqlc.ListJumbleEntriesParams{
		Status:   optionalText(string(query.Status)),
		Channel:  optionalText(string(query.Channel)),
		After:    after,
		PageSize: int32(size + 1),
	})
	if err != nil {
		return repository.Page{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("listing the jumble: %w", err))
	}

	hasMore := len(rows) > size
	if hasMore {
		rows = rows[:size]
	}

	entries := make([]domain.Entry, 0, len(rows))
	for _, row := range rows {
		entry, err := entryFrom(row)
		if err != nil {
			return repository.Page{}, err
		}
		entries = append(entries, entry)
	}

	page := repository.Page{Entries: entries, HasMore: hasMore}
	if hasMore {
		page.NextCursor = r.cursors.Encode(security.Position{ID: entries[len(entries)-1].ID})
	}
	return page, nil
}

func (r JumbleRepository) Settle(ctx context.Context, entry domain.Entry) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	id, err := uuidOf(entry.ID)
	if err != nil {
		return false, err
	}
	target, err := optionalUUID(entry.TargetItemID)
	if err != nil {
		return false, err
	}
	if entry.SettledAt == nil {
		// The aggregate stamps every settlement; a decision with no moment is a programming error
		// rather than a row to write.
		return false, shared.ErrInternal.WithDetail("jumble.entry_incomplete")
	}

	changed, err := queries.SettleJumbleEntry(ctx, sqlc.SettleJumbleEntryParams{
		ID: id, Status: string(entry.Status), TargetItemID: target,
		SettledAt: timestampOf(*entry.SettledAt),
	})
	if err != nil {
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("settling jumble entry %s: %w", entry.ID, err))
	}
	return changed == 1, nil
}

// entryFrom reads a row back, defensively: the row outlives the release that wrote it.
func entryFrom(row sqlc.ListJumbleEntriesRow) (domain.Entry, error) {
	id, err := idFrom(row.ID)
	if err != nil {
		return domain.Entry{}, err
	}
	target, err := optionalID(row.TargetItemID)
	if err != nil {
		return domain.Entry{}, err
	}

	attachments := make([]shared.ID, 0, len(row.Attachments))
	for _, raw := range row.Attachments {
		attachment, err := idFrom(raw)
		if err != nil {
			return domain.Entry{}, err
		}
		attachments = append(attachments, attachment)
	}

	entry := domain.Entry{
		ID:           id,
		Channel:      domain.Channel(row.Channel),
		Sender:       stringFrom(row.Sender),
		RawSubject:   stringFrom(row.RawSubject),
		RawBody:      stringFrom(row.RawBody),
		Attachments:  attachments,
		Status:       domain.Status(row.Status),
		TargetItemID: target,
		ReceivedAt:   timeFrom(row.ReceivedAt),
	}
	if row.ProcessedAt.Valid {
		settled := timeFrom(row.ProcessedAt)
		entry.SettledAt = &settled
	}
	return entry, nil
}

// JumbleIntakeRepository is the tenant's one webhook address (G-10).
//
// The hasher is the intake's own purpose over the installation secret, so a rule's inbound token
// presented here matches nothing and a stored hash can never be replayed as a credential of
// another kind (security.md §5).
type JumbleIntakeRepository struct {
	hasher security.InboundTokenHasher
}

func NewJumbleIntakeRepository(hasher security.InboundTokenHasher) JumbleIntakeRepository {
	return JumbleIntakeRepository{hasher: hasher}
}

var _ repository.Intake = JumbleIntakeRepository{}

func (r JumbleIntakeRepository) SetToken(
	ctx context.Context, token integration.InboundToken, at time.Time,
) error {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return err
	}

	if err := queries.SetJumbleIntakeToken(ctx, sqlc.SetJumbleIntakeTokenParams{
		TokenHash: r.hasher.Hash(token.Secret()),
		RotatedAt: timestampOf(at),
	}); err != nil {
		return shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("storing the intake token: %w", err))
	}
	return nil
}

func (r JumbleIntakeRepository) VerifyToken(
	ctx context.Context, token integration.InboundToken,
) (bool, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return false, err
	}

	_, err = queries.FindJumbleIntakeByToken(ctx, r.hasher.Hash(token.Secret()))
	if err != nil {
		if IsNoRows(err) {
			return false, nil
		}
		return false, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("verifying the intake token: %w", err))
	}
	return true, nil
}

func (r JumbleIntakeRepository) RotatedAt(ctx context.Context) (time.Time, error) {
	queries, err := queriesFrom(ctx)
	if err != nil {
		return time.Time{}, err
	}

	rotated, err := queries.FindJumbleIntake(ctx)
	if err != nil {
		if IsNoRows(err) {
			return time.Time{}, shared.ErrNotFound.WithDetail("jumble.intake_not_minted")
		}
		return time.Time{}, shared.ErrUnavailable.
			WithDetail("postgres.query_failed").
			WithCause(fmt.Errorf("reading the intake: %w", err))
	}
	return timeFrom(rotated), nil
}
