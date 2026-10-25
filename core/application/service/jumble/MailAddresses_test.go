// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// A place's address (ADR-0092, UC-JUM-09 checks 1, 2 and 8): minted on the first read, replaced
// on request, shaped only by who may shape the place, and absent without a receiving domain.

func addressWriter(book *addressBook, auth *authorizer, domainName string) MailAddressWriter {
	collection := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}
	return MailAddressWriter{
		Addresses: book,
		Places: places{
			{Kind: domain.TargetJumble}: {Target: domain.MailTarget{Kind: domain.TargetJumble},
				Path: []identity.Scope{identity.TenantScope()}},
			collection: {Target: collection, Name: "Büro"},
		},
		Authorizer: auth, Audit: &auditSink{}, UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now),
		IDs: ids{next: entryID}, Entropy: &entropy{}, Domain: domainName,
	}
}

func TestTheFirstReadMintsTheAddressAndTheNextReadsIt(t *testing.T) {
	book, auth := newAddressBook(), &authorizer{}
	writer := addressWriter(book, auth, inboundDomain)
	target := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}

	first, err := ShowMailAddress{Writer: writer}.Execute(context.Background(), actor(), target)
	if err != nil || !first.Available {
		t.Fatalf("first read: %+v, %v", first, err)
	}
	if !strings.HasPrefix(first.Text, "buro.") || !strings.HasSuffix(first.Text, "@"+inboundDomain) {
		t.Errorf("the address reads %q", first.Text)
	}
	second, _ := ShowMailAddress{Writer: writer}.Execute(context.Background(), actor(), target)
	if second.Text != first.Text || len(book.rows) != 1 {
		t.Errorf("the second read minted again: %q, %d rows", second.Text, len(book.rows))
	}
	if auth.requests[0].Permission != service.PermissionRead {
		t.Errorf("reading asked %s", auth.requests[0].Permission)
	}
}

func TestANewAddressRetiresTheOldOneAndKeepsWhoMaySend(t *testing.T) {
	book, auth := newAddressBook(), &authorizer{}
	writer := addressWriter(book, auth, inboundDomain)
	target := domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection}
	if _, err := (SetMailSenderPolicy{Writer: writer}).Execute(
		context.Background(), actor(), target, domain.SendersAnyone); err != nil {
		t.Fatal(err)
	}
	old, _ := book.Live(context.Background(), target)

	rotated, err := RotateMailAddress{Writer: writer}.Execute(context.Background(), actor(), target)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Address.Code == old.Code {
		t.Error("the new address has the old code")
	}
	if _, err := book.TenantOf(context.Background(), old.Code); !errors.Is(err, shared.ErrNotFound) {
		t.Error("the old code still names the workspace")
	}
	if rotated.Address.Policy != domain.SendersAnyone || rotated.Address.OpenedBy != account {
		t.Errorf("the new address lost who may send: %+v", rotated.Address)
	}
	if auth.requests[len(auth.requests)-1].Permission != service.PermissionStructure {
		t.Errorf("replacing a collection's address asked %s", auth.requests[len(auth.requests)-1].Permission)
	}
}

func TestShapingTheJumblesAddressIsTheAutomationPermission(t *testing.T) {
	book, auth := newAddressBook(), &authorizer{}
	writer := addressWriter(book, auth, inboundDomain)
	if _, err := (RotateMailAddress{Writer: writer}).Execute(
		context.Background(), actor(), domain.MailTarget{Kind: domain.TargetJumble}); err != nil {
		t.Fatal(err)
	}
	if auth.requests[0].Permission != service.PermissionAutomation {
		t.Errorf("asked %s", auth.requests[0].Permission)
	}
	auth.refuse = shared.ErrForbidden
	if _, err := (RotateMailAddress{Writer: writer}).Execute(
		context.Background(), actor(), domain.MailTarget{Kind: domain.TargetJumble}); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a refused caller replaced the address: %v", err)
	}
}

func TestWithoutAReceivingDomainThereIsNoAddress(t *testing.T) {
	book := newAddressBook()
	writer := addressWriter(book, &authorizer{}, "")
	view, err := ShowMailAddress{Writer: writer}.Execute(
		context.Background(), actor(), domain.MailTarget{Kind: domain.TargetJumble})
	if err != nil || view.Available || len(book.rows) != 0 {
		t.Errorf("answered %+v, %v with %d rows", view, err, len(book.rows))
	}
	if _, err := (RotateMailAddress{Writer: writer}).Execute(
		context.Background(), actor(), domain.MailTarget{Kind: domain.TargetJumble}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("replacing without a domain answered %v", err)
	}
}

func TestAReplyAddressIsTheEntrysOwn(t *testing.T) {
	book := newAddressBook()
	writer := addressWriter(book, &authorizer{}, inboundDomain)
	writer.Places = places{
		{Kind: domain.TargetItem, ID: mailItem}: {
			Target: domain.MailTarget{Kind: domain.TargetItem, ID: mailItem}, Name: "Startseite texten",
		},
	}
	address, err := MailReplies{Writer: writer}.ReplyAddress(context.Background(), tenant, mailItem)
	if err != nil || !strings.HasPrefix(address, "startseite-texten.") {
		t.Fatalf("reply address %q, %v", address, err)
	}
	again, _ := MailReplies{Writer: writer}.ReplyAddress(context.Background(), tenant, mailItem)
	if again != address {
		t.Errorf("a second notification got %q", again)
	}
	writer.Domain = ""
	if none, _ := (MailReplies{Writer: writer}).ReplyAddress(context.Background(), tenant, mailItem); none != "" {
		t.Errorf("without a domain the reply address is %q", none)
	}
}

func TestAMailMeantForAnEntryIsSortedInAsAComment(t *testing.T) {
	h := newHarness()
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: entryID, TenantID: tenant, Channel: domain.ChannelEmail, Sender: "client@example.net",
		RawSubject: "Re: Startseite", RawBody: "Looks good.\n> quoted",
		Attachments: []shared.ID{mediaID},
		MeantFor:    domain.MailTarget{Kind: domain.TargetItem, ID: mailItem}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = h.store.Insert(context.Background(), entry)
	calls := &catalogue{}

	settled, err := CommentJumbleEntry{Writer: h.writer, Catalogue: calls}.Execute(
		context.Background(), actor(), entryID, "")
	if err != nil {
		t.Fatalf("settling: %v", err)
	}
	if settled.Status != domain.StatusProcessed || settled.TargetItemID != mailItem {
		t.Errorf("settled %+v", settled)
	}
	if len(calls.calls) != 2 || calls.calls[0].name != addCommentName ||
		calls.calls[0].in["body"] != "From: client@example.net\n\nLooks good." ||
		calls.calls[1].name != attachMediaName {
		t.Errorf("calls %+v", calls.calls)
	}
}
