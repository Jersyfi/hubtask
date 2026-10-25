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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Mail on the receiving domain (ADR-0092, UC-JUM-09): the code names the place, a recognised
// sender acts as themselves through the catalogue, and whatever is not clearly allowed waits in
// the jumble, meant for the place.

const inboundDomain = "in.example.test"

var (
	mailCollection = shared.ID("01936f2a-7c1e-7000-8000-000000009001")
	mailItem       = shared.ID("01936f2a-7c1e-7000-8000-000000009002")
	opener         = shared.ID("01936f2a-7c1e-7000-8000-000000009003")
)

// addressBook is the address store, in memory.
type addressBook struct {
	rows    []domain.MailAddress
	revoked map[shared.ID]bool
	counts  map[shared.ID]int
}

func newAddressBook() *addressBook {
	return &addressBook{revoked: map[shared.ID]bool{}, counts: map[shared.ID]int{}}
}

func (b *addressBook) TenantOf(_ context.Context, code string) (shared.ID, error) {
	for _, row := range b.rows {
		if row.Code == code && !b.revoked[row.ID] {
			return row.TenantID, nil
		}
	}
	return "", mailGone()
}

func (b *addressBook) Live(_ context.Context, target domain.MailTarget) (domain.MailAddress, error) {
	for _, row := range b.rows {
		if row.Target == target && !b.revoked[row.ID] {
			return row, nil
		}
	}
	return domain.MailAddress{}, mailGone()
}

func (b *addressBook) ByCode(ctx context.Context, code string) (domain.MailAddress, error) {
	for _, row := range b.rows {
		if row.Code == code && !b.revoked[row.ID] {
			return row, nil
		}
	}
	return domain.MailAddress{}, mailGone()
}

func (b *addressBook) Replace(_ context.Context, address domain.MailAddress, _ time.Time) error {
	for _, row := range b.rows {
		if row.Target == address.Target {
			b.revoked[row.ID] = true
		}
	}
	b.rows = append(b.rows, address)
	return nil
}

func (b *addressBook) SetPolicy(_ context.Context, address domain.MailAddress) error {
	for index, row := range b.rows {
		if row.ID == address.ID && !b.revoked[row.ID] {
			b.rows[index] = address
			return nil
		}
	}
	return mailGone()
}

func (b *addressBook) CountDelivery(_ context.Context, id shared.ID, _ time.Time) (int, error) {
	b.counts[id]++
	return b.counts[id], nil
}

// places answers fixed places.
type places map[domain.MailTarget]MailPlace

func (p places) Find(_ context.Context, target domain.MailTarget) (MailPlace, error) {
	if place, ok := p[target]; ok {
		return place, nil
	}
	return MailPlace{}, mailGone()
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

// call is one use case the delivery performed, as whom.
type call struct {
	name  string
	actor appshared.ActorContext
	in    usecase.Input
}

// catalogue records calls, and refuses where told to.
type catalogue struct {
	calls  []call
	refuse error
}

func (c *catalogue) Invoke(
	_ context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	c.calls = append(c.calls, call{name: name, actor: actor, in: in})
	if c.refuse != nil && name != attachMediaName {
		return nil, c.refuse
	}
	switch name {
	case createWorkItemName:
		return usecase.Output{"id": mailItem.String()}, nil
	case addCommentName:
		return usecase.Output{"id": "01936f2a-7c1e-7000-8000-00000000c001"}, nil
	}
	return usecase.Output{}, nil
}

type deliveryRig struct {
	deliver   DeliverAddressedMail
	book      *addressBook
	entries   *entryStore
	catalogue *catalogue
	media     *ingestedMedia
}

func newDeliveryRig(t *testing.T) *deliveryRig {
	t.Helper()
	rig := &deliveryRig{
		book: newAddressBook(), entries: newEntryStore(), catalogue: &catalogue{},
		media: &ingestedMedia{},
	}
	rig.deliver = DeliverAddressedMail{
		Addresses: rig.book,
		Places: places{
			{Kind: domain.TargetJumble}:                         {Target: domain.MailTarget{Kind: domain.TargetJumble}},
			{Kind: domain.TargetCollection, ID: mailCollection}: {Target: domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}, Name: "Büro"},
			{Kind: domain.TargetItem, ID: mailItem}:             {Target: domain.MailTarget{Kind: domain.TargetItem, ID: mailItem}, Name: "Startseite"},
		},
		Accounts: people{
			{ID: account, Kind: identity.AccountUser, Status: identity.AccountActive, Email: "anna@example.org", DisplayName: "Anna"},
			{ID: opener, Kind: identity.AccountUser, Status: identity.AccountActive, Email: "olga@example.org", DisplayName: "Olga"},
		},
		Entries: rig.entries, Media: rig.media, Catalogue: rig.catalogue, Events: &published{},
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
		Domain: inboundDomain,
	}
	return rig
}

// address mints an address for the place and answers it as a person types it.
func (r *deliveryRig) address(t *testing.T, target domain.MailTarget, label string) string {
	t.Helper()
	secret := []byte{byte(len(r.book.rows) + 1), 2, 3, 4, 5, 6, 7, 8, 9, 10}
	address, err := domain.NewMailAddress(
		shared.ID("01936f2a-7c1e-7000-8000-0000000a000"+string(rune('1'+len(r.book.rows)))),
		tenant, target, secret, account, now)
	if err != nil {
		t.Fatal(err)
	}
	r.book.rows = append(r.book.rows, address)
	return address.Address(label, inboundDomain)
}

func TestAMemberMailsAnEntryIntoACollection(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}, "Büro")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "Anna@Example.org", Subject: "Angebot Druckerei",
		Body:        "Bitte prüfen.\n\nAm 3.2.2026 um 10:00 schrieb Druckerei:\n> Angebot",
		Attachments: []mediaservice.IngestedFile{{FileName: "angebot.pdf", Content: []byte("%PDF")}},
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if result.Outcome != MailBecameEntry || result.ItemID != mailItem {
		t.Fatalf("became %+v", result)
	}
	if len(rig.catalogue.calls) != 2 {
		t.Fatalf("calls %+v", rig.catalogue.calls)
	}
	create, attach := rig.catalogue.calls[0], rig.catalogue.calls[1]
	if create.name != createWorkItemName || create.actor.AccountID != account ||
		create.in["title"] != "Angebot Druckerei" || create.in["notes"] != "Bitte prüfen." ||
		create.in["collection_id"] != mailCollection.String() {
		t.Errorf("created %+v as %+v", create.in, create.actor)
	}
	if attach.name != attachMediaName || attach.in["media_id"] != attachmentID.String() {
		t.Errorf("attached %+v", attach)
	}
	if len(rig.entries.rows) != 0 {
		t.Errorf("%d jumble entries for a member's mail", len(rig.entries.rows))
	}
}

func TestAMemberMailsACommentOntoAnEntry(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetItem, ID: mailItem}, "Startseite")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{"someone@else.test", to}, Sender: "anna@example.org",
		Subject: "Re: Startseite", Body: "Passt so.\n> earlier",
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if result.Outcome != MailBecameComment || result.ItemID != mailItem {
		t.Fatalf("became %+v", result)
	}
	comment := rig.catalogue.calls[0]
	if comment.name != addCommentName || comment.in["body"] != "Passt so." ||
		comment.actor.AccountID != account {
		t.Errorf("commented %+v as %+v", comment.in, comment.actor)
	}
}

func TestAStrangerWaitsInTheJumbleMeantForThePlace(t *testing.T) {
	rig := newDeliveryRig(t)
	target := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}
	to := rig.address(t, target, "Büro")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "stranger@example.net", Subject: "Offer",
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if result.Outcome != MailBecameJumbleEntry || len(rig.catalogue.calls) != 0 {
		t.Fatalf("became %+v with calls %+v", result, rig.catalogue.calls)
	}
	entry := rig.entries.rows[result.EntryID]
	if entry.MeantFor != target || entry.Channel != domain.ChannelEmail {
		t.Errorf("the entry is %+v", entry)
	}
}

func TestAMemberWithoutTheRightWaitsInTheJumble(t *testing.T) {
	rig := newDeliveryRig(t)
	rig.catalogue.refuse = shared.ErrForbidden
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}, "Büro")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "anna@example.org", Subject: "Hi",
	})
	if err != nil || result.Outcome != MailBecameJumbleEntry {
		t.Fatalf("became %+v, %v", result, err)
	}
}

func TestAForgedOrSpamMailIsNotTakenAsTheMember(t *testing.T) {
	for name, mail := range map[string]AddressedMail{
		"spf failed": {Sender: "anna@example.org", Subject: "Hi", AuthFailed: true},
		"spam":       {Sender: "anna@example.org", Subject: "Hi", Spam: true},
	} {
		t.Run(name, func(t *testing.T) {
			rig := newDeliveryRig(t)
			mail.Recipients = []string{
				rig.address(t, domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}, "x"),
			}
			result, err := rig.deliver.Execute(context.Background(), mail)
			if err != nil || result.Outcome != MailBecameJumbleEntry || len(rig.catalogue.calls) != 0 {
				t.Fatalf("became %+v, %v, calls %+v", result, err, rig.catalogue.calls)
			}
		})
	}
}

func TestOpenToAnyoneWritesAsTheOpenerHeadedWithTheSender(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}, "Büro")
	rig.book.rows[0].Policy, rig.book.rows[0].OpenedBy = domain.SendersAnyone, opener

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "client@example.net", SenderName: "Client",
		Subject: "Request", Body: "Please call.",
	})
	if err != nil || result.Outcome != MailBecameEntry {
		t.Fatalf("became %+v, %v", result, err)
	}
	create := rig.catalogue.calls[0]
	if create.actor.AccountID != opener ||
		create.in["notes"] != "From: Client <client@example.net>\n\nPlease call." {
		t.Errorf("created %+v as %s", create.in, create.actor.AccountID)
	}
}

func TestTheJumblesAddressMakesAJumbleEntry(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetJumble}, "jumble")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "anna@example.org", Subject: "Note",
	})
	if err != nil || result.Outcome != MailBecameJumbleEntry {
		t.Fatalf("became %+v, %v", result, err)
	}
	if entry := rig.entries.rows[result.EntryID]; !entry.MeantFor.IsZero() {
		t.Errorf("a jumble mail names a place: %+v", entry.MeantFor)
	}
}

func TestWhatLeadsNowhereStoresNothing(t *testing.T) {
	rig := newDeliveryRig(t)
	collection := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}
	old := rig.address(t, collection, "Büro")
	rig.book.revoked[rig.book.rows[0].ID] = true

	cases := map[string][]string{
		"a replaced address":  {old},
		"another domain":      {"buero." + rig.book.rows[0].Code + "@elsewhere.test"},
		"no recipient at all": nil,
	}
	for name, recipients := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := rig.deliver.Execute(context.Background(), AddressedMail{
				Recipients: recipients, Sender: "anna@example.org", Subject: "Hi",
			})
			if !errors.Is(err, shared.ErrNotFound) {
				t.Errorf("answered %v", err)
			}
		})
	}
	if len(rig.media.files) != 0 || len(rig.entries.rows) != 0 || len(rig.catalogue.calls) != 0 {
		t.Error("something was stored for an address that leads nowhere")
	}
}

func TestAClosedPlaceTakesNothing(t *testing.T) {
	rig := newDeliveryRig(t)
	target := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}
	to := rig.address(t, target, "Büro")
	closed := rig.deliver.Places.(places)
	place := closed[target]
	place.Closed = true
	closed[target] = place

	if _, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Sender: "anna@example.org", Subject: "Hi",
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an archived place answered %v", err)
	}
}

func TestAnAutomaticMailIsDiscardedAndTooManyAreRefused(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetJumble}, "jumble")

	result, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Subject: "Out of office", Automatic: true,
	})
	if err != nil || result.Outcome != MailDiscarded || len(rig.entries.rows) != 0 {
		t.Fatalf("an automatic mail became %+v, %v", result, err)
	}

	rig.book.counts[rig.book.rows[0].ID] = domain.MailDeliveriesPerHour
	if _, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Subject: "One more",
	}); !errors.Is(err, shared.ErrRateLimited) {
		t.Errorf("the sixty-first delivery answered %v", err)
	}
}

func TestWithoutAReceivingDomainNothingIsTaken(t *testing.T) {
	rig := newDeliveryRig(t)
	to := rig.address(t, domain.MailTarget{Kind: domain.TargetJumble}, "jumble")
	rig.deliver.Domain = ""
	if _, err := rig.deliver.Execute(context.Background(), AddressedMail{
		Recipients: []string{to}, Subject: "Hi",
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("answered %v", err)
	}
}
