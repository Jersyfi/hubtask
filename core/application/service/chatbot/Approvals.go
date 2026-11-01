// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"errors"
	"strconv"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The use case names and audit codes of a person's chat link (UC-SVC-09 check 6).
const (
	LinkChatAccountName   = "LinkChatAccount"
	UnlinkChatAccountName = "UnlinkChatAccount"

	LinkStartedAction audit.Action = "chatbot.link_started"
	LinkedAction      audit.Action = "chatbot.linked"
	UnlinkedAction    audit.Action = "chatbot.unlinked"

	listApprovals  = "ListMyApprovals"
	decideApproval = "DecideApproval"
	linkExempt     = "a chat link is a person's own arrangement; what they decide through it is recorded by the approval."
)

// LinkChatAccount answers the caller a one-time code that links the chat account saying it to the
// bot.
type LinkChatAccount struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h LinkChatAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: LinkChatAccountName, Permission: service.PermissionRead,
		Summary: "Answers a one-time code, for fifteen minutes, that links the chat account saying `!link <code>` to the bot " +
			"to the caller: the bot then posts the approvals waiting for their yes to them and takes their `!approve` and " +
			"`!decline` as theirs.",
		SideEffects: "Stores the code's hash, replacing one not yet said, and writes an audit entry.", TokenScope: botsRead,
		Input:    []usecase.Field{botInput()},
		Audit:    usecase.AuditDeclaration{Required: true, Action: LinkStartedAction, TargetType: botTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: linkExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			b := h.Bots
			id, err := in.ID("bot_id")
			if err != nil {
				return nil, err
			}
			bot, _, err := b.find(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := b.authorize(ctx, actor, bot.HubID, false, LinkStartedAction, id); err != nil {
				return nil, err
			}
			if b.Links == nil || b.Entropy == nil || actor.AccountID.IsZero() {
				return nil, shared.ErrUnavailable.WithDetail("chatbot.homeserver_unreachable")
			}
			random, err := b.Entropy.Bytes(integration.ChatLinkCodeBytes)
			if err != nil {
				return nil, err
			}
			code := integration.ChatLinkCode(random)
			now := b.Clock.Now()
			expires := now.Add(integration.ChatLinkLifetime)
			err = b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := b.Links.Start(ctx, b.IDs.NewID(), id, actor.AccountID, integration.ChatLinkCodeHash(code), expires, now); err != nil {
					return err
				}
				return b.record(ctx, actor, LinkStartedAction, id)
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"code": code, "command": integration.LinkCommand + " " + code, "expires_at": expires.UTC()}, nil
		}),
	}
}

// UnlinkChatAccount takes the caller's chat account off the bot.
type UnlinkChatAccount struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h UnlinkChatAccount) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UnlinkChatAccountName, Permission: service.PermissionRead,
		Summary:     "Unlinks the caller's chat account from the bot: it tells them of nothing more and takes nothing said there as theirs.",
		SideEffects: "Removes the link and writes an audit entry.", TokenScope: botsRead,
		Input:    []usecase.Field{botInput()},
		Audit:    usecase.AuditDeclaration{Required: true, Action: UnlinkedAction, TargetType: botTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: linkExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			b := h.Bots
			id, err := in.ID("bot_id")
			if err != nil {
				return nil, err
			}
			bot, _, err := b.find(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := b.authorize(ctx, actor, bot.HubID, false, UnlinkedAction, id); err != nil {
				return nil, err
			}
			if b.Links == nil {
				return nil, shared.ErrNotFound.WithDetail("chatbot.link_unknown")
			}
			return usecase.Output{}, b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := b.Links.Delete(ctx, id, actor.AccountID); err != nil {
					return err
				}
				return b.record(ctx, actor, UnlinkedAction, id)
			})
		}),
	}
}

// linkedBots answers which bots the actor's chat account is linked to.
func (b Bots) linkedBots(ctx context.Context, actor appshared.ActorContext) map[shared.ID]bool {
	out := map[shared.ID]bool{}
	if b.Links == nil || actor.AccountID.IsZero() {
		return out
	}
	links, err := b.Links.OfAccount(ctx, actor.AccountID)
	if err != nil {
		return out
	}
	for _, l := range links {
		out[l.BotID] = true
	}
	return out
}

// person is the actor a linked chat account decides as: the person themselves, reading and
// deciding entries only - what the app's approver does on *Waiting for my yes*.
func person(bot integration.ChatBot, link integration.ChatLink) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: bot.TenantID, AccountID: link.AccountID, AccountName: link.AccountName,
		Scopes: []string{"items:read", "items:write"}, Locale: bot.Locale,
	}
}

// said is what a person said to a bot: who, where a card to them alone goes, and the words.
type said struct {
	sender, address, text string
	bare                  bool
}

// command answers a link, approve or decline command, or false for anything else.
func (b Bots) command(ctx context.Context, bot integration.ChatBot, s said) (roomport.Card, bool) {
	cmd, ok := integration.ParseChatCommand(s.text, s.bare)
	if !ok || b.Links == nil || s.sender == "" {
		return roomport.Card{}, false
	}
	text := func(code string, ref string) roomport.Card {
		return roomport.Card{Headline: b.Texts.Render(bot.Locale, code, map[string]string{"ref": ref})}
	}
	scope := persistence.Scope{TenantID: bot.TenantID}
	if cmd.Verb == integration.LinkCommand {
		if s.address == "" {
			return text("chat.bot_link_private_only", ""), true
		}
		var account shared.ID
		err := b.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			var err error
			if account, err = b.Links.Claim(ctx, bot.ID, integration.ChatLinkCodeHash(cmd.Arg), s.sender, s.address, b.Clock.Now()); err != nil {
				return err
			}
			return b.record(ctx, appshared.ActorContext{Kind: appshared.ActorUser, TenantID: bot.TenantID, AccountID: account}, LinkedAction, bot.ID,
				audit.Change{Field: "platform", Classification: audit.Open, To: string(bot.Platform)})
		})
		if err != nil {
			return text("chat.bot_link_refused", ""), true
		}
		return text("chat.bot_linked", ""), true
	}
	var link integration.ChatLink
	if err := b.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		link, err = b.Links.ByUser(ctx, bot.ID, s.sender)
		return err
	}); err != nil {
		return text("chat.bot_not_linked", ""), true
	}
	who := person(bot, link)
	approval, found := b.waiting(ctx, who, cmd.Arg)
	if !found {
		return text("chat.bot_approval_not_found", cmd.Arg), true
	}
	verdict, done := string(servicedesk.VerdictApprove), "chat.bot_approved"
	if cmd.Verb == integration.DeclineCommand {
		verdict, done = string(servicedesk.VerdictDecline), "chat.bot_declined"
	}
	_, err := b.Invoker.Invoke(ctx, decideApproval, who, usecase.Input{"approval_id": approval.String("id"), "verdict": verdict, "reason": cmd.Reason})
	switch {
	case err == nil:
		return text(done, cmd.Arg), true
	case codeOf(err) == "approvals.reason_required":
		return text("chat.bot_reason_required", cmd.Arg), true
	}
	return text("chat.bot_decision_refused", cmd.Arg), true
}

// waiting answers the approval waiting for the person whose reference is ref; none or two alike
// is none - a reference that could be either is decided for neither.
func (b Bots) waiting(ctx context.Context, who appshared.ActorContext, ref string) (usecase.Output, bool) {
	out, err := b.Invoker.Invoke(ctx, listApprovals, who, usecase.Input{})
	if err != nil {
		return nil, false
	}
	rows, _ := out["data"].([]usecase.Output)
	var match usecase.Output
	for _, row := range rows {
		if integration.ApprovalRef(shared.ID(row.String("id"))) != ref {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = row
	}
	return match, match != nil
}

// ApprovalCards tells the linked people of an approval in chat (UC-SVC-09 check 6): beside the
// mail, every chat account a person a level begins to count - or reminds - linked to a bot gets
// a card, queued in the approval's transaction and posted by its own job.
type ApprovalCards struct {
	Links interface {
		OfAccount(ctx context.Context, accountID shared.ID) ([]integration.ChatLink, error)
	}
	Jobs Enqueuer
}

// ApprovalAsked queues a card for each linked chat account of the people; the person who caused
// it is not told of their own request.
func (c ApprovalCards) ApprovalAsked(ctx context.Context, approval servicedesk.Approval, people []shared.ID, actor shared.ID) error {
	return c.queue(ctx, approval, people, actor, "asked")
}

// ApprovalReminded queues a reminder's card.
func (c ApprovalCards) ApprovalReminded(ctx context.Context, approval servicedesk.Approval, people []shared.ID) error {
	return c.queue(ctx, approval, people, "", "reminded")
}

func (c ApprovalCards) queue(ctx context.Context, approval servicedesk.Approval, people []shared.ID, actor shared.ID, why string) error {
	if c.Links == nil || c.Jobs == nil {
		return nil
	}
	for _, p := range people {
		if p.IsZero() || p == actor {
			continue
		}
		links, err := c.Links.OfAccount(ctx, p)
		if err != nil {
			return err
		}
		for _, l := range links {
			key := approval.ID.String() + ":" + strconv.Itoa(approval.Current) + ":" + why + ":" + l.ID.String()
			if _, err := c.Jobs.Enqueue(ctx, queue.Request{
				Kind: queue.KindChatApprovalCard, TenantID: approval.TenantID, DedupeKey: key,
				Payload: map[string]any{"link_id": l.ID.String(), "approval_id": approval.ID.String(), "txn": key},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// TellApproval posts one approval's card to one linked chat account, if the approval still
// waits for the person: a card says what waits and its reference, the entry's title only where
// the person alone reads it - never in a Matrix room others share. An address the platform
// cannot post to, or credentials it no longer takes, are recorded on the bot and end the job; a
// platform out of reach is retried.
func (b Bots) TellApproval(ctx context.Context, tenantID, linkID, approvalID shared.ID, txn string) error {
	if b.Links == nil {
		return nil
	}
	scope := persistence.Scope{TenantID: tenantID}
	var (
		link   integration.ChatLink
		bot    integration.ChatBot
		sealed crypto.Sealed
	)
	err := b.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		if link, err = b.Links.Find(ctx, linkID); err != nil {
			return err
		}
		bot, sealed, err = b.Store.Find(ctx, link.BotID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bot.Enabled {
		return nil
	}
	bot.TenantID = tenantID
	approval, waits := b.waitingByID(ctx, person(bot, link), approvalID)
	if !waits {
		return nil
	}
	base := ""
	if b.Addresses != nil {
		base, _ = b.Addresses.Of(ctx)
	}
	ref := integration.ApprovalRef(approvalID)
	card := roomport.Card{Headline: b.Texts.Render(bot.Locale, "chat.bot_approval_waiting", map[string]string{"ref": ref}), Title: approval.String("item_title")}
	if base != "" {
		card.Link = strings.TrimSuffix(base, "/") + "/approvals"
	}
	call, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	opened, err := b.Encryptor.Open(call, sealed, tokenPurpose(bot.ID))
	if err != nil {
		return b.told(ctx, bot, "chatbot.token_revoked")
	}
	if bot.Platform == integration.FormatMatrix || bot.Platform == "" {
		// The bot's room is shared: the card names the person and the reference, not the entry.
		card.Headline, card.Title = link.ChatUser+": "+card.Headline, ""
		err = b.Room.Post(call, bot.Homeserver, opened, link.Address, "hubtask-approval-"+txn, notice(card))
	} else {
		adapter := b.Platforms[bot.Platform]
		creds, uerr := unbundle(opened)
		if adapter == nil || uerr != nil {
			return b.told(ctx, bot, "chatbot.token_revoked")
		}
		err = adapter.Tell(call, creds, bot.Homeserver, link.Address, card)
	}
	switch code := codeOf(err); {
	case err == nil:
		return nil
	case code == "chatbot.cannot_tell" || code == "chatbot.token_revoked":
		return b.told(ctx, bot, code)
	}
	return err
}

// waitingByID answers the approval if it waits for the person now.
func (b Bots) waitingByID(ctx context.Context, who appshared.ActorContext, id shared.ID) (usecase.Output, bool) {
	out, err := b.Invoker.Invoke(ctx, listApprovals, who, usecase.Input{})
	if err != nil {
		return nil, false
	}
	rows, _ := out["data"].([]usecase.Output)
	for _, row := range rows {
		if row.String("id") == id.String() {
			return row, true
		}
	}
	return nil, false
}

// told records a card that could not go out on the bot, as a refused delivery is. A Matrix bot's
// own read records its refusals, and a write here could set its read position back.
func (b Bots) told(ctx context.Context, bot integration.ChatBot, failure string) error {
	if failure == bot.Failure || !bot.Platform.PostsToIntake() {
		return nil
	}
	bot.Read("", failure, b.Clock.Now())
	return b.UnitOfWork.Within(ctx, persistence.Scope{TenantID: bot.TenantID}, func(ctx context.Context) error { return b.Store.Record(ctx, bot) })
}
