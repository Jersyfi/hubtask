// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendarpush"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// CalendarPushRepository stores the calendar pushes (migration 0278) through the unit of work's
// transaction (rule 3).
type CalendarPushRepository struct{}

// NewCalendarPushRepository answers the store.
func NewCalendarPushRepository() CalendarPushRepository { return CalendarPushRepository{} }

var _ repository.Store = CalendarPushRepository{}

func calendarPushFailed(what string, err error) error {
	return fmt.Errorf("postgres: calendar push: %s: %w", what, err)
}

func calendarPushOf(r sqlc.ListCalendarPushesRow) integration.CalendarPush {
	var k cmdbIDs
	return integration.CalendarPush{
		ID: k.of(r.ID), AccountID: k.of(r.AccountID), FeedID: k.of(r.FeedID), Provider: integration.PushProvider(r.Provider),
		Address: r.Address, State: integration.PushState(r.State), Failure: r.Failure, PushedAt: timeFrom(r.PushedAt),
		CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}
}

func (CalendarPushRepository) List(ctx context.Context, accountID shared.ID) ([]integration.CalendarPush, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCalendarPushes(ctx, account)
	if err != nil {
		return nil, calendarPushFailed("listing", err)
	}
	out := make([]integration.CalendarPush, 0, len(rows))
	for _, r := range rows {
		out = append(out, calendarPushOf(r))
	}
	return out, nil
}

func (CalendarPushRepository) Count(ctx context.Context, accountID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountCalendarPushes(ctx, account)
	if err != nil {
		return 0, calendarPushFailed("counting", err)
	}
	return int(n), nil
}

func (CalendarPushRepository) Find(ctx context.Context, id shared.ID) (integration.CalendarPush, crypto.Sealed, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return integration.CalendarPush{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.CalendarPush{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("calendar_push.unknown")
	}
	r, err := q.FindCalendarPush(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return integration.CalendarPush{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("calendar_push.unknown")
	}
	if err != nil {
		return integration.CalendarPush{}, crypto.Sealed{}, calendarPushFailed("finding", err)
	}
	push := calendarPushOf(sqlc.ListCalendarPushesRow{
		ID: r.ID, AccountID: r.AccountID, FeedID: r.FeedID, Provider: r.Provider, Address: r.Address, State: r.State,
		Failure: r.Failure, PushedAt: r.PushedAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	})
	return push, crypto.Sealed{KeyID: r.SecretKeyID, Ciphertext: r.SecretEnc}, nil
}

func (CalendarPushRepository) Connect(ctx context.Context, push integration.CalendarPush, sealed crypto.Sealed) (shared.ID, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	id, err := uuidOf(push.ID)
	if err != nil {
		return "", err
	}
	account, err := uuidOf(push.AccountID)
	if err != nil {
		return "", err
	}
	feed, err := uuidOf(push.FeedID)
	if err != nil {
		return "", err
	}
	if err := q.UpsertCalendarPush(ctx, sqlc.UpsertCalendarPushParams{
		ID: id, AccountID: account, FeedID: feed, Provider: string(push.Provider), Address: push.Address,
		SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, CreatedAt: timestampOf(push.CreatedAt),
	}); err != nil {
		return "", calendarPushFailed("connecting", err)
	}
	stored, err := q.FindCalendarPushByFeed(ctx, sqlc.FindCalendarPushByFeedParams{FeedID: feed, Provider: string(push.Provider)})
	if err != nil {
		return "", calendarPushFailed("reading back", err)
	}
	var k cmdbIDs
	return k.of(stored), nil
}

func (CalendarPushRepository) Record(ctx context.Context, push integration.CalendarPush) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(push.ID)
	if err != nil {
		return err
	}
	if err := q.UpdateCalendarPushRun(ctx, sqlc.UpdateCalendarPushRunParams{
		State: string(push.State), Failure: push.Failure, PushedAt: nullableTime(push.PushedAt),
		UpdatedAt: timestampOf(push.UpdatedAt), ID: id,
	}); err != nil {
		return calendarPushFailed("recording", err)
	}
	return nil
}

func (CalendarPushRepository) Reseal(ctx context.Context, pushID shared.ID, sealed crypto.Sealed) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(pushID)
	if err != nil {
		return err
	}
	if err := q.UpdateCalendarPushSecret(ctx, sqlc.UpdateCalendarPushSecretParams{SecretEnc: sealed.Ciphertext, SecretKeyID: sealed.KeyID, ID: id}); err != nil {
		return calendarPushFailed("resealing", err)
	}
	return nil
}

func (CalendarPushRepository) Disconnect(ctx context.Context, pushID shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(pushID)
	if err != nil {
		return shared.ErrNotFound.WithDetail("calendar_push.unknown")
	}
	n, err := q.DeleteCalendarPush(ctx, id)
	if err != nil {
		return calendarPushFailed("disconnecting", err)
	}
	if n == 0 {
		return shared.ErrNotFound.WithDetail("calendar_push.unknown")
	}
	return nil
}

func (CalendarPushRepository) Events(ctx context.Context, pushID shared.ID) ([]integration.PushedEvent, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuidOf(pushID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListCalendarPushEvents(ctx, id)
	if err != nil {
		return nil, calendarPushFailed("listing events", err)
	}
	var k cmdbIDs
	out := make([]integration.PushedEvent, 0, len(rows))
	for _, r := range rows {
		out = append(out, integration.PushedEvent{EntryID: k.of(r.EntryID), RemoteID: r.RemoteID, Digest: r.Digest})
	}
	return out, nil
}

func (CalendarPushRepository) PutEvent(ctx context.Context, pushID shared.ID, event integration.PushedEvent, at time.Time) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(pushID)
	if err != nil {
		return err
	}
	entry, err := uuidOf(event.EntryID)
	if err != nil {
		return err
	}
	if err := q.UpsertCalendarPushEvent(ctx, sqlc.UpsertCalendarPushEventParams{
		PushID: id, EntryID: entry, RemoteID: event.RemoteID, Digest: event.Digest, PushedAt: timestampOf(at),
	}); err != nil {
		return calendarPushFailed("putting an event", err)
	}
	return nil
}

func (CalendarPushRepository) DropEvent(ctx context.Context, pushID, entryID shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(pushID)
	if err != nil {
		return err
	}
	entry, err := uuidOf(entryID)
	if err != nil {
		return err
	}
	if err := q.DeleteCalendarPushEvent(ctx, sqlc.DeleteCalendarPushEventParams{PushID: id, EntryID: entry}); err != nil {
		return calendarPushFailed("dropping an event", err)
	}
	return nil
}

func (CalendarPushRepository) Await(ctx context.Context, pending repository.Pending, stateHash []byte) error {
	q, err := queriesFrom(ctx)
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
	feed, err := uuidOf(pending.FeedID)
	if err != nil {
		return err
	}
	if err := q.InsertCalendarPushAuthorization(ctx, sqlc.InsertCalendarPushAuthorizationParams{
		ID: id, AccountID: account, FeedID: feed, Provider: string(pending.Provider), StateHash: stateHash,
		VerifierEnc: pending.Verifier.Ciphertext, VerifierKeyID: pending.Verifier.KeyID, ExpiresAt: timestampOf(pending.ExpiresAt),
	}); err != nil {
		return calendarPushFailed("awaiting", err)
	}
	return nil
}

func (CalendarPushRepository) Take(ctx context.Context, stateHash []byte) (repository.Pending, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return repository.Pending{}, err
	}
	r, err := q.TakeCalendarPushAuthorization(ctx, stateHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return repository.Pending{}, shared.ErrNotFound.WithDetail("calendar_push.state_unknown")
	}
	if err != nil {
		return repository.Pending{}, calendarPushFailed("taking", err)
	}
	var k cmdbIDs
	return repository.Pending{
		ID: k.of(r.ID), AccountID: k.of(r.AccountID), FeedID: k.of(r.FeedID), Provider: integration.PushProvider(r.Provider),
		Verifier: crypto.Sealed{KeyID: r.VerifierKeyID, Ciphertext: r.VerifierEnc}, ExpiresAt: timeFrom(r.ExpiresAt),
	}, nil
}

func (CalendarPushRepository) DeleteExpired(ctx context.Context, now time.Time) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	n, err := q.DeleteExpiredCalendarPushAuthorizations(ctx, timestampOf(now))
	if err != nil {
		return 0, calendarPushFailed("forgetting", err)
	}
	return int(n), nil
}
