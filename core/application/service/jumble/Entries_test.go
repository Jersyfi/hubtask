// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.ID("01936f2a-7c1e-7000-8000-000000001001")
	account = shared.ID("01936f2a-7c1e-7000-8000-000000001002")
	entryID = shared.ID("01936f2a-7c1e-7000-8000-000000001003")
	mediaID = shared.ID("01936f2a-7c1e-7000-8000-000000001004")
	now     = time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC)
)

// entryStore is the repository in memory.
type entryStore struct {
	rows  map[shared.ID]domain.Entry
	order []shared.ID
}

func newEntryStore() *entryStore { return &entryStore{rows: map[shared.ID]domain.Entry{}} }

func (s *entryStore) Insert(_ context.Context, entry domain.Entry) error {
	s.rows[entry.ID] = entry
	s.order = append(s.order, entry.ID)
	return nil
}

func (s *entryStore) Find(_ context.Context, id shared.ID) (domain.Entry, error) {
	entry, found := s.rows[id]
	if !found {
		return domain.Entry{}, shared.ErrNotFound.WithDetail("jumble.entry_not_found")
	}
	return entry, nil
}

func (s *entryStore) List(_ context.Context, query repository.Query) (repository.Page, error) {
	page := repository.Page{}
	for i := len(s.order) - 1; i >= 0; i-- {
		entry := s.rows[s.order[i]]
		if query.Status != "" && entry.Status != query.Status {
			continue
		}
		if query.Channel != "" && entry.Channel != query.Channel {
			continue
		}
		page.Entries = append(page.Entries, entry)
	}
	return page, nil
}

func (s *entryStore) Settle(_ context.Context, entry domain.Entry) (bool, error) {
	current, found := s.rows[entry.ID]
	if !found || current.Status != domain.StatusNew {
		return false, nil
	}
	s.rows[entry.ID] = entry
	return true, nil
}

// mediaStore answers the objects a submission names.
type mediaStore struct {
	rows     map[shared.ID]media.Object
	adjusted map[shared.ID]int
}

func newMediaStore() *mediaStore {
	return &mediaStore{
		rows: map[shared.ID]media.Object{
			mediaID: {ID: mediaID, TenantID: tenant, Status: media.StatusReady, Usage: media.UsageAttachment},
		},
		adjusted: map[shared.ID]int{},
	}
}

func (m *mediaStore) Find(_ context.Context, id shared.ID) (media.Object, error) {
	object, found := m.rows[id]
	if !found {
		return media.Object{}, shared.ErrNotFound.WithDetail("media.object_not_found")
	}
	return object, nil
}

func (m *mediaStore) AdjustRefCount(_ context.Context, id shared.ID, delta int) error {
	m.adjusted[id] += delta
	return nil
}

// authorizer records what was asked and refuses when told to.
type authorizer struct {
	requests []access.Request
	refuse   error
}

func (a *authorizer) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	return a.refuse
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type published struct{ envelopes []event.Envelope }

func (p *published) Append(_ context.Context, envelope event.Envelope) error {
	p.envelopes = append(p.envelopes, envelope)
	return nil
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
func (u unitOfWork) WithinReadOnly(ctx context.Context, s persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, s, fn)
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

type harness struct {
	writer Writer
	store  *entryStore
	media  *mediaStore
	auth   *authorizer
	sink   *auditSink
	events *published
}

func newHarness() *harness {
	h := &harness{
		store: newEntryStore(), media: newMediaStore(),
		auth: &authorizer{}, sink: &auditSink{}, events: &published{},
	}
	h.writer = Writer{
		Entries: h.store, Media: h.media, Events: h.events,
		Authorizer: h.auth, Audit: h.sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}
	return h
}

func actor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account,
		Scopes: []string{"items:read", "items:write"},
	}
}

// The acceptance criterion: an entry submitted through a near channel lands with its channel, and
// fires jumble.entry_received - inside the same transaction as the row.
func TestASubmissionLandsAndAnnounces(t *testing.T) {
	h := newHarness()

	entry, err := (SubmitJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(),
		SubmitCommand{
			Channel: domain.ChannelQuickCapture,
			RawBody: "Call the customer back", Attachments: []shared.ID{mediaID},
		})
	if err != nil {
		t.Fatalf("submitting: %v", err)
	}
	if entry.Channel != domain.ChannelQuickCapture || entry.Status != domain.StatusNew {
		t.Errorf("the entry reads %+v", entry)
	}
	if _, stored := h.store.rows[entry.ID]; !stored {
		t.Fatal("the entry was not stored")
	}

	if len(h.events.envelopes) != 1 || h.events.envelopes[0].Type != event.JumbleEntryReceived {
		t.Fatalf("announced %+v", h.events.envelopes)
	}
	payload := h.events.envelopes[0].Payload
	for _, name := range []string{"raw_body", "raw_subject", "sender"} {
		if _, leaked := payload[name]; leaked {
			t.Errorf("the event payload carries %s", name)
		}
	}

	// The attachment was proved sealed and counted as a reference.
	if h.media.adjusted[mediaID] != 1 {
		t.Errorf("the attachment's count moved by %d", h.media.adjusted[mediaID])
	}

	// The trail names the channel and never the content.
	if len(h.sink.entries) != 1 || h.sink.entries[0].Action != EntrySubmittedAction {
		t.Fatalf("audited %+v", h.sink.entries)
	}
	for _, value := range h.sink.entries[0].Changes {
		if text, ok := value.(string); ok && text == "Call the customer back" {
			t.Error("the audit entry carries the content")
		}
	}
}

// The far channels are the intakes' own: an entry claiming one here would forge its provenance.
func TestTheFarChannelsCannotBeClaimed(t *testing.T) {
	h := newHarness()

	for _, channel := range []domain.Channel{domain.ChannelEmail, domain.ChannelWebhook} {
		_, err := (SubmitJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(),
			SubmitCommand{Channel: channel, RawBody: "forged"})
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s: error %v, want a validation refusal", channel, err)
		}
	}
	if len(h.store.rows) != 0 {
		t.Error("a forged channel was stored")
	}
}

// An attachment has to exist and be sealed - the failure #231 recorded, refused at the door.
func TestAnUnsealedAttachmentIsRefused(t *testing.T) {
	h := newHarness()
	h.media.rows[mediaID] = media.Object{ID: mediaID, TenantID: tenant, Status: media.StatusPending, Usage: media.UsageAttachment}

	_, err := (SubmitJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(),
		SubmitCommand{RawBody: "with a staging", Attachments: []shared.ID{mediaID}})
	if err == nil {
		t.Fatal("a PENDING staging was accepted as an attachment")
	}

	missing := shared.ID("01936f2a-7c1e-7000-8000-000000001005")
	_, err = (SubmitJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(),
		SubmitCommand{RawBody: "with a ghost", Attachments: []shared.ID{missing}})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation refusal", err)
	}
	if code := shared.AsError(err).DetailCode; code != "jumble.attachment_unknown" {
		t.Errorf("detail code %s", code)
	}
}

// The listing asks the read permission and filters by state and channel.
func TestTheListingFiltersAndIsAuthorised(t *testing.T) {
	h := newHarness()
	submit := SubmitJumbleEntry{Writer: h.writer}
	for i, channel := range []domain.Channel{domain.ChannelAPI, domain.ChannelQuickCapture} {
		h.writer.IDs = ids{next: shared.ID("01936f2a-7c1e-7000-8000-00000000101" + string(rune('0'+i)))}
		submit.Writer = h.writer
		if _, err := submit.Execute(context.Background(), actor(),
			SubmitCommand{Channel: channel, RawBody: "x"}); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	page, err := (ListJumbleEntries{Writer: h.writer}).Execute(context.Background(), actor(),
		repository.Query{Channel: domain.ChannelAPI})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Entries) != 1 || page.Entries[0].Channel != domain.ChannelAPI {
		t.Errorf("the filtered page reads %+v", page.Entries)
	}

	last := h.auth.requests[len(h.auth.requests)-1]
	if string(last.Permission) != "READ" || last.TokenScope != "items:read" {
		t.Errorf("the listing asked %+v", last)
	}
}

// A refusal from the authoriser stores nothing and announces nothing.
func TestARefusedSubmissionLeavesNoTrace(t *testing.T) {
	h := newHarness()
	h.auth.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := (SubmitJumbleEntry{Writer: h.writer}).Execute(context.Background(), actor(),
		SubmitCommand{RawBody: "refused"})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if len(h.store.rows) != 0 || len(h.events.envelopes) != 0 {
		t.Error("a refused submission left a trace")
	}
}
