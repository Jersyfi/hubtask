// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendarpush

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/calendarpush"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	tenant   = shared.ID("00000000-0000-7000-8000-0000000000c1")
	owner    = shared.ID("00000000-0000-7000-8000-0000000000c2")
	stranger = shared.ID("00000000-0000-7000-8000-0000000000c3")
	feedID   = shared.ID("00000000-0000-7000-8000-0000000000c4")
	now      = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clockAt struct{ now time.Time }

func (c clockAt) Now() time.Time { return c.now }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.ID(fmt.Sprintf("00000000-0000-7000-8000-%012d", c.n))
}

type entropy struct{}

func (entropy) Bytes(n int) ([]byte, error) { return make([]byte, n), nil }

// plain seals by naming the purpose, so a grant opened under another push's purpose fails.
type plain struct{}

func (plain) Seal(_ context.Context, s secret.Secret, p crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: string(p), Ciphertext: []byte(s.Reveal())}, nil
}

func (plain) Open(_ context.Context, s crypto.Sealed, p crypto.Purpose) (secret.Secret, error) {
	if s.KeyID != string(p) {
		return secret.Secret{}, errors.New("another purpose")
	}
	return secret.New(string(s.Ciphertext)), nil
}

type store struct {
	pushes  map[shared.ID]integration.CalendarPush
	sealed  map[shared.ID]crypto.Sealed
	events  map[shared.ID]integration.PushedEvent
	pending map[string]repository.Pending
}

func newStore() *store {
	return &store{pushes: map[shared.ID]integration.CalendarPush{}, sealed: map[shared.ID]crypto.Sealed{},
		events: map[shared.ID]integration.PushedEvent{}, pending: map[string]repository.Pending{}}
}

func (s *store) List(_ context.Context, account shared.ID) ([]integration.CalendarPush, error) {
	var out []integration.CalendarPush
	for _, p := range s.pushes {
		if p.AccountID == account {
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *store) Count(ctx context.Context, account shared.ID) (int, error) {
	l, _ := s.List(ctx, account)
	return len(l), nil
}
func (s *store) Find(_ context.Context, id shared.ID) (integration.CalendarPush, crypto.Sealed, error) {
	p, ok := s.pushes[id]
	if !ok {
		return p, crypto.Sealed{}, shared.ErrNotFound
	}
	return p, s.sealed[id], nil
}
func (s *store) Connect(_ context.Context, p integration.CalendarPush, sealed crypto.Sealed) (shared.ID, error) {
	for id, known := range s.pushes {
		if known.FeedID == p.FeedID && known.Provider == p.Provider {
			s.sealed[id] = sealed
			return id, nil
		}
	}
	s.pushes[p.ID], s.sealed[p.ID] = p, sealed
	return p.ID, nil
}
func (s *store) Record(_ context.Context, p integration.CalendarPush) error {
	s.pushes[p.ID] = p
	return nil
}
func (s *store) Reseal(_ context.Context, id shared.ID, sealed crypto.Sealed) error {
	s.sealed[id] = sealed
	return nil
}
func (s *store) Disconnect(_ context.Context, id shared.ID) error {
	delete(s.pushes, id)
	return nil
}
func (s *store) Events(context.Context, shared.ID) ([]integration.PushedEvent, error) {
	var out []integration.PushedEvent
	for _, e := range s.events {
		out = append(out, e)
	}
	return out, nil
}
func (s *store) PutEvent(_ context.Context, _ shared.ID, e integration.PushedEvent, _ time.Time) error {
	s.events[e.EntryID] = e
	return nil
}
func (s *store) DropEvent(_ context.Context, _, entry shared.ID) error {
	delete(s.events, entry)
	return nil
}
func (s *store) Await(_ context.Context, p repository.Pending, hash []byte) error {
	s.pending[string(hash)] = p
	return nil
}
func (s *store) Take(_ context.Context, hash []byte) (repository.Pending, error) {
	p, ok := s.pending[string(hash)]
	if !ok {
		return p, shared.ErrNotFound
	}
	delete(s.pending, string(hash))
	return p, nil
}
func (s *store) DeleteExpired(context.Context, time.Time) (int, error) { return 0, nil }

type feeds struct{ feed integration.CalendarFeed }

func (f feeds) Insert(context.Context, integration.CalendarFeed, integration.FeedToken) error {
	return nil
}
func (f feeds) FindByToken(context.Context, integration.FeedToken) (integration.CalendarFeed, error) {
	return f.feed, nil
}
func (f feeds) Find(_ context.Context, id shared.ID) (integration.CalendarFeed, error) {
	if id != f.feed.ID {
		return integration.CalendarFeed{}, shared.ErrNotFound
	}
	return f.feed, nil
}
func (f feeds) ListForAccount(context.Context, shared.ID) ([]integration.CalendarFeed, error) {
	return nil, nil
}
func (f feeds) Revoke(context.Context, shared.ID, time.Time) (bool, error) { return false, nil }

type rows struct{ rows FeedRows }

func (r *rows) ExecuteFeed(context.Context, shared.ID, shared.ID) (FeedRows, error) {
	return r.rows, nil
}

type authorizer struct{ revoked bool }

func (authorizer) Exchange(_ context.Context, client maildomain.OAuthClient, _ secret.Secret, code, _, _ string) (mailboxport.Token, error) {
	return mailboxport.Token{Access: secret.New("access"), Refresh: secret.New("refresh-" + code), Email: "ada@example.org"}, nil
}
func (a authorizer) Refresh(context.Context, maildomain.OAuthClient, secret.Secret, secret.Secret) (mailboxport.Token, error) {
	if a.revoked {
		return mailboxport.Token{}, shared.ErrForbidden.WithDetail("mail.oauth_revoked")
	}
	return mailboxport.Token{Access: secret.New("access")}, nil
}

type calendar struct {
	puts    []string
	deletes []string
}

func (c *calendar) Put(_ context.Context, _ integration.PushProvider, _ secret.Secret, remote string, e integration.PushEvent) (string, error) {
	c.puts = append(c.puts, remote+">"+e.Summary)
	if remote == "" {
		remote = "evt-" + e.EntryID.String()
	}
	return remote, nil
}
func (c *calendar) Delete(_ context.Context, _ integration.PushProvider, _ secret.Secret, remote string) error {
	c.deletes = append(c.deletes, remote)
	return nil
}

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, r)
	return "job", nil
}

type fixture struct {
	pushes   Pushes
	store    *store
	rows     *rows
	calendar *calendar
	jobs     *jobs
}

func newFixture(revoked bool) fixture {
	f := fixture{store: newStore(), rows: &rows{}, calendar: &calendar{}, jobs: &jobs{}}
	f.pushes = Pushes{
		Store: f.store, Rows: f.rows, Calendar: f.calendar, Authorizer: authorizer{revoked: revoked}, Encryptor: plain{},
		Entropy: entropy{}, Apps: map[string]App{"google": {ClientID: "client", ClientSecret: secret.New("s")}},
		RedirectURL: "https://hubtask.example/calendar-push/callback", Jobs: f.jobs, UnitOfWork: passthrough{},
		Clock: clockAt{now}, IDs: &counter{},
		Feeds: feeds{feed: integration.CalendarFeed{ID: feedID, TenantID: tenant, AccountID: owner, ViewID: "v", Kind: integration.CalendarKind}},
	}
	return f
}

func actorOf(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, Scopes: []string{viewsRead}}
}

func call(t *testing.T, d usecase.Descriptor, actor appshared.ActorContext, in map[string]any) (usecase.Output, error) {
	t.Helper()
	return d.Handler.Invoke(context.Background(), actor, usecase.Input(in))
}

// connect walks the provider's sign-in for the owner's feed and answers the push.
func connect(t *testing.T, f fixture) usecase.Output {
	t.Helper()
	started, err := call(t, AuthorizeCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"feed_id": feedID.String(), "provider": "GOOGLE"})
	if err != nil {
		t.Fatal(err)
	}
	address, _ := url.Parse(started["authorize_url"].(string))
	query := address.Query()
	if !strings.Contains(query.Get("scope"), "calendar.events") || strings.Contains(query.Get("scope"), "mail.google") || strings.Contains(query.Get("scope"), "IMAP") || query.Get("code_challenge") == "" {
		t.Errorf("the consent asks for %q", query.Get("scope"))
	}
	done, err := call(t, CompleteCalendarPushAuthorization{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"state": query.Get("state"), "code": "c1"})
	if err != nil {
		t.Fatal(err)
	}
	return done
}

// UC-INT-16 check 1: the owner connects their own feed through the provider's consent for the
// calendar's events alone; the push is kept with the account signed in with and its first run
// queued. Somebody else's feed is not found, and a state is good once, for whoever started it.
func TestTheOwnerConnectsTheirFeedThroughTheProvidersConsent(t *testing.T) {
	f := newFixture(false)
	done := connect(t, f)
	if done["provider"] != "GOOGLE" || done["address"] != "ada@example.org" || done["state"] != "ACTIVE" {
		t.Errorf("push = %v", done)
	}
	if len(f.jobs.requests) != 1 || f.jobs.requests[0].Kind != queue.KindCalendarPush {
		t.Errorf("jobs = %v", f.jobs.requests)
	}
	if _, err := call(t, AuthorizeCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(stranger), map[string]any{"feed_id": feedID.String(), "provider": "GOOGLE"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else's feed: %v", err)
	}
	if _, err := call(t, AuthorizeCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"feed_id": feedID.String(), "provider": "MICROSOFT"}); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("a provider the installation has no app for: %v", err)
	}
	started, _ := call(t, AuthorizeCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"feed_id": feedID.String(), "provider": "GOOGLE"})
	address, _ := url.Parse(started["authorize_url"].(string))
	state := address.Query().Get("state")
	if _, err := call(t, CompleteCalendarPushAuthorization{Pushes: f.pushes}.Descriptor(), actorOf(stranger), map[string]any{"state": state, "code": "c"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("another person completing: %v", err)
	}
	if _, err := call(t, CompleteCalendarPushAuthorization{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"state": state, "code": "c"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a state used twice: %v", err)
	}
	listed, err := call(t, ListCalendarPushes{Pushes: f.pushes}.Descriptor(), actorOf(stranger), nil)
	if err != nil || len(listed["data"].([]usecase.Output)) != 0 {
		t.Errorf("somebody else listed %v, %v", listed, err)
	}
	if _, err := call(t, DisconnectCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(stranger), map[string]any{"push_id": done["id"]}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else disconnected: %v", err)
	}
}

// UC-INT-16 check 2 and 3: a run writes the feed's dated entries - the title, the moment and the
// link - updates what changed, removes what the feed no longer serves and leaves the rest.
func TestARunWritesTheFeedsChanges(t *testing.T) {
	f := newFixture(false)
	f.pushes.Addresses = base("https://hubtask.example")
	id := shared.ID(connect(t, f)["id"].(string))
	due := &work.DueDate{At: now.Add(48 * time.Hour)}
	f.rows.rows = FeedRows{Items: []work.WorkItem{
		{ID: "e1", Title: "Ship it", Due: due},
		{ID: "e2", Title: "Plan", Due: due},
		{ID: "e3", Title: "No date"},
	}}
	next, err := f.pushes.Run(context.Background(), tenant, id)
	if err != nil || !next.Equal(now.Add(integration.PushInterval)) {
		t.Fatalf("next = %v, %v", next, err)
	}
	if len(f.calendar.puts) != 2 || len(f.store.events) != 2 {
		t.Fatalf("puts = %v", f.calendar.puts)
	}
	f.calendar.puts = nil
	f.rows.rows = FeedRows{Items: []work.WorkItem{{ID: "e1", Title: "Ship it now", Due: due}}}
	if _, err := f.pushes.Run(context.Background(), tenant, id); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.calendar.puts, ",") != "evt-e1>Ship it now" || strings.Join(f.calendar.deletes, ",") != "evt-e2" {
		t.Errorf("puts = %v, deletes = %v", f.calendar.puts, f.calendar.deletes)
	}
	f.calendar.puts = nil
	if _, err := f.pushes.Run(context.Background(), tenant, id); err != nil || len(f.calendar.puts) != 0 {
		t.Errorf("an unchanged feed wrote %v, %v", f.calendar.puts, err)
	}
	if pushed := f.store.pushes[id]; !pushed.PushedAt.Equal(now) || pushed.Failure != "" {
		t.Errorf("push = %+v", pushed)
	}
	events := Events(FeedRows{Items: []work.WorkItem{{ID: "e9", Title: "Day", Due: &work.DueDate{At: time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC), DateOnly: true}}}, TimeZone: "Europe/Berlin"}, "https://h")
	if len(events) != 1 || !events[0].AllDay || events[0].Start.Format("2006-01-02") != "2026-11-03" || events[0].URL != "https://h/items/e9" {
		t.Errorf("an all-day entry = %+v", events)
	}
}

// UC-INT-16 check 4: a grant the provider no longer takes stops the push, which says so; a
// disconnected push runs no more.
func TestARevokedGrantStopsThePush(t *testing.T) {
	f := newFixture(true)
	id := shared.ID(connect(t, f)["id"].(string))
	next, err := f.pushes.Run(context.Background(), tenant, id)
	if err != nil || !next.IsZero() {
		t.Fatalf("next = %v, %v", next, err)
	}
	if pushed := f.store.pushes[id]; pushed.State != integration.PushFailed || pushed.Failure != "calendar_push.grant_revoked" {
		t.Errorf("push = %+v", pushed)
	}
	if _, err := call(t, DisconnectCalendarPush{Pushes: f.pushes}.Descriptor(), actorOf(owner), map[string]any{"push_id": id.String()}); err != nil {
		t.Fatal(err)
	}
	if next, err := f.pushes.Run(context.Background(), tenant, id); err != nil || !next.IsZero() {
		t.Errorf("a disconnected push ran: %v, %v", next, err)
	}
}

type base string

func (b base) Of(context.Context) (string, error) { return string(b), nil }
