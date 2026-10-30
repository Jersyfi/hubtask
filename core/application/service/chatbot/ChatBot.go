// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the use cases of a hub's Matrix bot (UC-INT-18): an administrator of the hub
// seats a Matrix account in one room, and every minute it reads what the room said - a link to
// one of the hub's entries is answered with the entry's title and status, a message `!task
// <title>` becomes an entry in the hub's collection. It reads and writes as whoever set it up,
// while they may, so a room never learns more than they could show it.
package chatbot

import (
	"context"
	"errors"
	"html"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/chatbot"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The use case names.
const (
	ListChatBotsName  = "ListChatBots"
	CreateChatBotName = "CreateChatBot"
	UpdateChatBotName = "UpdateChatBot"
	DeleteChatBotName = "DeleteChatBot"
)

// The stable audit codes (audit.md §2). What a room says never reaches the trail (rule 10).
const (
	ReadAction    audit.Action = "chatbot.read"
	CreatedAction audit.Action = "chatbot.created"
	UpdatedAction audit.Action = "chatbot.updated"
	DeletedAction audit.Action = "chatbot.deleted"
	botTarget                  = "chat_bot"
	botsRead                   = "containers:read"
	botsWrite                  = "automation:manage"
	exemptReason               = "a chat bot is the hub's configuration; an entry it captures is recorded by the entry's own use case."

	getEntry    = "GetWorkItem"
	createEntry = "CreateWorkItem"
	callLimit   = 20 * time.Second
	runLimit    = time.Minute
)

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Invoker runs another use case by its name, as the bot's actor.
type Invoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Entries reads the summaries of live entries: their hub and collection.
type Entries interface {
	Entries(ctx context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error)
}

// Texts renders the bot's sentences from the message catalogue: a room is read by people, the
// chat deliverer's exception to rule 8.
type Texts interface {
	Render(locale, code string, params map[string]string) string
}

// Addresses answers the workspace's address, for the links the bot reads and answers.
type Addresses interface {
	Of(ctx context.Context) (string, error)
}

// Sealer seals and opens a bot's token under its own purpose.
type Sealer interface {
	Seal(ctx context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error)
	Open(ctx context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error)
}

// Enqueuer queues a bot's run.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Bots is what every chat bot use case shares.
type Bots struct {
	Store      repository.Store
	Containers workrepo.Containers
	Entries    Entries
	Room       roomport.Room
	Invoker    Invoker
	Texts      Texts
	Addresses  Addresses
	Encryptor  Sealer
	Authorizer Authorizer
	Jobs       Enqueuer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Job is a bot's run: one per bot, rescheduling itself every minute.
func Job(tenantID, botID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindChatBot, TenantID: tenantID, DedupeKey: botID.String(), RunAt: at.UTC(),
		Payload: map[string]any{"bot_id": botID.String()},
	}
}

func tokenPurpose(id shared.ID) crypto.Purpose { return crypto.Purpose("chatbot.token:" + id.String()) }

func hubPath(hubID shared.ID) []identity.Scope {
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func (b Bots) authorize(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, write bool, action audit.Action, id shared.ID) error {
	permission, scope := service.PermissionRead, botsRead
	if write {
		permission, scope = service.PermissionStructure, botsWrite
	}
	return b.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: hubPath(hubID), Action: action, TokenScope: scope, TargetType: botTarget, TargetID: id,
	})
}

func (b Bots) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, changes ...audit.Change) error {
	return b.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: b.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: botTarget, TargetID: id, Changes: audit.Changes(changes...),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// collectionOf refuses a collection that is not a living one of the hub.
func (b Bots) collectionOf(ctx context.Context, actor appshared.ActorContext, hubID, collectionID shared.ID) error {
	return b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		hub, err := b.Containers.Find(ctx, hubID)
		if err != nil || hub.Type != work.ContainerHub || hub.DeletedAt != nil {
			return shared.ErrNotFound.WithDetail("chatbot.hub_not_found")
		}
		c, err := b.Containers.Find(ctx, collectionID)
		if err != nil || c.Type != work.ContainerCollection || c.DeletedAt != nil || c.ParentID != hubID {
			return invalid("/collection_id", "chatbot.collection_not_in_hub")
		}
		return nil
	})
}

func (b Bots) find(ctx context.Context, actor appshared.ActorContext, id shared.ID) (integration.ChatBot, crypto.Sealed, error) {
	var (
		bot    integration.ChatBot
		sealed crypto.Sealed
	)
	err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		bot, sealed, err = b.Store.Find(ctx, id)
		return err
	})
	return bot, sealed, err
}

// Output is a bot on the wire: never its token.
func Output(bot integration.ChatBot) usecase.Output {
	out := usecase.Output{
		"id": bot.ID.String(), "hub_id": bot.HubID.String(), "collection_id": bot.CollectionID.String(),
		"homeserver": bot.Homeserver, "room": bot.Room, "bot_user": bot.BotUser, "unfurl": bot.Unfurl,
		"capture": bot.Capture, "enabled": bot.Enabled, "run_as": bot.RunAs.String(),
		"created_at": bot.CreatedAt.UTC(), "updated_at": bot.UpdatedAt.UTC(),
	}
	if bot.Locale != "" {
		out["locale"] = bot.Locale
	}
	if bot.Failure != "" {
		out["failure"] = bot.Failure
	}
	return out
}

func hubInput() usecase.Field {
	return usecase.Field{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}
}

func botInput() usecase.Field {
	return usecase.Field{Name: "bot_id", Kind: usecase.KindID, Required: true, Description: "The bot."}
}

// ListChatBots answers a hub's bots.
type ListChatBots struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h ListChatBots) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListChatBotsName, Permission: service.PermissionRead,
		Summary: "Lists the Matrix bots seated in a hub's rooms: each with its homeserver, room, account, the " +
			"collection it captures into, whether it unfurls links and captures tasks, who it acts as and its " +
			"last refusal - never its token.",
		SideEffects: "None. Reads only.", TokenScope: botsRead, ReadOnly: true,
		Input:    []usecase.Field{hubInput()},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: botTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			if err := h.Bots.authorize(ctx, actor, hub, false, ReadAction, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = h.Bots.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := h.Bots.Store.List(ctx, hub)
				for _, bot := range list {
					rows = append(rows, Output(bot))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// CreateChatBot seats a Matrix account in a room for a hub.
type CreateChatBot struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h CreateChatBot) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateChatBotName, Permission: service.PermissionStructure,
		Summary: "Seats a Matrix account in one room for a hub, by its homeserver, the room's identifier and the " +
			"account's access token: it joins the room and from then on answers links to the hub's entries with " +
			"their title and status, and makes `!task <title>` messages entries in the chosen collection - as the " +
			"caller, while they may. The token is kept sealed and never answered.",
		SideEffects: "Joins the room, stores the bot with its sealed token, queues its first read and writes an audit entry.",
		TokenScope:  botsWrite,
		Input: []usecase.Field{
			hubInput(),
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The hub's collection captured tasks land in."},
			{Name: "homeserver", Kind: usecase.KindString, Required: true, Description: "The account's homeserver, its https address."},
			{Name: "room", Kind: usecase.KindString, Required: true, Description: "The room's identifier, `!…:server`."},
			{Name: "access_token", Kind: usecase.KindString, Required: true, Description: "The bot account's access token."},
			{Name: "locale", Kind: usecase.KindString, Description: "The language the bot answers in; the installation's when empty."},
			{Name: "unfurl", Kind: usecase.KindBool, Description: "Whether links to the hub's entries are answered."},
			{Name: "capture", Kind: usecase.KindBool, Description: "Whether `!task` messages become entries."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: CreatedAction, TargetType: botTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateChatBot) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Bots
	hub, err := in.ID("hub_id")
	if err != nil {
		return nil, err
	}
	if err := b.authorize(ctx, actor, hub, true, CreatedAction, hub); err != nil {
		return nil, err
	}
	collection, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	homeserver, room, locale, err := integration.CheckChatBot(in.String("homeserver"), in.String("room"), in.String("locale"), in.Bool("unfurl"), in.Bool("capture"))
	if err != nil {
		return nil, err
	}
	raw := strings.TrimSpace(in.String("access_token"))
	if raw == "" {
		return nil, invalid("/access_token", "chatbot.token_required")
	}
	if err := b.collectionOf(ctx, actor, hub, collection); err != nil {
		return nil, err
	}
	token := secret.New(raw)
	call, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	user, err := b.Room.WhoAmI(call, homeserver, token)
	if err != nil {
		return nil, err
	}
	if err := b.Room.Join(call, homeserver, token, room); err != nil {
		return nil, err
	}
	now := b.Clock.Now()
	bot := integration.ChatBot{
		ID: b.IDs.NewID(), TenantID: actor.TenantID, HubID: hub, CollectionID: collection, Homeserver: homeserver,
		Room: room, BotUser: user, Locale: locale, Unfurl: in.Bool("unfurl"), Capture: in.Bool("capture"), Enabled: true,
		RunAs: actor.AccountID, CreatedAt: now, UpdatedAt: now, Version: 1,
	}
	sealed, err := b.Encryptor.Seal(ctx, token, tokenPurpose(bot.ID))
	if err != nil {
		return nil, err
	}
	err = b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		n, err := b.Store.Count(ctx, hub)
		if err != nil {
			return err
		}
		if n >= integration.MaxChatBotsPerHub {
			return invalid("/hub_id", "chatbot.too_many")
		}
		if err := b.Store.Insert(ctx, bot, sealed); err != nil {
			return err
		}
		if b.Jobs != nil {
			if _, err := b.Jobs.Enqueue(ctx, Job(actor.TenantID, bot.ID, now)); err != nil {
				return err
			}
		}
		return b.record(ctx, actor, CreatedAction, bot.ID,
			audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()},
			audit.Change{Field: "homeserver", Classification: audit.Open, To: homeserver})
	})
	if err != nil {
		return nil, err
	}
	return Output(bot), nil
}

// UpdateChatBot changes what a bot does; whoever changes it is who it acts as.
type UpdateChatBot struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h UpdateChatBot) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateChatBotName, Permission: service.PermissionStructure,
		Summary: "Changes a hub's Matrix bot: switches it on or off, what it does, the collection it captures into " +
			"and its language. Whoever changes it is who it acts as from then on.",
		SideEffects: "Stores the change, queues a read when switched on and writes an audit entry.", TokenScope: botsWrite,
		Input: []usecase.Field{
			botInput(),
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether the bot reads its room."},
			{Name: "unfurl", Kind: usecase.KindBool, Description: "Whether links to the hub's entries are answered."},
			{Name: "capture", Kind: usecase.KindBool, Description: "Whether `!task` messages become entries."},
			{Name: "collection_id", Kind: usecase.KindID, Description: "The hub's collection captured tasks land in."},
			{Name: "locale", Kind: usecase.KindString, Description: "The language the bot answers in."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: UpdatedAction, TargetType: botTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h UpdateChatBot) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Bots
	id, err := in.ID("bot_id")
	if err != nil {
		return nil, err
	}
	bot, _, err := b.find(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := b.authorize(ctx, actor, bot.HubID, true, UpdatedAction, id); err != nil {
		return nil, err
	}
	wasOn := bot.Enabled
	if in.Present("enabled") {
		bot.Enabled = in.Bool("enabled")
	}
	if in.Present("unfurl") {
		bot.Unfurl = in.Bool("unfurl")
	}
	if in.Present("capture") {
		bot.Capture = in.Bool("capture")
	}
	if in.Present("locale") {
		bot.Locale = in.String("locale")
	}
	if in.Present("collection_id") {
		if bot.CollectionID, err = in.ID("collection_id"); err != nil {
			return nil, err
		}
		if err := b.collectionOf(ctx, actor, bot.HubID, bot.CollectionID); err != nil {
			return nil, err
		}
	}
	if _, _, bot.Locale, err = integration.CheckChatBot(bot.Homeserver, bot.Room, bot.Locale, bot.Unfurl, bot.Capture); err != nil {
		return nil, err
	}
	now := b.Clock.Now()
	bot.RunAs, bot.UpdatedAt = actor.AccountID, now
	err = b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := b.Store.Update(ctx, bot); err != nil {
			return err
		}
		if bot.Enabled && !wasOn && b.Jobs != nil {
			if _, err := b.Jobs.Enqueue(ctx, Job(actor.TenantID, bot.ID, now)); err != nil {
				return err
			}
		}
		return b.record(ctx, actor, UpdatedAction, bot.ID)
	})
	if err != nil {
		return nil, err
	}
	bot.Version++
	return Output(bot), nil
}

// DeleteChatBot takes a bot out of its room's hub.
type DeleteChatBot struct{ Bots Bots }

// Descriptor is the catalogue entry.
func (h DeleteChatBot) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DeleteChatBotName, Permission: service.PermissionStructure,
		Summary: "Removes a hub's Matrix bot: its token is forgotten and the room is read no more. The account " +
			"stays in the room until someone removes it there; the entries it captured stay.",
		SideEffects: "Removes the bot and writes an audit entry.", TokenScope: botsWrite,
		Input:    []usecase.Field{botInput()},
		Audit:    usecase.AuditDeclaration{Required: true, Action: DeletedAction, TargetType: botTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
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
			if err := b.authorize(ctx, actor, bot.HubID, true, DeletedAction, id); err != nil {
				return nil, err
			}
			err = b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := b.Store.Delete(ctx, id); err != nil {
					return err
				}
				return b.record(ctx, actor, DeletedAction, id)
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}

// runAs is the actor a bot reads and writes as: who set it up, through the import door.
func runAs(bot integration.ChatBot) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorAutomation, TenantID: bot.TenantID, AccountID: bot.RunAs,
		Scopes: []string{"items:read", "items:write"}, Channel: string(work.ChannelImport),
	}
}

// Run is one bot's read of its room. It answers when to read next, or the zero time when the bot
// is gone, off or its token refused.
func (b Bots) Run(ctx context.Context, tenantID, botID shared.ID) (time.Time, error) {
	scope := persistence.Scope{TenantID: tenantID}
	var (
		bot    integration.ChatBot
		sealed crypto.Sealed
	)
	err := b.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		bot, sealed, err = b.Store.Find(ctx, botID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if !bot.Enabled {
		return time.Time{}, nil
	}
	bot.TenantID = tenantID
	run, cancel := context.WithTimeout(ctx, runLimit)
	defer cancel()
	since, failure := b.read(run, bot, sealed)
	now := b.Clock.Now()
	bot.Read(since, failure, now)
	if err := b.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error { return b.Store.Record(ctx, bot) }); err != nil {
		return time.Time{}, err
	}
	if failure == "chatbot.token_revoked" {
		return time.Time{}, nil
	}
	return now.Add(integration.ChatBotInterval), nil
}

func codeOf(err error) string {
	var e *shared.Error
	if errors.As(err, &e) && e.DetailCode != "" {
		return e.DetailCode
	}
	return "chatbot.homeserver_unreachable"
}

// read answers the next position and a refusal's code; each message is answered at most once,
// its event identifier the notice's transaction.
func (b Bots) read(ctx context.Context, bot integration.ChatBot, sealed crypto.Sealed) (string, string) {
	token, err := b.Encryptor.Open(ctx, sealed, tokenPurpose(bot.ID))
	if err != nil {
		return "", "chatbot.token_revoked"
	}
	next, messages, err := b.Room.Read(ctx, bot.Homeserver, token, bot.Room, bot.Since)
	if err != nil {
		return "", codeOf(err)
	}
	base := ""
	if b.Addresses != nil {
		base, _ = b.Addresses.Of(ctx)
	}
	actor := runAs(bot)
	for _, m := range messages {
		if m.Sender == bot.BotUser {
			continue
		}
		for _, notice := range b.answer(ctx, actor, bot, base, m) {
			if err := b.Room.Post(ctx, bot.Homeserver, token, bot.Room, notice.txn, notice.Notice); err != nil {
				return next, codeOf(err)
			}
		}
	}
	return next, ""
}

type reply struct {
	roomport.Notice
	txn string
}

func (b Bots) answer(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, base string, m roomport.Message) []reply {
	out := []reply{}
	if title, ok := integration.Captured(m.Body); ok && bot.Capture {
		created, err := b.Invoker.Invoke(ctx, createEntry, actor, usecase.Input{
			"type": "TASK", "title": title, "collection_id": bot.CollectionID.String(),
		})
		headline := b.Texts.Render(bot.Locale, "chat.bot_captured", nil)
		link := ""
		if err != nil {
			headline = b.Texts.Render(bot.Locale, "chat.bot_capture_refused", nil)
			title = ""
		} else if base != "" {
			link = strings.TrimSuffix(base, "/") + "/items/" + created.String("id")
		}
		return append(out, reply{Notice: notice(headline, title, link), txn: "hubtask-" + m.EventID})
	}
	if !bot.Unfurl {
		return out
	}
	ids := integration.LinkedEntries(m.Body, base)
	if len(ids) == 0 {
		return out
	}
	var summaries []workrepo.EntrySummary
	if err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = b.Entries.Entries(ctx, ids)
		return err
	}); err != nil {
		return out
	}
	inHub := map[shared.ID]bool{}
	for _, s := range summaries {
		inHub[s.ID] = s.HubID == bot.HubID
	}
	for i, id := range ids {
		if !inHub[id] {
			// Another hub's entry, or none: this room is not told it exists.
			continue
		}
		entry, err := b.Invoker.Invoke(ctx, getEntry, actor, usecase.Input{"item_id": id.String()})
		if err != nil {
			continue
		}
		headline := b.Texts.Render(bot.Locale, "chat.bot_unfurled", nil)
		if status := entry.String("status"); status != "" {
			headline = b.Texts.Render(bot.Locale, "chat.bot_unfurled_status", map[string]string{"status": status})
		}
		link := strings.TrimSuffix(base, "/") + "/items/" + id.String()
		out = append(out, reply{Notice: notice(headline, entry.String("title"), link), txn: "hubtask-" + m.EventID + "-" + string(rune('a'+i))})
	}
	return out
}

// notice is a sentence, an entry's title and its link at most - never its notes (UC-INT-18
// check 2, data-protection.md §9).
func notice(headline, title, link string) roomport.Notice {
	lines := []string{headline}
	formatted := "<strong>" + html.EscapeString(headline) + "</strong>"
	if title != "" {
		lines = append(lines, title)
	}
	if link != "" {
		lines = append(lines, link)
		label := title
		if label == "" {
			label = link
		}
		formatted += "<br><a href=\"" + html.EscapeString(link) + "\">" + html.EscapeString(label) + "</a>"
	} else if title != "" {
		formatted += "<br>" + html.EscapeString(title)
	}
	return roomport.Notice{Body: strings.Join(lines, "\n"), HTML: formatted}
}
