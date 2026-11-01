// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	repository "github.com/Jersyfi/hubtask/core/application/repository/chatbot"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// ChatLinkRepository stores the chat accounts people linked to a hub's chat bot (migration 0321)
// through the unit of work's transaction (rule 3).
type ChatLinkRepository struct{}

var _ repository.Links = ChatLinkRepository{}

func chatLinkUnknown() error { return shared.ErrNotFound.WithDetail("chatbot.link_unknown") }

func (ChatLinkRepository) Start(ctx context.Context, id, botID, accountID shared.ID, codeHash []byte, expiresAt, now time.Time) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := uuidsOf([]shared.ID{id, botID, accountID})
	if err != nil {
		return err
	}
	if err := q.StartChatLink(ctx, sqlc.StartChatLinkParams{
		ID: ids[0], BotID: ids[1], AccountID: ids[2], CodeHash: codeHash, CodeExpiresAt: timestampOf(expiresAt), CreatedAt: timestampOf(now),
	}); err != nil {
		return chatBotFailed("starting a link", err)
	}
	return nil
}

func (ChatLinkRepository) Claim(ctx context.Context, botID shared.ID, codeHash []byte, chatUser, address string, now time.Time) (shared.ID, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return "", err
	}
	bot, err := uuidOf(botID)
	if err != nil {
		return "", chatLinkUnknown()
	}
	if err := q.ReleaseChatUser(ctx, sqlc.ReleaseChatUserParams{BotID: bot, ChatUser: chatUser}); err != nil {
		return "", chatBotFailed("releasing a chat user", err)
	}
	row, err := q.ClaimChatLink(ctx, sqlc.ClaimChatLinkParams{
		BotID: bot, CodeHash: codeHash, ChatUser: chatUser, Address: address, Now: timestampOf(now),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", chatLinkUnknown()
	}
	if err != nil {
		return "", chatBotFailed("claiming a link", err)
	}
	var k cmdbIDs
	return k.of(row.AccountID), nil
}

func (ChatLinkRepository) ByUser(ctx context.Context, botID shared.ID, chatUser string) (integration.ChatLink, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return integration.ChatLink{}, err
	}
	bot, err := uuidOf(botID)
	if err != nil || chatUser == "" {
		return integration.ChatLink{}, chatLinkUnknown()
	}
	row, err := q.FindChatLinkByUser(ctx, sqlc.FindChatLinkByUserParams{BotID: bot, ChatUser: chatUser})
	if errors.Is(err, pgx.ErrNoRows) {
		return integration.ChatLink{}, chatLinkUnknown()
	}
	if err != nil {
		return integration.ChatLink{}, chatBotFailed("finding a link", err)
	}
	var k cmdbIDs
	return integration.ChatLink{ID: k.of(row.ID), BotID: botID, AccountID: k.of(row.AccountID), AccountName: row.DisplayName, ChatUser: chatUser}, nil
}

func (ChatLinkRepository) OfAccount(ctx context.Context, accountID shared.ID) ([]integration.ChatLink, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return nil, err
	}
	account, err := uuidOf(accountID)
	if err != nil {
		return nil, nil
	}
	rows, err := q.ListChatLinksOfAccount(ctx, account)
	if err != nil {
		return nil, chatBotFailed("listing links", err)
	}
	var k cmdbIDs
	out := make([]integration.ChatLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, integration.ChatLink{ID: k.of(r.ID), BotID: k.of(r.BotID), AccountID: accountID, Address: r.Address})
	}
	return out, nil
}

func (ChatLinkRepository) Find(ctx context.Context, id shared.ID) (integration.ChatLink, error) {
	q, err := queriesFrom(ctx)
	if err != nil {
		return integration.ChatLink{}, err
	}
	key, err := uuidOf(id)
	if err != nil {
		return integration.ChatLink{}, chatLinkUnknown()
	}
	r, err := q.FindChatLink(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return integration.ChatLink{}, chatLinkUnknown()
	}
	if err != nil {
		return integration.ChatLink{}, chatBotFailed("finding a link", err)
	}
	var k cmdbIDs
	return integration.ChatLink{
		ID: k.of(r.ID), BotID: k.of(r.BotID), AccountID: k.of(r.AccountID), AccountName: r.DisplayName, ChatUser: r.ChatUser, Address: r.Address,
	}, nil
}

func (ChatLinkRepository) Delete(ctx context.Context, botID, accountID shared.ID) error {
	q, err := queriesFrom(ctx)
	if err != nil {
		return err
	}
	ids, err := uuidsOf([]shared.ID{botID, accountID})
	if err != nil {
		return chatLinkUnknown()
	}
	n, err := q.DeleteChatLink(ctx, sqlc.DeleteChatLinkParams{BotID: ids[0], AccountID: ids[1]})
	if err != nil {
		return chatBotFailed("removing a link", err)
	}
	if n == 0 {
		return chatLinkUnknown()
	}
	return nil
}
