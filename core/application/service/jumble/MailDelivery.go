// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	addCommentName  = "AddComment"
	attachMediaName = "AttachMedia"
	commentsWrite   = "comments:write"
)

// MailOutcome is what a delivery became.
type MailOutcome string

const (
	// MailBecameJumbleEntry is a jumble entry - the jumble's own mail, or anything not clearly
	// allowed into the place it was meant for.
	MailBecameJumbleEntry MailOutcome = "JUMBLE_ENTRY"
	// MailBecameEntry is a new entry in a collection.
	MailBecameEntry MailOutcome = "ENTRY"
	// MailBecameComment is a comment on an entry.
	MailBecameComment MailOutcome = "COMMENT"
	// MailDiscarded is a message a machine sent on its own; nothing was stored.
	MailDiscarded MailOutcome = "DISCARDED"
)

// AddressedMail is one message on the receiving domain, parsed.
type AddressedMail struct {
	// Recipients are the addresses the delivery names, the envelope's first. The first one at the
	// receiving domain with a live code decides.
	Recipients []string
	Sender     string
	SenderName string
	Subject    string
	Body       string
	// Attachments as MailDelivery's: bytes, judged by the media guard.
	Attachments []mediaservice.IngestedFile
	Raw         []byte
	Unparseable bool
	// Automatic, Spam and AuthFailed are the header signals (ADR-0092 decisions 5 and 7).
	Automatic  bool
	Spam       bool
	AuthFailed bool
}

// MailDeliveryResult says what the delivery became and where it is.
type MailDeliveryResult struct {
	Outcome MailOutcome
	// EntryID is the jumble entry, ItemID the entry or the entry commented on, CommentID the
	// comment.
	EntryID   shared.ID
	ItemID    shared.ID
	CommentID shared.ID
}

// MailAccounts is the slice of the account store a delivery reads: who a sender's address
// belongs to, and who opened an address to anyone.
type MailAccounts interface {
	Find(ctx context.Context, accountID shared.ID) (identity.Account, error)
	FindByEmail(ctx context.Context, email string) (identity.Account, error)
}

var _ MailAccounts = identityrepo.Accounts(nil)

// DeliverAddressedMail routes one message to the place its address leads to (ADR-0092).
//
// Not a catalogue entry, for IntakeMail's reason: the code authenticates a place, never a person.
// What a recognised sender does goes through the catalogue as that person, so their rights are
// asked exactly where a click asks them.
type DeliverAddressedMail struct {
	Addresses repository.MailAddresses
	Places    PlaceFinder
	Accounts  MailAccounts
	Entries   repository.Entries
	// Media is optional, IntakeMail's way.
	Media      AttachmentIngest
	Catalogue  Catalogue
	Events     outboxEvents
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Domain is the receiving domain; empty answers every delivery not-found.
	Domain string
}

// Execute delivers.
func (h DeliverAddressedMail) Execute(
	ctx context.Context, mail AddressedMail,
) (MailDeliveryResult, error) {
	if h.Domain == "" {
		return MailDeliveryResult{}, mailGone()
	}
	// A loop ends at its first turn, before the address is even looked at (decision 7).
	if mail.Automatic {
		return MailDeliveryResult{Outcome: MailDiscarded}, nil
	}

	code, tenantID, err := h.resolve(ctx, mail.Recipients)
	if err != nil {
		return MailDeliveryResult{}, err
	}
	scope := persistence.Scope{TenantID: tenantID}

	var (
		address domain.MailAddress
		place   MailPlace
		sender  identity.Account
		known   bool
	)
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, err := h.Addresses.ByCode(ctx, code)
		if err != nil {
			return err
		}
		address = found
		count, err := h.Addresses.CountDelivery(ctx, address.ID, h.Clock.Now())
		if err != nil {
			return err
		}
		if count > domain.MailDeliveriesPerHour {
			return shared.ErrRateLimited.WithDetail("mail.address_busy")
		}
		if place, err = h.Places.Find(ctx, address.Target); err != nil {
			return err
		}
		if place.Closed {
			return mailGone()
		}
		sender, known = h.recognise(ctx, mail)
		return nil
	})
	if err != nil {
		return MailDeliveryResult{}, err
	}

	files := mail.Attachments
	if mail.Unparseable {
		files = append(files, mediaservice.IngestedFile{FileName: RawMailFileName, Content: mail.Raw})
	}
	stored, err := h.ingest(ctx, tenantID, files)
	if err != nil {
		return MailDeliveryResult{}, err
	}

	if address.Target.Kind == domain.TargetJumble || mail.Spam || mail.Unparseable {
		return h.toJumble(ctx, tenantID, mail, stored, address.Target)
	}

	actor, ok := h.actorFor(ctx, scope, address, sender, known)
	if !ok {
		return h.toJumble(ctx, tenantID, mail, stored, address.Target)
	}
	result, err := h.write(ctx, actor, address.Target, mail, stored, !known)
	if errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) {
		// The sender may not do this here - the jumble waits for somebody who may.
		return h.toJumble(ctx, tenantID, mail, stored, address.Target)
	}
	return result, err
}

// resolve finds the first recipient whose code names a live address, and its tenant.
func (h DeliverAddressedMail) resolve(
	ctx context.Context, recipients []string,
) (string, shared.ID, error) {
	for _, recipient := range recipients {
		code, ok := domain.MailCodeOf(recipient, h.Domain)
		if !ok {
			continue
		}
		var tenantID shared.ID
		err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
			func(ctx context.Context) error {
				found, err := h.Addresses.TenantOf(ctx, code)
				tenantID = found
				return err
			})
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		return code, tenantID, nil
	}
	return "", "", mailGone()
}

// recognise finds the person the sender's address belongs to: an active person account of the
// workspace, and only where the inbound service did not say the address was forged.
func (h DeliverAddressedMail) recognise(
	ctx context.Context, mail AddressedMail,
) (identity.Account, bool) {
	if mail.AuthFailed || h.Accounts == nil || !strings.Contains(mail.Sender, "@") {
		return identity.Account{}, false
	}
	account, err := h.Accounts.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(mail.Sender)))
	if err != nil || account.Kind != identity.AccountUser || account.Status != identity.AccountActive {
		return identity.Account{}, false
	}
	return account, true
}

// actorFor is who the mail is written as: the recognised sender, or - under ANYONE - the account
// that opened the address to everyone. False sends the mail to the jumble.
func (h DeliverAddressedMail) actorFor(
	ctx context.Context, scope persistence.Scope, address domain.MailAddress,
	sender identity.Account, known bool,
) (appshared.ActorContext, bool) {
	account := sender
	if !known {
		if address.Policy != domain.SendersAnyone || address.OpenedBy.IsZero() || h.Accounts == nil {
			return appshared.ActorContext{}, false
		}
		err := h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
			found, err := h.Accounts.Find(ctx, address.OpenedBy)
			account = found
			return err
		})
		if err != nil || account.Status != identity.AccountActive {
			return appshared.ActorContext{}, false
		}
	}
	return appshared.ActorContext{
		Kind:        appshared.ActorUser,
		TenantID:    scope.TenantID,
		AccountID:   account.ID,
		AccountName: account.DisplayName,
		// Exactly what writing by mail needs, and nothing more: the person's own token could not do
		// less, and this one cannot do more.
		Scopes:   []string{itemsWriteScope, commentsWrite},
		Locale:   account.Locale,
		TimeZone: account.TimeZone,
	}, true
}

// write performs the act as the actor, in one transaction: the entry or the comment, and the files.
func (h DeliverAddressedMail) write(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
	mail AddressedMail, files []shared.ID, onBehalf bool,
) (MailDeliveryResult, error) {
	text := domain.CutQuotedReply(mail.Body)
	if onBehalf {
		text = fromLine(mail) + "\n\n" + text
	}
	var result MailDeliveryResult
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		switch target.Kind {
		case domain.TargetCollection:
			input := usecase.Input{
				"type": "TASK", "title": mailTitle(mail), "collection_id": target.ID.String(),
			}
			if strings.TrimSpace(text) != "" {
				input["notes"] = cutBytes(strings.TrimSpace(text), domain.MaxBodyBytes)
			}
			out, err := h.Catalogue.Invoke(ctx, createWorkItemName, actor, input)
			if err != nil {
				return err
			}
			itemID, err := shared.ParseID(out.String("id"))
			if err != nil {
				return shared.ErrInternal.WithDetail("mail.entry_incomplete").WithCause(err)
			}
			result = MailDeliveryResult{Outcome: MailBecameEntry, ItemID: itemID}
		default:
			body := strings.TrimSpace(text)
			if body == "" || (onBehalf && strings.TrimSpace(domain.CutQuotedReply(mail.Body)) == "") {
				body = strings.TrimSpace(fromLine(mail) + "\n\n" + mailTitle(mail))
			}
			out, err := h.Catalogue.Invoke(ctx, addCommentName, actor, usecase.Input{
				"item_id": target.ID.String(), "body": cutBytes(body, domain.MaxBodyBytes),
			})
			if err != nil {
				return err
			}
			commentID, _ := shared.ParseID(out.String("id"))
			result = MailDeliveryResult{
				Outcome: MailBecameComment, ItemID: target.ID, CommentID: commentID,
			}
		}
		for _, file := range files {
			if _, err := h.Catalogue.Invoke(ctx, attachMediaName, actor, usecase.Input{
				"item_id": result.ItemID.String(), "media_id": file.String(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

// toJumble stores the mail as a jumble entry, meant for the place where that was not the jumble.
func (h DeliverAddressedMail) toJumble(
	ctx context.Context, tenantID shared.ID, mail AddressedMail, files []shared.ID,
	target domain.MailTarget,
) (MailDeliveryResult, error) {
	body := cutBytes(mail.Body, domain.MaxBodyBytes)
	if mail.Unparseable {
		body = cutBytes(printable(string(mail.Raw)), domain.MaxBodyBytes)
	}
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: h.IDs.NewID(), TenantID: tenantID, Channel: domain.ChannelEmail,
		Sender:      senderWithin(mail.Sender),
		RawSubject:  cut(mail.Subject, domain.MaxSubjectLength),
		RawBody:     body,
		Attachments: files,
		MeantFor:    target,
		Now:         h.Clock.Now(),
	})
	if err != nil {
		return MailDeliveryResult{}, err
	}
	err = h.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		if err := h.Entries.Insert(ctx, entry); err != nil {
			return err
		}
		if h.Events == nil {
			return nil
		}
		envelope, err := event.NewJumbleEntryReceived(
			h.IDs.NewID(), entry, event.Actor{Kind: shared.ActorSystem}, entry.ReceivedAt,
			event.Cause{})
		if err != nil {
			return err
		}
		return h.Events.Append(ctx, envelope)
	})
	if err != nil {
		return MailDeliveryResult{}, err
	}
	return MailDeliveryResult{Outcome: MailBecameJumbleEntry, EntryID: entry.ID}, nil
}

func (h DeliverAddressedMail) ingest(
	ctx context.Context, tenantID shared.ID, files []mediaservice.IngestedFile,
) ([]shared.ID, error) {
	if h.Media == nil || len(files) == 0 {
		return nil, nil
	}
	return h.Media.Execute(ctx, tenantID, files)
}

// mailTitle is the subject, or the text's first line, or the sender - an entry needs a title, and
// none of these puts words in the workspace nobody wrote.
func mailTitle(mail AddressedMail) string {
	if subject := strings.TrimSpace(mail.Subject); subject != "" {
		return cut(subject, domain.MaxSubjectLength)
	}
	for _, line := range strings.Split(domain.CutQuotedReply(mail.Body), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return cut(line, domain.MaxSubjectLength)
		}
	}
	return cut(fromLine(mail), domain.MaxSubjectLength)
}

// fromLine names the sender above what an account writes on their behalf.
func fromLine(mail AddressedMail) string {
	sender := senderWithin(mail.Sender)
	if name := strings.TrimSpace(mail.SenderName); name != "" {
		return "From: " + name + " <" + sender + ">"
	}
	return "From: " + sender
}
