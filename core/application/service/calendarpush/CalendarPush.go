// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendarpush is the use cases of a calendar push (UC-INT-16): a person writes their own
// calendar feed into their own calendar at Microsoft 365 or Google, signing in there with the
// installation's app and a consent that asks for the calendar's events alone, and takes it off
// again. A run every quarter hour reads the feed as its owner, evaluated now, and writes what
// changed.
//
// Everything here is the owner's: nobody lists, connects or disconnects somebody else's push, an
// administrator included, because a push is a credential to that person's own calendar.
package calendarpush

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendarpush"
	feedrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	calendarport "github.com/Jersyfi/hubtask/core/port/calendarpush"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The use case names.
const (
	ListCalendarPushesName                = "ListCalendarPushes"
	AuthorizeCalendarPushName             = "AuthorizeCalendarPush"
	CompleteCalendarPushAuthorizationName = "CompleteCalendarPushAuthorization"
	DisconnectCalendarPushName            = "DisconnectCalendarPush"
)

// The stable audit codes (audit.md §2). An event's title never reaches the trail (rule 10).
const (
	ListedAction       audit.Action = "calendar_push.listed"
	StartedAction      audit.Action = "calendar_push.authorization_started"
	ConnectedAction    audit.Action = "calendar_push.connected"
	DisconnectedAction audit.Action = "calendar_push.disconnected"
	pushTarget                      = "calendar_push"
	viewsRead                       = "views:read"
	exemptReason                    = "a calendar push writes a feed into its owner's own calendar; it changes no entry."

	stateBytes    = 32
	verifierBytes = 32
	pendingTTL    = 10 * time.Minute
	exchangeLimit = 20 * time.Second
	runLimit      = 2 * time.Minute
)

// App is an OAuth client the installation registered at a provider, the mailboxes' own, so that
// connecting is a button.
type App struct {
	ClientID     string
	ClientSecret secret.Secret
}

// Sealer seals and opens a grant under its push's purpose: the slice of the encryptor a push uses.
type Sealer interface {
	Seal(ctx context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error)
	Open(ctx context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error)
}

// Entropy draws unguessable bytes.
type Entropy interface {
	Bytes(n int) ([]byte, error)
}

// FeedReader answers a feed's rows as its owner reads them now; not found when the feed, its
// view or its owner's sight of it is gone.
type FeedReader interface {
	ExecuteFeed(ctx context.Context, tenantID, feedID shared.ID) (FeedRows, error)
}

// FeedRows is what a feed serves now.
type FeedRows struct {
	Items    []work.WorkItem
	TimeZone string
}

// Addresses answers the workspace's address, for the link back into the product.
type Addresses interface {
	Of(ctx context.Context) (string, error)
}

// Enqueuer queues a push's run.
type Enqueuer interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Pushes is what every calendar push use case shares.
type Pushes struct {
	Store      repository.Store
	Feeds      feedrepo.CalendarFeeds
	Rows       FeedReader
	Calendar   calendarport.Calendar
	Authorizer mailboxport.Authorizer
	Encryptor  Sealer
	Entropy    Entropy
	// Apps are the installation's apps by provider key; a provider without one is not offered.
	Apps map[string]App
	// RedirectURL is the web UI's page the provider sends the browser back to.
	RedirectURL string
	Addresses   Addresses
	Jobs        Enqueuer
	Audit       audit.Sink
	UnitOfWork  persistence.UnitOfWork
	Clock       clock.Clock
	IDs         clock.IDGenerator
}

// Job is a push's run: one per push, rescheduling itself.
func Job(tenantID, pushID shared.ID, at time.Time) queue.Request {
	return queue.Request{
		Kind: queue.KindCalendarPush, TenantID: tenantID, DedupeKey: pushID.String(), RunAt: at.UTC(),
		Payload: map[string]any{"push_id": pushID.String()},
	}
}

func secretPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("calendar_push.refresh:" + id.String())
}

func verifierPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("calendar_push.verifier:" + id.String())
}

func stateHash(state string) []byte {
	sum := sha256.Sum256([]byte(state))
	return sum[:]
}

// client is the installation's app at the provider, asking for the push's scopes alone.
func (p Pushes) client(provider integration.PushProvider) (maildomain.OAuthClient, secret.Secret, error) {
	app, ok := p.Apps[provider.OAuthKey()]
	preset, known := maildomain.OAuthPresetNamed(provider.OAuthKey())
	if !ok || !known || app.ClientID == "" || p.RedirectURL == "" {
		return maildomain.OAuthClient{}, secret.Secret{}, shared.ErrUnavailable.WithDetail("calendar_push.provider_not_offered")
	}
	return maildomain.OAuthClient{
		Provider: provider.OAuthKey(), AuthorizeURL: preset.AuthorizeURL, TokenURL: preset.TokenURL,
		Scopes: provider.PushScopes(), ClientID: app.ClientID, Installation: true,
	}, app.ClientSecret, nil
}

// Offered answers the providers this installation can push to.
func (p Pushes) Offered() []string {
	out := []string{}
	for _, provider := range []integration.PushProvider{integration.PushMicrosoft, integration.PushGoogle} {
		if _, _, err := p.client(provider); err == nil {
			out = append(out, string(provider))
		}
	}
	return out
}

func (p Pushes) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, changes ...audit.Change) {
	if p.Audit == nil {
		return
	}
	_ = p.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: p.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: pushTarget, TargetID: id, Changes: audit.Changes(changes...),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// Output is a push on the wire: never its grant.
func Output(push integration.CalendarPush) usecase.Output {
	out := usecase.Output{
		"id": push.ID.String(), "feed_id": push.FeedID.String(), "provider": string(push.Provider),
		"address": push.Address, "state": string(push.State), "created_at": push.CreatedAt.UTC(),
	}
	if push.Failure != "" {
		out["failure"] = push.Failure
	}
	if !push.PushedAt.IsZero() {
		out["pushed_at"] = push.PushedAt.UTC()
	}
	return out
}

// ListCalendarPushes answers the caller's own pushes.
type ListCalendarPushes struct{ Pushes Pushes }

// Descriptor is the catalogue entry.
func (h ListCalendarPushes) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListCalendarPushesName,
		Summary: "Lists the caller's own calendar pushes - each calendar feed written into their calendar at " +
			"Microsoft 365 or Google - with the account signed in with, whether it goes through and when it last " +
			"ran, and the providers this installation offers. Never anybody else's.",
		SideEffects: "None. Reads only.", TokenScope: viewsRead, ReadOnly: true,
		Audit:    usecase.AuditDeclaration{Action: ListedAction, TargetType: pushTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := actor.RequireScope(viewsRead); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err := h.Pushes.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				pushes, err := h.Pushes.Store.List(ctx, actor.AccountID)
				for _, push := range pushes {
					rows = append(rows, Output(push))
				}
				return err
			})
			return usecase.Output{"data": rows, "providers": h.Pushes.Offered()}, err
		}),
	}
}

// AuthorizeCalendarPush starts connecting one of the caller's calendar feeds to a provider.
type AuthorizeCalendarPush struct{ Pushes Pushes }

// Descriptor is the catalogue entry.
func (h AuthorizeCalendarPush) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AuthorizeCalendarPushName,
		Summary: "Starts writing one of the caller's own calendar feeds into their calendar at Microsoft 365 or " +
			"Google: answers the provider's page where they sign in and consent to the calendar's events alone. " +
			"The connection waits ten minutes for the answer.",
		SideEffects: "Remembers the waiting connection and writes an audit entry.", TokenScope: viewsRead,
		Input: []usecase.Field{
			{Name: "feed_id", Kind: usecase.KindID, Required: true, Description: "The caller's own calendar feed."},
			{Name: "provider", Kind: usecase.KindString, Required: true, Enum: []string{string(integration.PushMicrosoft), string(integration.PushGoogle)}, Description: "The calendar to write into."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: StartedAction, TargetType: pushTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h AuthorizeCalendarPush) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	p := h.Pushes
	if err := actor.RequireScope(viewsRead); err != nil {
		return nil, err
	}
	feedID, err := in.ID("feed_id")
	if err != nil {
		return nil, err
	}
	provider, err := integration.ParsePushProvider(in.String("provider"))
	if err != nil {
		return nil, err
	}
	client, _, err := p.client(provider)
	if err != nil {
		return nil, err
	}
	now := p.Clock.Now()
	state, err := p.draw(stateBytes)
	if err != nil {
		return nil, err
	}
	verifier, err := p.draw(verifierBytes)
	if err != nil {
		return nil, err
	}
	pending := repository.Pending{ID: p.IDs.NewID(), AccountID: actor.AccountID, FeedID: feedID, Provider: provider, ExpiresAt: now.Add(pendingTTL)}
	if pending.Verifier, err = p.Encryptor.Seal(ctx, secret.New(verifier), verifierPurpose(pending.ID)); err != nil {
		return nil, err
	}
	err = p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		feed, err := p.Feeds.Find(ctx, feedID)
		if err != nil {
			return shared.ErrNotFound.WithDetail("calendar_push.feed_unknown")
		}
		if _, err := integration.NewCalendarPush(pending.ID, actor.TenantID, actor.AccountID, feed, provider, "", now); err != nil {
			return err
		}
		if _, err := p.Store.DeleteExpired(ctx, now); err != nil {
			return err
		}
		count, err := p.Store.Count(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if count >= integration.MaxCalendarPushes {
			return shared.ErrConflict.WithDetail("calendar_push.pushes_full")
		}
		return p.Store.Await(ctx, pending, stateHash(state))
	})
	if err != nil {
		return nil, err
	}
	p.record(ctx, actor, StartedAction, pending.ID, audit.Change{Field: "provider", Classification: audit.Open, To: string(provider)})
	return usecase.Output{
		"authorize_url": client.AuthorizationURL(p.RedirectURL, state, verifier, ""),
		"expires_at":    pending.ExpiresAt.UTC(),
	}, nil
}

func (p Pushes) draw(n int) (string, error) {
	drawn, err := p.Entropy.Bytes(n)
	if err != nil {
		return "", shared.ErrInternal.WithDetail("calendar_push.unavailable").WithCause(err)
	}
	return base64.RawURLEncoding.EncodeToString(drawn), nil
}

// CompleteCalendarPushAuthorization finishes connecting with what the provider sent back.
type CompleteCalendarPushAuthorization struct{ Pushes Pushes }

// Descriptor is the catalogue entry.
func (h CompleteCalendarPushAuthorization) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompleteCalendarPushAuthorizationName,
		Summary: "Finishes connecting a calendar push with what the provider sent the browser back with: trades " +
			"the code, keeps the grant sealed, and starts writing the feed into the calendar within minutes. " +
			"Connecting the same feed to the same provider again replaces the grant.",
		SideEffects: "Stores the push with its sealed grant, queues its first run and writes an audit entry.",
		TokenScope:  viewsRead,
		Input: []usecase.Field{
			{Name: "state", Kind: usecase.KindString, Required: true, Description: "The state the provider sent back."},
			{Name: "code", Kind: usecase.KindString, Description: "The authorization code the provider sent back."},
			{Name: "error", Kind: usecase.KindString, Description: "The provider's refusal, when the person said no."},
		},
		Audit:    usecase.AuditDeclaration{Required: true, Action: ConnectedAction, TargetType: pushTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h CompleteCalendarPushAuthorization) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	p := h.Pushes
	if err := actor.RequireScope(viewsRead); err != nil {
		return nil, err
	}
	state := in.String("state")
	if state == "" {
		return nil, shared.ErrValidation.WithDetail("calendar_push.state_unknown")
	}
	var pending repository.Pending
	// Taken - and so gone - first: a state is good for one try, whatever it brings.
	if err := p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		pending, err = p.Store.Take(ctx, stateHash(state))
		return err
	}); err != nil {
		return nil, err
	}
	now := p.Clock.Now()
	if pending.AccountID != actor.AccountID {
		return nil, shared.ErrNotFound.WithDetail("calendar_push.state_unknown")
	}
	if !now.Before(pending.ExpiresAt) {
		return nil, shared.ErrValidation.WithDetail("calendar_push.expired")
	}
	code := in.String("code")
	if in.String("error") != "" || code == "" {
		return nil, shared.ErrValidation.WithDetail("calendar_push.denied")
	}
	client, clientSecret, err := p.client(pending.Provider)
	if err != nil {
		return nil, err
	}
	verifier, err := p.Encryptor.Open(ctx, pending.Verifier, verifierPurpose(pending.ID))
	if err != nil {
		return nil, err
	}
	exchange, cancel := context.WithTimeout(ctx, exchangeLimit)
	defer cancel()
	token, err := p.Authorizer.Exchange(exchange, client, clientSecret, code, verifier.Reveal(), p.RedirectURL)
	if err != nil {
		return nil, shared.ErrValidation.WithDetail("calendar_push.denied").WithCause(err)
	}
	if token.Refresh.IsEmpty() {
		return nil, shared.ErrValidation.WithDetail("calendar_push.no_refresh")
	}
	var push integration.CalendarPush
	err = p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		feed, err := p.Feeds.Find(ctx, pending.FeedID)
		if err != nil {
			return shared.ErrNotFound.WithDetail("calendar_push.feed_unknown")
		}
		push, err = integration.NewCalendarPush(p.IDs.NewID(), actor.TenantID, actor.AccountID, feed, pending.Provider, token.Email, now)
		if err != nil {
			return err
		}
		sealed, err := p.Encryptor.Seal(ctx, token.Refresh, secretPurpose(push.ID))
		if err != nil {
			return err
		}
		stored, err := p.Store.Connect(ctx, push, sealed)
		if err != nil {
			return err
		}
		if stored != push.ID {
			// The feed was connected to this provider before: its push keeps its identity and
			// takes the new grant, sealed under that identity's purpose.
			push.ID = stored
			resealed, err := p.Encryptor.Seal(ctx, token.Refresh, secretPurpose(stored))
			if err != nil {
				return err
			}
			if err := p.Store.Reseal(ctx, stored, resealed); err != nil {
				return err
			}
		}
		if p.Jobs != nil {
			_, err = p.Jobs.Enqueue(ctx, Job(actor.TenantID, push.ID, now))
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	p.record(ctx, actor, ConnectedAction, push.ID, audit.Change{Field: "provider", Classification: audit.Open, To: string(push.Provider)})
	return Output(push), nil
}

// DisconnectCalendarPush takes a push off: nothing more is written.
type DisconnectCalendarPush struct{ Pushes Pushes }

// Descriptor is the catalogue entry.
func (h DisconnectCalendarPush) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: DisconnectCalendarPushName,
		Summary: "Takes one of the caller's own calendar pushes off: its grant is forgotten and nothing more is " +
			"written. The events it wrote stay in the calendar for its owner to remove there.",
		SideEffects: "Removes the push and what it remembers writing, and writes an audit entry.",
		TokenScope:  viewsRead,
		Input:       []usecase.Field{{Name: "push_id", Kind: usecase.KindID, Required: true, Description: "The push."}},
		Audit:       usecase.AuditDeclaration{Required: true, Action: DisconnectedAction, TargetType: pushTarget, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			if err := actor.RequireScope(viewsRead); err != nil {
				return nil, err
			}
			id, err := in.ID("push_id")
			if err != nil {
				return nil, err
			}
			p := h.Pushes
			err = p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				push, _, err := p.Store.Find(ctx, id)
				if err != nil {
					return err
				}
				if push.AccountID != actor.AccountID {
					return shared.ErrNotFound.WithDetail("calendar_push.unknown")
				}
				return p.Store.Disconnect(ctx, id)
			})
			if err != nil {
				return nil, err
			}
			p.record(ctx, actor, DisconnectedAction, id)
			return usecase.Output{}, nil
		}),
	}
}

// Run is one push's run: it reads the feed as its owner, draws an access token and writes what
// changed. It answers when to run next, or the zero time when the push is gone or stopped.
func (p Pushes) Run(ctx context.Context, tenantID, pushID shared.ID) (time.Time, error) {
	scope := persistence.Scope{TenantID: tenantID}
	var (
		push    integration.CalendarPush
		sealed  crypto.Sealed
		written []integration.PushedEvent
	)
	err := p.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		if push, sealed, err = p.Store.Find(ctx, pushID); err != nil {
			return err
		}
		written, err = p.Store.Events(ctx, pushID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if push.State != integration.PushActive {
		return time.Time{}, nil
	}
	run, cancel := context.WithTimeout(ctx, runLimit)
	defer cancel()
	failure, permanent := p.push(run, tenantID, push, sealed, written)
	now := p.Clock.Now()
	push.Pushed(now, failure, permanent)
	if err := p.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error { return p.Store.Record(ctx, push) }); err != nil {
		return time.Time{}, err
	}
	if push.State != integration.PushActive {
		return time.Time{}, nil
	}
	return now.Add(integration.PushInterval), nil
}

// push writes one run and answers the failure's code, and whether it is for good.
func (p Pushes) push(ctx context.Context, tenantID shared.ID, push integration.CalendarPush, sealed crypto.Sealed, written []integration.PushedEvent) (string, bool) {
	rows, err := p.Rows.ExecuteFeed(ctx, tenantID, push.FeedID)
	if errors.Is(err, shared.ErrNotFound) {
		return "calendar_push.feed_unknown", true
	}
	if err != nil {
		return "calendar_push.provider_unreachable", false
	}
	refresh, err := p.Encryptor.Open(ctx, sealed, secretPurpose(push.ID))
	if err != nil {
		return "calendar_push.grant_revoked", true
	}
	client, clientSecret, err := p.client(push.Provider)
	if err != nil {
		return "calendar_push.provider_not_offered", false
	}
	token, err := p.Authorizer.Refresh(ctx, client, clientSecret, refresh)
	if err != nil {
		return codeOf(err)
	}
	if !token.Refresh.IsEmpty() && token.Refresh.Reveal() != refresh.Reveal() {
		if resealed, err := p.Encryptor.Seal(ctx, token.Refresh, secretPurpose(push.ID)); err == nil {
			_ = p.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
				return p.Store.Reseal(ctx, push.ID, resealed)
			})
		}
	}
	base := ""
	if p.Addresses != nil {
		base, _ = p.Addresses.Of(ctx)
	}
	plan := integration.PlanPush(Events(rows, base), written)
	scope := persistence.Scope{TenantID: tenantID}
	now := p.Clock.Now()
	keep := func(entry shared.ID, remote, digest string) error {
		return p.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			return p.Store.PutEvent(ctx, push.ID, integration.PushedEvent{EntryID: entry, RemoteID: remote, Digest: digest}, now)
		})
	}
	for _, event := range plan.Create {
		remote, err := p.Calendar.Put(ctx, push.Provider, token.Access, "", event)
		if err != nil {
			return codeOf(err)
		}
		if err := keep(event.EntryID, remote, event.Digest()); err != nil {
			return "calendar_push.provider_unreachable", false
		}
	}
	for _, update := range plan.Update {
		remote, err := p.Calendar.Put(ctx, push.Provider, token.Access, update.RemoteID, update.Event)
		if err != nil {
			return codeOf(err)
		}
		if err := keep(update.Event.EntryID, remote, update.Event.Digest()); err != nil {
			return "calendar_push.provider_unreachable", false
		}
	}
	for _, gone := range plan.Delete {
		if err := p.Calendar.Delete(ctx, push.Provider, token.Access, gone.RemoteID); err != nil {
			return codeOf(err)
		}
		if err := p.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
			return p.Store.DropEvent(ctx, push.ID, gone.EntryID)
		}); err != nil {
			return "calendar_push.provider_unreachable", false
		}
	}
	return "", false
}

// codeOf reads a refusal: a grant the provider no longer takes is for good, anything else passes.
func codeOf(err error) (string, bool) {
	var typed *shared.Error
	if errors.As(err, &typed) {
		switch typed.DetailCode {
		case "calendar_push.grant_revoked", "mail.oauth_revoked", "mail.oauth_refused":
			return "calendar_push.grant_revoked", true
		}
	}
	return "calendar_push.provider_unreachable", false
}

// Events are a feed's dated entries as a push writes them: the feed's own shape, an all-day entry
// on its day in its own zone or the owner's.
func Events(rows FeedRows, base string) []integration.PushEvent {
	zone, err := time.LoadLocation(rows.TimeZone)
	if err != nil || rows.TimeZone == "" {
		zone = time.UTC
	}
	link := ""
	if base != "" {
		link = strings.TrimRight(base, "/") + "/items/"
	}
	out := make([]integration.PushEvent, 0, len(rows.Items))
	for _, item := range rows.Items {
		if item.Due == nil {
			continue
		}
		event := integration.PushEvent{EntryID: item.ID, Summary: item.Title, Start: item.Due.At.UTC()}
		if link != "" {
			event.URL = link + item.ID.String()
		}
		if item.Due.DateOnly {
			local := zone
			if item.Due.TimeZone != "" {
				if own, err := time.LoadLocation(item.Due.TimeZone); err == nil {
					local = own
				}
			}
			day := item.Due.At.In(local)
			event.Start, event.AllDay = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC), true
		}
		out = append(out, event)
	}
	return out
}
