// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	tenant     = shared.ID("00000000-0000-7000-8000-0000000000e1")
	anna       = shared.ID("00000000-0000-7000-8000-0000000000e2")
	bert       = shared.ID("00000000-0000-7000-8000-0000000000e3")
	hub        = shared.ID("00000000-0000-7000-8000-0000000000e4")
	collection = shared.ID("00000000-0000-7000-8000-0000000000e5")
	otherHub   = shared.ID("00000000-0000-7000-8000-0000000000e6")
	entry      = shared.ID("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a51")
	elsewhere  = shared.ID("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a52")
	now        = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	base       = "https://hub.example.org"
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type clockAt struct{}

func (clockAt) Now() time.Time { return now }

type ids struct{}

func (ids) NewID() shared.ID { return "00000000-0000-7000-8000-0000000000f1" }

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, e audit.Entry) error {
	t.entries = append(t.entries, e)
	return nil
}

// gate lets Anna do everything; Bert only reads.
type gate struct{}

func (gate) Authorize(_ context.Context, actor appshared.ActorContext, r access.Request) error {
	if actor.AccountID == bert && r.Permission != service.PermissionRead {
		return shared.ErrForbidden.WithDetail("access.denied")
	}
	return nil
}

type containers struct{ workrepo.Containers }

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id == collection {
		return work.Container{ID: id, Type: work.ContainerCollection, ParentID: hub}, nil
	}
	return work.Container{ID: id, Type: work.ContainerHub}, nil
}

type entries struct{}

func (entries) Entries(_ context.Context, list []shared.ID) ([]workrepo.EntrySummary, error) {
	var out []workrepo.EntrySummary
	for _, id := range list {
		switch id {
		case entry:
			out = append(out, workrepo.EntrySummary{ID: id, HubID: hub, CollectionID: collection})
		case elsewhere:
			out = append(out, workrepo.EntrySummary{ID: id, HubID: otherHub})
		}
	}
	return out, nil
}

type store struct {
	bots   map[shared.ID]integration.ChatBot
	sealed map[shared.ID]crypto.Sealed
}

func (s *store) List(_ context.Context, h shared.ID) ([]integration.ChatBot, error) {
	var out []integration.ChatBot
	for _, b := range s.bots {
		if b.HubID == h {
			out = append(out, b)
		}
	}
	return out, nil
}
func (s *store) Count(ctx context.Context, h shared.ID) (int, error) {
	l, _ := s.List(ctx, h)
	return len(l), nil
}
func (s *store) Find(_ context.Context, id shared.ID) (integration.ChatBot, crypto.Sealed, error) {
	b, ok := s.bots[id]
	if !ok {
		return b, crypto.Sealed{}, shared.ErrNotFound
	}
	return b, s.sealed[id], nil
}
func (s *store) Insert(_ context.Context, b integration.ChatBot, sealed crypto.Sealed) error {
	s.bots[b.ID], s.sealed[b.ID] = b, sealed
	return nil
}
func (s *store) Update(_ context.Context, b integration.ChatBot) error {
	s.bots[b.ID] = b
	return nil
}
func (s *store) Record(_ context.Context, b integration.ChatBot) error {
	s.bots[b.ID] = b
	return nil
}
func (s *store) Delete(_ context.Context, id shared.ID) error {
	delete(s.bots, id)
	return nil
}

type room struct {
	messages []roomport.Message
	posted   []string
	revoked  bool
}

func (r *room) WhoAmI(context.Context, string, secret.Secret) (string, error) {
	return "@hubtask:example.org", nil
}
func (r *room) Join(context.Context, string, secret.Secret, string) error { return nil }
func (r *room) Read(_ context.Context, _ string, token secret.Secret, _, since string) (string, []roomport.Message, error) {
	if r.revoked {
		return "", nil, shared.ErrForbidden.WithDetail("chatbot.token_revoked")
	}
	if token.Reveal() != "tok" {
		return "", nil, errors.New("wrong token")
	}
	if since == "" {
		return "s1", nil, nil
	}
	return "s2", r.messages, nil
}
func (r *room) Post(_ context.Context, _ string, _ secret.Secret, _, txn string, n roomport.Notice) error {
	r.posted = append(r.posted, txn+"|"+n.Body)
	return nil
}

type invoker struct{ calls []string }

func (i *invoker) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	i.calls = append(i.calls, name+":"+actor.AccountID.String())
	if name == createEntry {
		return usecase.Output{"id": "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a59", "title": in.String("title")}, nil
	}
	return usecase.Output{"title": "Renew the certificate", "status": "IN_PROGRESS", "notes": "the secret plan"}, nil
}

type texts struct{}

func (texts) Render(locale, code string, params map[string]string) string {
	return locale + ":" + code + ":" + params["status"]
}

type address struct{}

func (address) Of(context.Context) (string, error) { return base, nil }

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

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, r queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, r)
	return "job", nil
}

type fixture struct {
	bots    Bots
	store   *store
	room    *room
	invoker *invoker
	jobs    *jobs
}

func newFixture() fixture {
	f := fixture{store: &store{bots: map[shared.ID]integration.ChatBot{}, sealed: map[shared.ID]crypto.Sealed{}},
		room: &room{}, invoker: &invoker{}, jobs: &jobs{}}
	f.bots = Bots{
		Store: f.store, Containers: containers{}, Entries: entries{}, Room: f.room, Invoker: f.invoker, Texts: texts{},
		Addresses: address{}, Encryptor: plain{}, Authorizer: gate{}, Jobs: f.jobs, Audit: &trail{},
		UnitOfWork: passthrough{}, Clock: clockAt{}, IDs: ids{},
	}
	return f
}

func actorOf(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: id}
}

func seat(t *testing.T, f fixture, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	return CreateChatBot{f.bots}.Descriptor().Handler.Invoke(context.Background(), actor, usecase.Input{
		"hub_id": hub.String(), "collection_id": collection.String(), "homeserver": "https://matrix.example.org",
		"room": "!r:example.org", "access_token": "tok", "locale": "de", "unfurl": true, "capture": true,
	})
}

// UC-INT-18 check 1: an administrator of the hub seats the bot - its account named, its token
// sealed and never answered, its first read queued; a reader of the hub cannot.
func TestABotIsSeatedByTheHubsAdministrator(t *testing.T) {
	f := newFixture()
	if _, err := seat(t, f, actorOf(bert)); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a reader seated a bot: %v", err)
	}
	out, err := seat(t, f, actorOf(anna))
	if err != nil {
		t.Fatal(err)
	}
	if out["bot_user"] != "@hubtask:example.org" || out["access_token"] != nil || len(f.jobs.requests) != 1 {
		t.Fatalf("out = %v, jobs %d", out, len(f.jobs.requests))
	}
	id := shared.ID(out["id"].(string))
	if string(f.store.sealed[id].Ciphertext) != "tok" || !strings.HasPrefix(f.store.sealed[id].KeyID, "chatbot.token:") {
		t.Fatalf("sealed = %+v", f.store.sealed[id])
	}
	_, err = CreateChatBot{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(anna), usecase.Input{
		"hub_id": hub.String(), "collection_id": hub.String(), "homeserver": "https://matrix.example.org",
		"room": "!r:example.org", "access_token": "tok", "capture": true,
	})
	var e *shared.Error
	if !errors.As(err, &e) || e.DetailCode != "chatbot.collection_not_in_hub" {
		t.Fatalf("a collection outside the hub: %v", err)
	}
}

// UC-INT-18 checks 2 and 3: the first read answers no history; then a link to the hub's entry is
// answered with its title, status and link - never its notes - another hub's entry not at all, a
// `!task` message becomes an entry as whoever seated the bot, and the bot's own messages are
// never read back.
func TestARunUnfurlsTheHubsEntriesAndCapturesTasks(t *testing.T) {
	f := newFixture()
	out, err := seat(t, f, actorOf(anna))
	if err != nil {
		t.Fatal(err)
	}
	id := shared.ID(out["id"].(string))
	next, err := f.bots.Run(context.Background(), tenant, id)
	if err != nil || !next.Equal(now.Add(time.Minute)) || f.store.bots[id].Since != "s1" || len(f.room.posted) != 0 {
		t.Fatalf("first run = %v %v %+v %v", next, err, f.store.bots[id], f.room.posted)
	}
	f.room.messages = []roomport.Message{
		{EventID: "$1", Sender: "@ada:example.org", Body: "look at " + base + "/items/" + entry.String() + " and " + base + "/items/" + elsewhere.String()},
		{EventID: "$2", Sender: "@ada:example.org", Body: "!task Call the supplier"},
		{EventID: "$3", Sender: "@hubtask:example.org", Body: "!task echo"},
	}
	if _, err := f.bots.Run(context.Background(), tenant, id); err != nil {
		t.Fatal(err)
	}
	if len(f.room.posted) != 2 {
		t.Fatalf("posted = %v", f.room.posted)
	}
	unfurl, captured := f.room.posted[0], f.room.posted[1]
	if !strings.HasPrefix(unfurl, "hubtask-$1-a|de:chat.bot_unfurled_status:IN_PROGRESS\nRenew the certificate\n"+base+"/items/"+entry.String()) ||
		strings.Contains(unfurl, "secret plan") || strings.Contains(unfurl, elsewhere.String()) {
		t.Fatalf("unfurl = %q", unfurl)
	}
	if !strings.HasPrefix(captured, "hubtask-$2|de:chat.bot_captured:\nCall the supplier\n"+base+"/items/") {
		t.Fatalf("captured = %q", captured)
	}
	if got := strings.Join(f.invoker.calls, ","); got != getEntry+":"+anna.String()+","+createEntry+":"+anna.String() {
		t.Fatalf("calls = %s", got)
	}
	if f.store.bots[id].Since != "s2" {
		t.Fatal("the position did not move on")
	}
}

// UC-INT-18 check 4: a bot switched off reads nothing, and a token the homeserver refuses stops it
// and says why.
func TestARefusedTokenStopsTheBot(t *testing.T) {
	f := newFixture()
	out, _ := seat(t, f, actorOf(anna))
	id := shared.ID(out["id"].(string))
	f.room.revoked = true
	next, err := f.bots.Run(context.Background(), tenant, id)
	if err != nil || !next.IsZero() || f.store.bots[id].Failure != "chatbot.token_revoked" {
		t.Fatalf("revoked run = %v %v %+v", next, err, f.store.bots[id])
	}
	_, err = UpdateChatBot{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(anna), usecase.Input{"bot_id": id.String(), "enabled": false})
	if err != nil {
		t.Fatal(err)
	}
	if next, _ := f.bots.Run(context.Background(), tenant, id); !next.IsZero() {
		t.Fatal("a bot switched off read its room")
	}
	if _, err := (DeleteChatBot{f.bots}).Descriptor().Handler.Invoke(context.Background(), actorOf(bert), usecase.Input{"bot_id": id.String()}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a reader removed the bot: %v", err)
	}
}
