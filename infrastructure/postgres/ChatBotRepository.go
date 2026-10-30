// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	repository "github.com/Jersyfi/hubtask/core/application/repository/chatbot"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ChatBotRepository stores a hub's Matrix bots (migration 0280) through the unit of work's
// transaction (rule 3).
type ChatBotRepository struct{}

// NewChatBotRepository answers the store.
func NewChatBotRepository() ChatBotRepository { return ChatBotRepository{} }

var _ repository.Store = ChatBotRepository{}

func chatBotFailed(what string, err error) error {
	return fmt.Errorf("postgres: chat bot: %s: %w", what, err)
}

func chatBotOf(r sqlc.ListChatBotsRow) integration.ChatBot {
	var k cmdbIDs
	return integration.ChatBot{
		ID: k.of(r.ID), HubID: k.of(r.HubID), CollectionID: k.of(r.CollectionID), Homeserver: r.Homeserver, Room: r.Room,
		BotUser: r.BotUser, Locale: r.Locale, Unfurl: r.Unfurl, Capture: r.Capture, Enabled: r.Enabled, RunAs: k.of(r.RunAs),
		Since: r.Since, Failure: r.Failure, CreatedAt: timeFrom(r.CreatedAt), UpdatedAt: timeFrom(r.UpdatedAt), Version: int(r.Version),
	}
}

func (ChatBotRepository) List(ctx context.Context, hubID shared.ID) ([]integration.ChatBot, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListChatBots(ctx, hub)
	if err != nil {
		return nil, chatBotFailed("listing", err)
	}
	out := make([]integration.ChatBot, 0, len(rows))
	for _, r := range rows {
		out = append(out, chatBotOf(r))
	}
	return out, nil
}

func (ChatBotRepository) Count(ctx context.Context, hubID shared.ID) (int, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return 0, err
	}
	hub, err := uuidOf(hubID)
	if err != nil {
		return 0, err
	}
	n, err := q.CountChatBots(ctx, hub)
	if err != nil {
		return 0, chatBotFailed("counting", err)
	}
	return int(n), nil
}

func (ChatBotRepository) Find(ctx context.Context, id shared.ID) (integration.ChatBot, crypto.Sealed, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return integration.ChatBot{}, crypto.Sealed{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.ChatBot{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("chatbot.unknown")
	}
	r, err := q.FindChatBot(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return integration.ChatBot{}, crypto.Sealed{}, shared.ErrNotFound.WithDetail("chatbot.unknown")
	}
	if err != nil {
		return integration.ChatBot{}, crypto.Sealed{}, chatBotFailed("finding", err)
	}
	bot := chatBotOf(sqlc.ListChatBotsRow{
		ID: r.ID, HubID: r.HubID, CollectionID: r.CollectionID, Homeserver: r.Homeserver, Room: r.Room, BotUser: r.BotUser,
		Locale: r.Locale, Unfurl: r.Unfurl, Capture: r.Capture, Enabled: r.Enabled, RunAs: r.RunAs, Since: r.Since,
		Failure: r.Failure, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, Version: r.Version,
	})
	return bot, crypto.Sealed{KeyID: r.SecretKeyID, Ciphertext: r.SecretEnc}, nil
}

func (ChatBotRepository) Insert(ctx context.Context, b integration.ChatBot, sealed crypto.Sealed) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var ids [4]pgtype.UUID
	for i, id := range []shared.ID{b.ID, b.HubID, b.CollectionID, b.RunAs} {
		if ids[i], err = uuidOf(id); err != nil {
			return err
		}
	}
	err = q.InsertChatBot(ctx, sqlc.InsertChatBotParams{
		ID: ids[0], HubID: ids[1], CollectionID: ids[2], Homeserver: b.Homeserver, Room: b.Room, BotUser: b.BotUser,
		Locale: b.Locale, Unfurl: b.Unfurl, Capture: b.Capture, RunAs: ids[3], SecretEnc: sealed.Ciphertext,
		SecretKeyID: sealed.KeyID, CreatedAt: timestampOf(b.CreatedAt),
	})
	if isUniqueViolation(err) {
		return shared.ErrConflict.WithDetail("chatbot.room_taken")
	}
	if err != nil {
		return chatBotFailed("inserting", err)
	}
	return nil
}

func (ChatBotRepository) Update(ctx context.Context, b integration.ChatBot) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	var ids [3]pgtype.UUID
	for i, id := range []shared.ID{b.ID, b.CollectionID, b.RunAs} {
		if ids[i], err = uuidOf(id); err != nil {
			return err
		}
	}
	if err := q.UpdateChatBot(ctx, sqlc.UpdateChatBotParams{
		ID: ids[0], CollectionID: ids[1], Locale: b.Locale, Unfurl: b.Unfurl, Capture: b.Capture, Enabled: b.Enabled,
		RunAs: ids[2], UpdatedAt: timestampOf(b.UpdatedAt),
	}); err != nil {
		return chatBotFailed("updating", err)
	}
	return nil
}

func (ChatBotRepository) Record(ctx context.Context, b integration.ChatBot) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	id, err := uuidOf(b.ID)
	if err != nil {
		return err
	}
	if err := q.RecordChatBotRun(ctx, sqlc.RecordChatBotRunParams{ID: id, Since: b.Since, Failure: b.Failure, UpdatedAt: timestampOf(b.UpdatedAt)}); err != nil {
		return chatBotFailed("recording", err)
	}
	return nil
}

func (ChatBotRepository) Delete(ctx context.Context, id shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	key, err := uuidOf(id)
	if err != nil {
		return shared.ErrNotFound.WithDetail("chatbot.unknown")
	}
	n, err := q.DeleteChatBot(ctx, key)
	if err != nil {
		return chatBotFailed("deleting", err)
	}
	if n == 0 {
		return shared.ErrNotFound.WithDetail("chatbot.unknown")
	}
	return nil
}
