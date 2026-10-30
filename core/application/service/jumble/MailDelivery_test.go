// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	mail "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The mail intake (ADR-0094, UC-JUM-09): a ticket key makes a comment, a handle a new entry, then
// the sender rules and the default; who may write is a level with lists; whatever no rule may file
// waits in the jumble with its reason, and every message is taken once.

const inboundDomain = "in.example.test"

var (
	mailCollection = shared.ID("01936f2a-7c1e-7000-8000-000000009001")
	mailItem       = shared.ID("01936f2a-7c1e-7000-8000-000000009002")
	opener         = shared.ID("01936f2a-7c1e-7000-8000-000000009003")
	otherPlace     = shared.ID("01936f2a-7c1e-7000-8000-000000009004")
)

// intake is the intake's store, in memory: settings, receipts and the places' names.
type intake struct {
	settings  mail.Settings
	receipts  []mail.Receipt
	names     map[string]shared.ID
	handles   map[string]shared.ID
	keys      map[mail.Key]shared.ID
	failWith  error
	setHandle map[shared.ID]string
}

func newIntake() *intake {
	return &intake{
		settings: mail.Settings{Enabled: true, Name: "familie", Senders: mail.Senders{Level: mail.LevelMembers}, OpenedBy: opener},
		names:    map[string]shared.ID{"familie": tenant},
		handles:  map[string]shared.ID{"einkauf": mailCollection, "buero": otherPlace},
		keys:     map[mail.Key]shared.ID{{Handle: "einkauf", Number: 42}: mailItem},
	}
}

func (s *intake) Find(context.Context) (mail.Settings, error) { return s.settings, s.failWith }
func (s *intake) Save(_ context.Context, settings mail.Settings, _ shared.ID, _ time.Time) error {
	s.settings = settings
	return nil
}

func (s *intake) WorkspaceNamed(_ context.Context, name string) (shared.ID, error) {
	if id, ok := s.names[name]; ok && s.settings.Enabled {
		return id, nil
	}
	return "", shared.ErrNotFound
}

func (s *intake) Seen(_ context.Context, messageID string) (bool, error) {
	for _, receipt := range s.receipts {
		if receipt.MessageID == messageID {
			return true, nil
		}
	}
	return false, nil
}

func (s *intake) Record(ctx context.Context, receipt mail.Receipt) (bool, error) {
	if seen, _ := s.Seen(ctx, receipt.MessageID); seen {
		return false, nil
	}
	s.receipts = append(s.receipts, receipt)
	return true, nil
}

func (s *intake) CountFrom(_ context.Context, sender string, _ time.Time) (int, error) {
	count := 0
	for _, receipt := range s.receipts {
		if receipt.Sender == sender {
			count++
		}
	}
	return count, nil
}

func (s *intake) Recent(context.Context, int) ([]mail.Receipt, error) { return s.receipts, nil }
func (s *intake) DeleteExpired(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (s *intake) CountExpired(context.Context, time.Time, int) (int, error) { return 0, nil }

func (s *intake) CollectionByHandle(_ context.Context, handle string) (shared.ID, error) {
	if id, ok := s.handles[handle]; ok {
		return id, nil
	}
	return "", shared.ErrNotFound
}

func (s *intake) ItemByKey(_ context.Context, key mail.Key) (shared.ID, error) {
	if id, ok := s.keys[key]; ok {
		return id, nil
	}
	return "", shared.ErrNotFound
}

func (s *intake) KeyOf(_ context.Context, itemID shared.ID) (mail.Key, error) {
	for key, id := range s.keys {
		if id == itemID {
			return key, nil
		}
	}
	return mail.Key{}, shared.ErrNotFound
}

func (s *intake) SetHandle(_ context.Context, collectionID shared.ID, handle string) error {
	if s.setHandle == nil {
		s.setHandle = map[shared.ID]string{}
	}
	s.setHandle[collectionID] = handle
	return nil
}

// people answers fixed accounts.
type people []identity.Account

func (p people) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	for _, account := range p {
		if account.ID == id {
			return account, nil
		}
	}
	return identity.Account{}, shared.ErrNotFound
}

func (p people) FindByEmail(_ context.Context, email string) (identity.Account, error) {
	for _, account := range p {
		if strings.EqualFold(account.Email, email) {
			return account, nil
		}
	}
	return identity.Account{}, shared.ErrNotFound
}

// call is one use case the intake performed, as whom.
type call struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
}

// catalogue records calls, and refuses where told to.
type catalogue struct {
	calls  []call
	refuse error
	// hidden refuses a field the way a collection's structure does: notes on a shopping list.
	hidden map[string]bool
}

func (c *catalogue) Invoke(
	_ context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	c.calls = append(c.calls, call{name: name, actor: actor, in: in})
	if c.refuse != nil && name != attachMediaName {
		return nil, c.refuse
	}
	if _, notes := in["notes"]; notes && c.hidden["notes"] && name == createWorkItemName {
		return nil, shared.ErrCapabilityNotSupported.WithDetail("items.capability_hidden")
	}
	if c.hidden[name] {
		return nil, shared.ErrCapabilityNotSupported.WithDetail("items.capability_hidden")
	}
	switch name {
	case createWorkItemName:
		return usecase.Output{"id": mailItem.String()}, nil
	case addCommentName:
		return usecase.Output{"id": "01936f2a-7c1e-7000-8000-00000000c001"}, nil
	}
	return usecase.Output{}, nil
}

type intakeRig struct {
	receive   ReceiveMail
	store     *intake
	entries   *entryStore
	catalogue *catalogue
}

func newIntakeRig() *intakeRig {
	rig := &intakeRig{store: newIntake(), entries: newEntryStore(), catalogue: &catalogue{}}
	rig.receive = ReceiveMail{
		Settings: rig.store, Receipts: rig.store, Places: rig.store,
		Accounts: people{
			{ID: account, Kind: identity.AccountUser, Status: identity.AccountActive, Email: "anna@example.org", DisplayName: "Anna"},
			{ID: opener, Kind: identity.AccountUser, Status: identity.AccountActive, Email: "olga@example.org", DisplayName: "Olga"},
		},
		Entries: rig.entries, Media: &ingestedMedia{}, Catalogue: rig.catalogue, Events: &published{},
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
		Domain: inboundDomain,
	}
	return rig
}

func (r *intakeRig) deliver(t *testing.T, message ReceivedMail) MailDeliveryResult {
	t.Helper()
	if message.Recipients == nil && message.TenantID.IsZero() {
		message.Recipients = []string{"familie@" + inboundDomain}
	}
	result, err := r.receive.Execute(context.Background(), message)
	if err != nil {
		t.Fatalf("receiving: %v", err)
	}
	return result
}

func TestAHandleInTheSubjectMakesAnEntryInItsCollection(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{
		Sender: "Anna@Example.org", Subject: "WG: Milch und Eier #Einkauf", MessageID: "m1",
		Body:        "Bitte mitbringen.\n\nAm 3.2.2026 um 10:00 schrieb Bob:\n> alt",
		Attachments: []mediaservice.IngestedFile{{FileName: "liste.pdf", Content: []byte("%PDF")}},
	})
	if result.Outcome != mail.OutcomeEntry || result.ItemID != mailItem {
		t.Fatalf("became %+v", result)
	}
	create, attach := rig.catalogue.calls[0], rig.catalogue.calls[1]
	if create.name != createWorkItemName || create.actor.AccountID != account ||
		create.in["title"] != "Milch und Eier" || create.in["notes"] != "Bitte mitbringen." ||
		create.in["collection_id"] != mailCollection.String() {
		t.Errorf("created %+v as %+v", create.in, create.actor)
	}
	if attach.name != attachMediaName || attach.in["media_id"] != attachmentID.String() {
		t.Errorf("attached %+v", attach)
	}
	if len(rig.store.receipts) != 1 || rig.store.receipts[0].Outcome != mail.OutcomeEntry ||
		rig.store.receipts[0].Door != InboundDoor {
		t.Errorf("receipts %+v", rig.store.receipts)
	}
}

func TestATicketKeyMakesACommentAndWinsOverAHandle(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{
		Sender: "anna@example.org", Subject: "Re: [EINKAUF-42] #buero", Body: "Erledigt.\n> alt", MessageID: "m2",
	})
	if result.Outcome != mail.OutcomeComment || result.ItemID != mailItem {
		t.Fatalf("became %+v", result)
	}
	if comment := rig.catalogue.calls[0]; comment.name != addCommentName || comment.in["body"] != "Erledigt." {
		t.Errorf("commented %+v", comment)
	}
}

func TestAPlusTagOnAConnectedMailboxRoutes(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{
		TenantID: tenant, Mailbox: "familie@gmail.com", Recipients: []string{"familie+einkauf-42@gmail.com"},
		Sender: "anna@example.org", Subject: "Antwort", Body: "Ja.", MessageID: "m3",
	})
	if result.Outcome != mail.OutcomeComment {
		t.Fatalf("became %+v", result)
	}
	if rig.store.receipts[0].Door != "familie@gmail.com" {
		t.Errorf("door %q", rig.store.receipts[0].Door)
	}
}

func TestASenderRuleAndTheDefaultFileWhatNamesNoPlace(t *testing.T) {
	rig := newIntakeRig()
	rig.store.settings.SenderRoutes = []mail.SenderRoute{{Match: "@example.org", CollectionID: otherPlace}}
	rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "Rechnung", MessageID: "m4"})
	if got := rig.catalogue.calls[0].in["collection_id"]; got != otherPlace.String() {
		t.Fatalf("filed into %v, want the sender rule's", got)
	}

	rig = newIntakeRig()
	rig.store.settings.DefaultCollectionID = mailCollection
	rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "Irgendwas", MessageID: "m5"})
	if got := rig.catalogue.calls[0].in["collection_id"]; got != mailCollection.String() {
		t.Fatalf("filed into %v, want the default", got)
	}
}

func TestWhatNamesNothingWaitsInTheJumbleWithItsReason(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "Hallo", MessageID: "m6"})
	if result.Outcome != mail.OutcomeJumble || result.Reason != mail.ReasonNoRoute {
		t.Fatalf("became %+v", result)
	}
	result = rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#gibtsnicht", MessageID: "m7"})
	if result.Reason != mail.ReasonUnknownPlace {
		t.Fatalf("an unknown handle became %+v", result)
	}
	if entry := rig.entries.rows[entryID]; entry.IntakeReason != string(mail.ReasonUnknownPlace) {
		t.Errorf("the entry carries %q", entry.IntakeReason)
	}
}

func TestAStrangerWaitsUnlessTheLevelAdmitsThem(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{Sender: "kunde@shop.test", Subject: "#einkauf Angebot", MessageID: "m8"})
	if result.Outcome != mail.OutcomeJumble || result.Reason != mail.ReasonSenderNotAllowed {
		t.Fatalf("became %+v", result)
	}
	entry := rig.entries.rows[entryID]
	if entry.MeantFor != (domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}) {
		t.Errorf("meant for %+v", entry.MeantFor)
	}

	rig = newIntakeRig()
	rig.store.settings.Senders = mail.Senders{Level: mail.LevelAllowed, Allowed: []string{"@shop.test"}}
	result = rig.deliver(t, ReceivedMail{
		Sender: "kunde@shop.test", SenderName: "Kunde", Subject: "#einkauf Angebot", Body: "Preis: 3 €", MessageID: "m9",
	})
	if result.Outcome != mail.OutcomeEntry {
		t.Fatalf("an allowed sender's mail became %+v", result)
	}
	create := rig.catalogue.calls[0]
	if create.actor.AccountID != opener || !strings.HasPrefix(create.in.String("notes"), "From: Kunde <kunde@shop.test>") {
		t.Errorf("written as %s with %q", create.actor.AccountID, create.in.String("notes"))
	}
}

func TestABlockedOrAutomaticOrFloodingSenderIsDropped(t *testing.T) {
	rig := newIntakeRig()
	rig.store.settings.Senders.Blocked = []string{"@spam.test"}
	if result := rig.deliver(t, ReceivedMail{Sender: "x@spam.test", Subject: "#einkauf", MessageID: "b1"}); result.Reason != mail.ReasonBlocked || result.Outcome != mail.OutcomeDiscarded {
		t.Fatalf("blocked became %+v", result)
	}
	if result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Automatic: true, MessageID: "b2"}); result.Reason != mail.ReasonAutomatic {
		t.Fatalf("automatic became %+v", result)
	}
	for index := range mail.MailsPerSenderPerHour {
		rig.store.receipts = append(rig.store.receipts, mail.Receipt{MessageID: "f" + string(rune('a'+index)), Sender: "flood@example.org"})
	}
	if result := rig.deliver(t, ReceivedMail{Sender: "flood@example.org", Subject: "#einkauf", MessageID: "b3"}); result.Reason != mail.ReasonTooMany {
		t.Fatalf("a flood became %+v", result)
	}
	if len(rig.catalogue.calls) != 0 || len(rig.entries.rows) != 0 {
		t.Errorf("a dropped message wrote %d calls and %d entries", len(rig.catalogue.calls), len(rig.entries.rows))
	}
}

func TestSpamUnparseableAndForbiddenGoToTheJumble(t *testing.T) {
	rig := newIntakeRig()
	if result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf", Spam: true, MessageID: "s1"}); result.Reason != mail.ReasonSpam {
		t.Fatalf("spam became %+v", result)
	}
	if result := rig.deliver(t, ReceivedMail{Unparseable: true, Raw: []byte("garbage"), MessageID: "s2"}); result.Reason != mail.ReasonUnparseable {
		t.Fatalf("unparseable became %+v", result)
	}
	rig.catalogue.refuse = shared.ErrForbidden
	if result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf", MessageID: "s3"}); result.Reason != mail.ReasonNotPermitted {
		t.Fatalf("forbidden became %+v", result)
	}
}

func TestAMessageIsTakenOnce(t *testing.T) {
	rig := newIntakeRig()
	first := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf Milch", MessageID: "<same@x>"})
	again := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf Milch", MessageID: "<same@x>"})
	if first.Duplicate || !again.Duplicate || len(rig.catalogue.calls) != 1 {
		t.Fatalf("first %+v, again %+v, %d calls", first, again, len(rig.catalogue.calls))
	}
	// Without a Message-ID the raw message's digest is the identity.
	rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf", Raw: []byte("raw")})
	if again := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf", Raw: []byte("raw")}); !again.Duplicate {
		t.Fatalf("the same raw message twice became %+v", again)
	}
}

func TestAnAddressNamingNoWorkspaceOrAnIntakeOffIsNotFound(t *testing.T) {
	rig := newIntakeRig()
	for _, message := range []ReceivedMail{
		{Recipients: []string{"nobody@" + inboundDomain}, Sender: "anna@example.org"},
		{Recipients: []string{"familie@elsewhere.test"}, Sender: "anna@example.org"},
	} {
		if _, err := rig.receive.Execute(context.Background(), message); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%v answered %v", message.Recipients, err)
		}
	}
	rig.store.settings.Enabled = false
	if _, err := rig.receive.Execute(context.Background(), ReceivedMail{TenantID: tenant, Sender: "a@b.de"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an intake that is off answered %v", err)
	}
	rig.receive.Domain = ""
	if _, err := rig.receive.Execute(context.Background(), ReceivedMail{Recipients: []string{"familie@" + inboundDomain}}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("no receiving domain answered %v", err)
	}
}

func TestAForgedOrUnknownSenderIsNobodyTheWorkspaceKnows(t *testing.T) {
	rig := newIntakeRig()
	result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", AuthFailed: true, Subject: "#einkauf", MessageID: "a1"})
	if result.Reason != mail.ReasonSenderNotAllowed {
		t.Fatalf("a forged member became %+v", result)
	}
	rig.store.settings.Senders.Level = mail.LevelAnyone
	rig.store.settings.OpenedBy = ""
	if result := rig.deliver(t, ReceivedMail{Sender: "x@y.test", Subject: "#einkauf", MessageID: "a2"}); result.Reason != mail.ReasonSenderNotAllowed {
		t.Fatalf("anyone without an opener became %+v", result)
	}
}

func TestTheTitleFallsBackToTheTextAndTheSender(t *testing.T) {
	if got := mailTitle(ReceivedMail{Subject: "#einkauf", Body: "\n  Erste Zeile\nzweite"}); got != "Erste Zeile" {
		t.Errorf("from the text: %q", got)
	}
	if got := mailTitle(ReceivedMail{Sender: "a@b.de", SenderName: "Anna"}); got != "From: Anna <a@b.de>" {
		t.Errorf("from the sender: %q", got)
	}
}

func TestCommentingAnEntryAnswersThroughTheCatalogue(t *testing.T) {
	h := newHarness()
	entry, _ := domain.NewEntry(domain.NewEntryInput{
		ID: entryID, TenantID: tenant, Channel: domain.ChannelEmail, RawSubject: "Only a subject", Now: now,
		MeantFor: domain.MailTarget{Kind: domain.TargetItem, ID: mailItem}, IntakeReason: string(mail.ReasonSenderNotAllowed),
	})
	_ = h.store.Insert(context.Background(), entry)
	calls := &catalogue{}
	// No entry named: the one the mail was meant for.
	out, err := CommentJumbleEntry{Writer: h.writer, Catalogue: calls}.Descriptor().Handler.Invoke(
		context.Background(), actor(), usecase.Input{"entry_id": entryID.String()})
	if err != nil || out["status"] != "PROCESSED" || calls.calls[0].in["body"] != "Only a subject" ||
		calls.calls[0].in["item_id"] != mailItem.String() {
		t.Fatalf("answered %+v, %v, calls %+v", out, err, calls.calls)
	}
	// Meant for nothing and naming nothing: there is no entry to comment on.
	other := newHarness()
	plain, _ := domain.NewEntry(domain.NewEntryInput{ID: entryID, TenantID: tenant, Channel: domain.ChannelAPI, RawSubject: "x", Now: now})
	_ = other.store.Insert(context.Background(), plain)
	if _, err := (CommentJumbleEntry{Writer: other.writer, Catalogue: &catalogue{}}).Execute(context.Background(), actor(), entryID, ""); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("no destination answered %v", err)
	}
}

// A shopping list keeps no description: the entry is made without, the text becomes its first
// comment, and a place that keeps no files takes the mail without them.
func TestAPlaceThatKeepsNoNotesStillTakesTheMail(t *testing.T) {
	rig := newIntakeRig()
	rig.catalogue.hidden = map[string]bool{"notes": true, attachMediaName: true}
	result := rig.deliver(t, ReceivedMail{
		Sender: "anna@example.org", Subject: "Gemüsebrühe #einkauf", Body: "Zwei Packungen.", MessageID: "n1",
		Attachments: []mediaservice.IngestedFile{{FileName: "a.pdf", Content: []byte("%PDF")}},
	})
	if result.Outcome != mail.OutcomeEntry {
		t.Fatalf("became %+v", result)
	}
	var created, commented bool
	for _, made := range rig.catalogue.calls {
		if made.name == createWorkItemName && made.in["notes"] == nil {
			created = true
		}
		if made.name == addCommentName && made.in["body"] == "Zwei Packungen." {
			commented = true
		}
	}
	if !created || !commented {
		t.Errorf("calls %+v", rig.catalogue.calls)
	}

	// A place that refuses outright sends the mail to the jumble rather than losing it.
	rig = newIntakeRig()
	rig.catalogue.refuse = shared.ErrValidation
	if result := rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf", MessageID: "n2"}); result.Outcome != mail.OutcomeJumble || result.Reason != mail.ReasonRefusedByPlace {
		t.Errorf("a refused write became %+v", result)
	}
}

type requesterLog struct {
	address, name string
	item          shared.ID
	at            time.Time
	calls         int
}

func (r *requesterLog) RecordContact(_ context.Context, address, name string, item shared.ID, at time.Time) error {
	r.address, r.name, r.item, r.at = address, name, item, at
	r.calls++
	return nil
}

// accepted.md B: a sender without an account whose mail an account filed on their behalf is
// recorded as a requester, with the entry and the moment; a member's own mail records none.
func TestASenderWithoutAnAccountIsRecordedAsARequester(t *testing.T) {
	rig := newIntakeRig()
	requesters := &requesterLog{}
	rig.receive.Requesters = requesters
	rig.store.settings.Senders = mail.Senders{Level: mail.LevelAnyone}
	result := rig.deliver(t, ReceivedMail{
		Sender: "Kunde@Shop.test", SenderName: "Kunde", Subject: "#einkauf Angebot", Body: "Preis", MessageID: "r1",
	})
	if result.Outcome != mail.OutcomeEntry {
		t.Fatalf("became %+v", result)
	}
	if requesters.calls != 1 || requesters.address != "kunde@shop.test" || requesters.name != "Kunde" ||
		requesters.item != result.ItemID || !requesters.at.Equal(now) {
		t.Errorf("recorded %+v", requesters)
	}

	rig = newIntakeRig()
	requesters = &requesterLog{}
	rig.receive.Requesters = requesters
	rig.deliver(t, ReceivedMail{Sender: "anna@example.org", Subject: "#einkauf Milch", MessageID: "r2"})
	if requesters.calls != 0 {
		t.Errorf("a member's own mail recorded a requester: %+v", requesters)
	}
}
