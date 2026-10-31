// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package chatbot is the use cases of a hub's chat bot: an administrator of the hub seats it in one
// room - a Matrix account read every minute (UC-INT-18), or a Slack, Teams or Mattermost app that
// posts to the bot's intake (UC-INT-19) - and a link to one of the hub's entries is answered with
// the entry's title and status, a command becomes an entry in the hub's collection. It reads and
// writes as whoever set it up, while they may, so a room never learns more than they could show it.
package chatbot

import (
	"context"
	"encoding/json"
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
	// answerLimit keeps an intake's answer inside what the platforms wait for: Slack three
	// seconds for an event's acknowledgement, Teams fifteen.
	answerLimit = 8 * time.Second
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
	// Platforms are the platforms that post to a bot's intake; one missing refuses its bots.
	Platforms  map[integration.DeliveryFormat]roomport.Platform
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
	platform := bot.Platform
	if platform == "" {
		platform = integration.FormatMatrix
	}
	out := usecase.Output{
		"id": bot.ID.String(), "hub_id": bot.HubID.String(), "collection_id": bot.CollectionID.String(),
		"platform": string(platform), "homeserver": bot.Homeserver, "room": bot.Room, "bot_user": bot.BotUser, "unfurl": bot.Unfurl,
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
		Summary: "Seats a chat bot in one room for a hub: a Matrix account by its homeserver, the room's identifier " +
			"and its access token, which joins the room; or a Slack, Teams or Mattermost app by the channel's " +
			"identifier and the app's credentials, which the platform proves on every post to the bot's intake. " +
			"From then on it answers links to the hub's entries with their title and status, and makes commands " +
			"entries in the chosen collection - as the caller, while they may. Credentials are kept sealed and never answered.",
		SideEffects: "Joins a Matrix room or proves the app's credentials with its platform, stores the bot with its sealed " +
			"credentials, queues a Matrix bot's first read and writes an audit entry.",
		TokenScope: botsWrite,
		Input: []usecase.Field{
			hubInput(),
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The hub's collection captured tasks land in."},
			{Name: "platform", Kind: usecase.KindString, Description: "MATRIX (the default), SLACK, TEAMS or MATTERMOST."},
			{Name: "homeserver", Kind: usecase.KindString, Description: "The Matrix homeserver or the Mattermost server, its https address."},
			{Name: "room", Kind: usecase.KindString, Required: true, Description: "The room's identifier: `!…:server`, a Slack or Mattermost channel's, a Teams conversation's."},
			{Name: "access_token", Kind: usecase.KindString, Description: "The Matrix account's access token, or the Slack app's bot token."},
			{Name: "signing_secret", Kind: usecase.KindString, Description: "The Slack app's signing secret."},
			{Name: "app_id", Kind: usecase.KindString, Description: "The Teams bot's application identifier."},
			{Name: "app_tenant", Kind: usecase.KindString, Description: "The Entra tenant the Teams bot is registered in."},
			{Name: "app_password", Kind: usecase.KindString, Description: "The Teams bot's client secret."},
			{Name: "command_token", Kind: usecase.KindString, Description: "The Mattermost slash command's token."},
			{Name: "webhook_token", Kind: usecase.KindString, Description: "The Mattermost outgoing webhook's token."},
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
	platform, err := integration.ParseChatPlatform(in.String("platform"))
	if err != nil {
		return nil, err
	}
	homeserver, room, locale, err := integration.CheckChatBotOn(platform, in.String("homeserver"), in.String("room"), in.String("locale"), in.Bool("unfurl"), in.Bool("capture"))
	if err != nil {
		return nil, err
	}
	token, creds, err := credentialsFrom(platform, in)
	if err != nil {
		return nil, err
	}
	if err := b.collectionOf(ctx, actor, hub, collection); err != nil {
		return nil, err
	}
	call, cancel := context.WithTimeout(ctx, callLimit)
	defer cancel()
	user, err := b.seat(call, platform, homeserver, room, token, creds)
	if err != nil {
		return nil, err
	}
	if platform.PostsToIntake() {
		token = secret.New(bundle(creds))
	}
	now := b.Clock.Now()
	bot := integration.ChatBot{
		ID: b.IDs.NewID(), TenantID: actor.TenantID, HubID: hub, CollectionID: collection, Platform: platform, Homeserver: homeserver,
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
		if b.Jobs != nil && !platform.PostsToIntake() {
			if _, err := b.Jobs.Enqueue(ctx, Job(actor.TenantID, bot.ID, now)); err != nil {
				return err
			}
		}
		return b.record(ctx, actor, CreatedAction, bot.ID,
			audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()},
			audit.Change{Field: "platform", Classification: audit.Open, To: string(platform)},
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
	if bot.Platform == "" {
		bot.Platform = integration.FormatMatrix
	}
	if _, _, bot.Locale, err = integration.CheckChatBotOn(bot.Platform, bot.Homeserver, bot.Room, bot.Locale, bot.Unfurl, bot.Capture); err != nil {
		return nil, err
	}
	now := b.Clock.Now()
	bot.RunAs, bot.UpdatedAt = actor.AccountID, now
	err = b.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := b.Store.Update(ctx, bot); err != nil {
			return err
		}
		if bot.Enabled && !wasOn && b.Jobs != nil && !bot.Platform.PostsToIntake() {
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
	if !bot.Enabled || bot.Platform.PostsToIntake() {
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
		for _, r := range b.answer(ctx, actor, bot, base, m) {
			if err := b.Room.Post(ctx, bot.Homeserver, token, bot.Room, r.txn, notice(r.Card)); err != nil {
				return next, codeOf(err)
			}
		}
	}
	return next, ""
}

type reply struct {
	roomport.Card
	txn string
}

// answer is a Matrix message's replies, each keyed by the message's event so a retried post is
// one notice.
func (b Bots) answer(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, base string, m roomport.Message) []reply {
	if title, ok := integration.Captured(m.Body); ok && bot.Capture {
		return []reply{{Card: b.captured(ctx, actor, bot, base, title), txn: "hubtask-" + m.EventID}}
	}
	out := []reply{}
	if !bot.Unfurl {
		return out
	}
	ids := integration.LinkedEntries(m.Body, base)
	inHub := b.ofHub(ctx, actor, bot, ids)
	for i, id := range ids {
		if card, ok := b.unfurled(ctx, actor, bot, base, id, inHub); ok {
			out = append(out, reply{Card: card, txn: "hubtask-" + m.EventID + "-" + string(rune('a'+i))})
		}
	}
	return out
}

// captured makes the entry a command asks for and answers its card, or the refusal's.
func (b Bots) captured(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, base, title string) roomport.Card {
	created, err := b.Invoker.Invoke(ctx, createEntry, actor, usecase.Input{
		"type": "TASK", "title": title, "collection_id": bot.CollectionID.String(),
	})
	if err != nil {
		return roomport.Card{Headline: b.Texts.Render(bot.Locale, "chat.bot_capture_refused", nil)}
	}
	card := roomport.Card{Headline: b.Texts.Render(bot.Locale, "chat.bot_captured", nil), Title: title}
	if base != "" {
		card.Link = strings.TrimSuffix(base, "/") + "/items/" + created.String("id")
	}
	return card
}

// ofHub answers which of the entries are the bot's hub's.
func (b Bots) ofHub(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, ids []shared.ID) map[shared.ID]bool {
	inHub := map[shared.ID]bool{}
	if len(ids) == 0 {
		return inHub
	}
	var summaries []workrepo.EntrySummary
	if err := b.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = b.Entries.Entries(ctx, ids)
		return err
	}); err != nil {
		return inHub
	}
	for _, s := range summaries {
		inHub[s.ID] = s.HubID == bot.HubID
	}
	return inHub
}

// unfurled answers an entry's card, read as the bot's actor; another hub's entry, or one the
// actor may not read, is not answered - the room is not told it exists.
func (b Bots) unfurled(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, base string, id shared.ID, inHub map[shared.ID]bool) (roomport.Card, bool) {
	if !inHub[id] {
		return roomport.Card{}, false
	}
	entry, err := b.Invoker.Invoke(ctx, getEntry, actor, usecase.Input{"item_id": id.String()})
	if err != nil {
		return roomport.Card{}, false
	}
	headline := b.Texts.Render(bot.Locale, "chat.bot_unfurled", nil)
	if status := entry.String("status"); status != "" {
		headline = b.Texts.Render(bot.Locale, "chat.bot_unfurled_status", map[string]string{"status": status})
	}
	return roomport.Card{Headline: headline, Title: entry.String("title"), Link: strings.TrimSuffix(base, "/") + "/items/" + id.String()}, true
}

// notice is a Matrix card: a sentence, an entry's title and its link at most - never its notes
// (UC-INT-18 check 2, data-protection.md §9).
func notice(card roomport.Card) roomport.Notice {
	headline, title, link := card.Headline, card.Title, card.Link
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

// credentials is the plain form of a platform bot's sealed credentials.
type credentials struct {
	Token         string `json:"token,omitempty"`
	SigningSecret string `json:"signing_secret,omitempty"`
	AppID         string `json:"app_id,omitempty"`
	AppTenant     string `json:"app_tenant,omitempty"`
	AppPassword   string `json:"app_password,omitempty"`
	CommandToken  string `json:"command_token,omitempty"`
	WebhookToken  string `json:"webhook_token,omitempty"`
}

func bundle(c roomport.Credentials) string {
	raw, _ := json.Marshal(credentials{
		Token: c.Token.Reveal(), SigningSecret: c.SigningSecret.Reveal(), AppID: c.AppID, AppTenant: c.AppTenant,
		AppPassword: c.AppPassword.Reveal(), CommandToken: c.CommandToken.Reveal(), WebhookToken: c.WebhookToken.Reveal(),
	})
	return string(raw)
}

func unbundle(s secret.Secret) (roomport.Credentials, error) {
	var c credentials
	if err := json.Unmarshal([]byte(s.Reveal()), &c); err != nil {
		return roomport.Credentials{}, err
	}
	return roomport.Credentials{
		Token: secret.New(c.Token), SigningSecret: secret.New(c.SigningSecret), AppID: c.AppID, AppTenant: c.AppTenant,
		AppPassword: secret.New(c.AppPassword), CommandToken: secret.New(c.CommandToken), WebhookToken: secret.New(c.WebhookToken),
	}, nil
}

// credentialsFrom reads what a platform's bot is seated with: a Matrix account's token alone, or
// the app's credentials - each platform's own, every one of them required for what the bot does.
func credentialsFrom(platform integration.DeliveryFormat, in usecase.Input) (secret.Secret, roomport.Credentials, error) {
	field := func(name string) string { return strings.TrimSpace(in.String(name)) }
	var c roomport.Credentials
	switch platform {
	case integration.FormatMatrix:
		if field("access_token") == "" {
			return secret.Secret{}, c, invalid("/access_token", "chatbot.token_required")
		}
		return secret.New(field("access_token")), c, nil
	case integration.FormatSlack:
		if !strings.HasPrefix(field("access_token"), "xoxb-") {
			return secret.Secret{}, c, invalid("/access_token", "chatbot.token_required")
		}
		if field("signing_secret") == "" {
			return secret.Secret{}, c, invalid("/signing_secret", "chatbot.signing_secret_required")
		}
		c.Token, c.SigningSecret = secret.New(field("access_token")), secret.New(field("signing_secret"))
	case integration.FormatTeams:
		app, tenant := strings.ToLower(field("app_id")), strings.ToLower(field("app_tenant"))
		if _, err := shared.ParseID(app); err != nil {
			return secret.Secret{}, c, invalid("/app_id", "chatbot.app_required")
		}
		if _, err := shared.ParseID(tenant); err != nil {
			return secret.Secret{}, c, invalid("/app_tenant", "chatbot.app_required")
		}
		if field("app_password") == "" {
			return secret.Secret{}, c, invalid("/app_password", "chatbot.app_required")
		}
		c.AppID, c.AppTenant, c.AppPassword = app, tenant, secret.New(field("app_password"))
	case integration.FormatMattermost:
		if in.Bool("unfurl") && field("webhook_token") == "" {
			return secret.Secret{}, c, invalid("/webhook_token", "chatbot.webhook_token_required")
		}
		if in.Bool("capture") && field("command_token") == "" {
			return secret.Secret{}, c, invalid("/command_token", "chatbot.command_token_required")
		}
		c.CommandToken, c.WebhookToken = secret.New(field("command_token")), secret.New(field("webhook_token"))
	}
	return secret.Secret{}, c, nil
}

// seat proves the bot's credentials before it is stored and answers its own identifier.
func (b Bots) seat(ctx context.Context, platform integration.DeliveryFormat, homeserver, room string, token secret.Secret, creds roomport.Credentials) (string, error) {
	if platform == integration.FormatMatrix {
		user, err := b.Room.WhoAmI(ctx, homeserver, token)
		if err != nil {
			return "", err
		}
		return user, b.Room.Join(ctx, homeserver, token, room)
	}
	adapter := b.Platforms[platform]
	if adapter == nil {
		return "", shared.ErrUnavailable.WithDetail("chatbot.homeserver_unreachable")
	}
	return adapter.Check(ctx, creds)
}

// Delivery is one post a platform made to a bot's intake.
type Delivery struct {
	TenantID shared.ID
	BotID    shared.ID
	Header   map[string][]string
	Body     []byte
}

// refused is every reason an intake does not serve: one answer, so the address answers nothing to
// whoever tries bots, secrets or timestamps (T-32).
func refused() error { return shared.ErrNotFound.WithDetail("chatbot.unknown") }

// Receive answers a platform's post to a bot's intake (UC-INT-19): proven by the platform's
// signature or token, it is answered in the platform's format as the bot's actor. A delivery from
// another room than the bot's is acknowledged and not answered; an answer the platform does not
// take is recorded on the bot, and the next one that goes through clears it.
func (b Bots) Receive(ctx context.Context, d Delivery) (roomport.Response, error) {
	scope := persistence.Scope{TenantID: d.TenantID}
	var (
		bot    integration.ChatBot
		sealed crypto.Sealed
	)
	if err := b.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		bot, sealed, err = b.Store.Find(ctx, d.BotID)
		return err
	}); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return roomport.Response{}, refused()
		}
		return roomport.Response{}, err
	}
	adapter := b.Platforms[bot.Platform]
	if !bot.Enabled || !bot.Platform.PostsToIntake() || adapter == nil {
		return roomport.Response{}, refused()
	}
	bot.TenantID = d.TenantID
	opened, err := b.Encryptor.Open(ctx, sealed, tokenPurpose(bot.ID))
	if err != nil {
		return roomport.Response{}, refused()
	}
	creds, err := unbundle(opened)
	if err != nil {
		return roomport.Response{}, refused()
	}
	in, err := adapter.Read(ctx, creds, bot.BotUser, roomport.Delivery{Header: d.Header, Body: d.Body, Now: b.Clock.Now()})
	if err != nil {
		return roomport.Response{}, refused()
	}
	call, cancel := context.WithTimeout(ctx, answerLimit)
	defer cancel()
	switch {
	case in.Asks == roomport.AsksChallenge:
		return adapter.Answer(call, creds, in, nil)
	case in.Asks == roomport.AsksNothing || !bot.SameRoom(in.Room):
		return adapter.Answer(call, creds, roomport.Inbound{}, nil)
	}
	base := ""
	if b.Addresses != nil {
		base, _ = b.Addresses.Of(ctx)
	}
	cards := b.cards(call, runAs(bot), bot, base, in)
	answer, err := adapter.Answer(call, creds, in, cards)
	failure := ""
	if err != nil {
		failure = codeOf(err)
		answer, _ = adapter.Answer(call, creds, roomport.Inbound{}, nil)
	}
	if failure != bot.Failure {
		bot.Read("", failure, b.Clock.Now())
		if err := b.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error { return b.Store.Record(ctx, bot) }); err != nil {
			return roomport.Response{}, err
		}
	}
	return answer, nil
}

// cards answers what a verified delivery asks, as far as the bot does it.
func (b Bots) cards(ctx context.Context, actor appshared.ActorContext, bot integration.ChatBot, base string, in roomport.Inbound) []roomport.Card {
	out := []roomport.Card{}
	switch in.Asks {
	case roomport.AsksCapture:
		if title, ok := integration.Captured(integration.CaptureCommand + " " + in.Text); ok && bot.Capture {
			out = append(out, b.captured(ctx, actor, bot, base, title))
		}
	case roomport.AsksUnfurl:
		if !bot.Unfurl {
			return out
		}
		type wanted struct {
			id   shared.ID
			link string
		}
		asked := []wanted{}
		for _, link := range in.Links {
			if ids := integration.LinkedEntries(link, base); len(ids) > 0 && len(asked) < integration.MaxUnfurlsPerPost {
				asked = append(asked, wanted{id: ids[0], link: link})
			}
		}
		if len(in.Links) == 0 {
			for _, id := range integration.LinkedEntries(in.Text, base) {
				asked = append(asked, wanted{id: id})
			}
		}
		ids := make([]shared.ID, 0, len(asked))
		for _, w := range asked {
			ids = append(ids, w.id)
		}
		inHub := b.ofHub(ctx, actor, bot, ids)
		for _, w := range asked {
			if card, ok := b.unfurled(ctx, actor, bot, base, w.id, inHub); ok {
				card.For = w.link
				out = append(out, card)
			}
		}
	}
	return out
}
