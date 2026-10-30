// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	mailrepo "github.com/Jersyfi/hubtask/core/application/repository/mail"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	mail "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	addCommentName  = "AddComment"
	attachMediaName = "AttachMedia"
	commentsWrite   = "comments:write"
	// InboundDoor is how a receipt names the operator's inbound service.
	InboundDoor = "inbound"
)

// ReceivedMail is one message for the intake, parsed - from the operator's inbound service or a
// connected mailbox, which are the same to everything below the door (ADR-0094 decision 1).
type ReceivedMail struct {
	// Recipients are the addresses the delivery names, the envelope's first.
	Recipients []string
	Sender     string
	SenderName string
	Subject    string
	Body       string
	// Attachments are bytes, judged by the media guard.
	Attachments []mediaservice.IngestedFile
	Raw         []byte
	Unparseable bool
	// Automatic, Spam and AuthFailed are the header signals.
	Automatic  bool
	Spam       bool
	AuthFailed bool
	// MessageID is the Message-ID header; empty falls back to a digest of the raw message.
	MessageID string
	// TenantID is the workspace a mailbox belongs to. Zero for the inbound service, whose
	// recipient address names the workspace.
	TenantID shared.ID
	// Mailbox is the connected mailbox's own address, whose plus tags route. Empty for the
	// inbound service.
	Mailbox string
}

// MailDeliveryResult says what a message became and where it is.
type MailDeliveryResult struct {
	Outcome mail.Outcome
	Reason  mail.Reason
	// EntryID is the jumble entry, ItemID the entry or the entry commented on, CommentID the
	// comment.
	EntryID   shared.ID
	ItemID    shared.ID
	CommentID shared.ID
	// Duplicate is a message the workspace had already received; nothing was done again.
	Duplicate bool
}

// MailAccounts is the slice of the account store the intake reads: who a sender's address belongs
// to, and the account that opened the intake.
type MailAccounts interface {
	Find(ctx context.Context, accountID shared.ID) (identity.Account, error)
	FindByEmail(ctx context.Context, email string) (identity.Account, error)
}

var _ MailAccounts = identityrepo.Accounts(nil)

// ReceiveMail files one message where it is meant to go (ADR-0094): a comment on the entry its
// ticket key names, a new entry in the collection its handle names or a sender rule or the default
// names, and otherwise the jumble - with the reason.
//
// Not a catalogue entry: an address authenticates nobody. What an admitted sender does goes through
// the catalogue as that person, or as the administrator who opened the intake, so their rights are
// asked exactly where a click asks them.
type ReceiveMail struct {
	Settings mailrepo.Settings
	Receipts mailrepo.Receipts
	Places   mailrepo.Places
	Accounts MailAccounts
	Entries  repository.Entries
	// Media is optional, IntakeMail's way.
	Media      AttachmentIngest
	Catalogue  Catalogue
	Events     outboxEvents
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Requesters records a sender without an account whose mail an account filed on their behalf
	// (REQUESTER, accepted.md B); nil records none.
	Requesters RequesterContacts
	// Domain is the operator's receiving domain; empty takes nothing from the inbound service.
	Domain string
}

// route is where the pipeline decided a message goes, and as whom.
type route struct {
	target  domain.MailTarget
	reason  mail.Reason
	actor   appshared.ActorContext
	behalf  bool
	discard bool
}

// Execute receives.
func (h ReceiveMail) Execute(ctx context.Context, message ReceivedMail) (MailDeliveryResult, error) {
	tenantID, err := h.tenantOf(ctx, message)
	if err != nil {
		return MailDeliveryResult{}, err
	}
	scope := persistence.Scope{TenantID: tenantID}
	if message.MessageID == "" {
		digest := sha256.Sum256(message.Raw)
		message.MessageID = "sha256:" + hex.EncodeToString(digest[:])
	}

	var (
		settings mail.Settings
		decided  route
		seen     bool
	)
	err = h.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, err := h.Settings.Find(ctx)
		if err != nil {
			return err
		}
		settings = found
		if !settings.Enabled {
			return mailGone()
		}
		if seen, err = h.Receipts.Seen(ctx, message.MessageID); err != nil || seen {
			return err
		}
		decided, err = h.decide(ctx, scope, settings, message)
		return err
	})
	if err != nil {
		return MailDeliveryResult{}, err
	}
	if seen {
		return MailDeliveryResult{Outcome: mail.OutcomeDiscarded, Duplicate: true}, nil
	}

	result, err := h.file(ctx, tenantID, message, decided)
	if err != nil {
		return MailDeliveryResult{}, err
	}
	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		recorded, err := h.Receipts.Record(ctx, h.receipt(message, result))
		result.Duplicate = !recorded
		return err
	})
	return result, err
}

// tenantOf is the workspace the message is for: the mailbox's own, or the one an address on the
// operator's domain names.
func (h ReceiveMail) tenantOf(ctx context.Context, message ReceivedMail) (shared.ID, error) {
	if !message.TenantID.IsZero() {
		return message.TenantID, nil
	}
	if h.Domain == "" {
		return "", mailGone()
	}
	for _, recipient := range message.Recipients {
		name, ok := mail.WorkspaceOf(recipient, h.Domain)
		if !ok {
			continue
		}
		var tenantID shared.ID
		err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
			func(ctx context.Context) error {
				found, err := h.Settings.WorkspaceNamed(ctx, name)
				tenantID = found
				return err
			})
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		return tenantID, nil
	}
	return "", mailGone()
}

// decide runs the routing order and the sender level. It reads; file writes.
func (h ReceiveMail) decide(
	ctx context.Context, scope persistence.Scope, settings mail.Settings, message ReceivedMail,
) (route, error) {
	sender := strings.ToLower(strings.TrimSpace(message.Sender))
	switch {
	case message.Automatic:
		return route{discard: true, reason: mail.ReasonAutomatic}, nil
	case settings.Senders.Blocks(sender):
		return route{discard: true, reason: mail.ReasonBlocked}, nil
	}
	sent, err := h.Receipts.CountFrom(ctx, cut(senderWithin(message.Sender), 320), h.Clock.Now().Add(-time.Hour))
	if err != nil {
		return route{}, err
	}
	if sent >= mail.MailsPerSenderPerHour {
		return route{discard: true, reason: mail.ReasonTooMany}, nil
	}

	target, reason, err := h.target(ctx, settings, message, sender)
	if err != nil {
		return route{}, err
	}
	decided := route{target: target, reason: reason}
	switch {
	case message.Unparseable:
		decided.reason = mail.ReasonUnparseable
	case message.Spam:
		decided.reason = mail.ReasonSpam
	}
	if decided.reason != mail.ReasonNone || target.Kind == domain.TargetJumble {
		return decided, nil
	}

	account, member := h.recognise(ctx, message)
	if !settings.Senders.Admits(sender, member) {
		decided.reason = mail.ReasonSenderNotAllowed
		return decided, nil
	}
	if !member {
		if settings.OpenedBy.IsZero() || h.Accounts == nil {
			decided.reason = mail.ReasonSenderNotAllowed
			return decided, nil
		}
		opener, err := h.Accounts.Find(ctx, settings.OpenedBy)
		if err != nil || opener.Status != identity.AccountActive {
			decided.reason = mail.ReasonSenderNotAllowed
			return decided, nil
		}
		account, decided.behalf = opener, true
	}
	decided.actor = actorOf(scope, account)
	return decided, nil
}

// target is the first rule of the routing order that matches (ADR-0094 decision 3).
func (h ReceiveMail) target(
	ctx context.Context, settings mail.Settings, message ReceivedMail, sender string,
) (domain.MailTarget, mail.Reason, error) {
	door := mail.Door{Mailbox: message.Mailbox, Domain: h.Domain, Workspace: settings.Name}
	named := mail.RouteOf(message.Subject, message.Recipients, door)
	for _, key := range named.Keys {
		id, err := h.Places.ItemByKey(ctx, key)
		if err == nil {
			return domain.MailTarget{Kind: domain.TargetItem, ID: id}, mail.ReasonNone, nil
		}
		if !errors.Is(err, shared.ErrNotFound) {
			return domain.MailTarget{}, "", err
		}
	}
	for _, handle := range named.Handles {
		id, err := h.Places.CollectionByHandle(ctx, handle)
		if err == nil {
			return domain.MailTarget{Kind: domain.TargetCollection, ID: id}, mail.ReasonNone, nil
		}
		if !errors.Is(err, shared.ErrNotFound) {
			return domain.MailTarget{}, "", err
		}
	}
	if id, ok := mail.RouteBySender(settings.SenderRoutes, sender); ok {
		return domain.MailTarget{Kind: domain.TargetCollection, ID: id}, mail.ReasonNone, nil
	}
	if !settings.DefaultCollectionID.IsZero() {
		return domain.MailTarget{Kind: domain.TargetCollection, ID: settings.DefaultCollectionID}, mail.ReasonNone, nil
	}
	reason := mail.ReasonNoRoute
	if len(named.Keys) > 0 || len(named.Handles) > 0 {
		reason = mail.ReasonUnknownPlace
	}
	return domain.MailTarget{Kind: domain.TargetJumble}, reason, nil
}

// file performs what was decided: nothing, the jumble, or the act as the admitted sender.
func (h ReceiveMail) file(
	ctx context.Context, tenantID shared.ID, message ReceivedMail, decided route,
) (MailDeliveryResult, error) {
	if decided.discard {
		return MailDeliveryResult{Outcome: mail.OutcomeDiscarded, Reason: decided.reason}, nil
	}
	files := message.Attachments
	if message.Unparseable {
		files = append(files, mediaservice.IngestedFile{FileName: RawMailFileName, Content: message.Raw})
	}
	stored, err := h.ingest(ctx, tenantID, files)
	if err != nil {
		return MailDeliveryResult{}, err
	}
	if decided.actor.AccountID.IsZero() {
		return h.toJumble(ctx, tenantID, message, stored, decided.target, decided.reason)
	}
	result, err := h.write(ctx, decided.actor, decided.target, message, stored, decided.behalf)
	switch {
	case errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound):
		// The sender may not do this there - the jumble waits for somebody who may.
		return h.toJumble(ctx, tenantID, message, stored, decided.target, mail.ReasonNotPermitted)
	case errors.Is(err, shared.ErrValidation) || errors.Is(err, shared.ErrCapabilityNotSupported):
		// The place keeps nothing of the kind - a reply to an entry on a list without comments.
		return h.toJumble(ctx, tenantID, message, stored, decided.target, mail.ReasonRefusedByPlace)
	}
	return result, err
}

// recognise finds the person the sender's address belongs to: an active person account of the
// workspace, and only where the provider did not say the address was forged.
func (h ReceiveMail) recognise(ctx context.Context, message ReceivedMail) (identity.Account, bool) {
	if message.AuthFailed || h.Accounts == nil || !strings.Contains(message.Sender, "@") {
		return identity.Account{}, false
	}
	account, err := h.Accounts.FindByEmail(ctx, strings.ToLower(strings.TrimSpace(message.Sender)))
	if err != nil || account.Kind != identity.AccountUser || account.Status != identity.AccountActive {
		return identity.Account{}, false
	}
	return account, true
}

func actorOf(scope persistence.Scope, account identity.Account) appshared.ActorContext {
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
		// A type's form narrows what arrives by mail (domain-model.md §2, "Forms").
		Channel: string(work.ChannelMail),
	}
}

// write performs the act as the actor, in one transaction: the entry or the comment, and the files.
func (h ReceiveMail) write(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
	message ReceivedMail, files []shared.ID, onBehalf bool,
) (MailDeliveryResult, error) {
	text := domain.CutQuotedReply(message.Body)
	if onBehalf {
		text = fromLine(message) + "\n\n" + text
	}
	var result MailDeliveryResult
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		switch target.Kind {
		case domain.TargetCollection:
			input := usecase.Input{
				"type": "TASK", "title": mailTitle(message), "collection_id": target.ID.String(),
			}
			if strings.TrimSpace(text) != "" {
				input["notes"] = cutBytes(strings.TrimSpace(text), domain.MaxBodyBytes)
			}
			out, err := h.Catalogue.Invoke(ctx, createWorkItemName, actor, input)
			afterwards := ""
			if err != nil && input["notes"] != nil && errors.Is(err, shared.ErrCapabilityNotSupported) {
				// The collection keeps no description - a shopping list: the entry without one, and
				// the mail's text as its first comment where the collection keeps comments.
				afterwards = input.String("notes")
				delete(input, "notes")
				out, err = h.Catalogue.Invoke(ctx, createWorkItemName, actor, input)
			}
			if err != nil {
				return err
			}
			itemID, err := shared.ParseID(out.String("id"))
			if err != nil {
				return shared.ErrInternal.WithDetail("mail.entry_incomplete").WithCause(err)
			}
			result = MailDeliveryResult{Outcome: mail.OutcomeEntry, ItemID: itemID}
			if afterwards != "" {
				if _, err := h.Catalogue.Invoke(ctx, addCommentName, actor, usecase.Input{
					"item_id": itemID.String(), "body": afterwards,
				}); err != nil && !errors.Is(err, shared.ErrCapabilityNotSupported) {
					return err
				}
			}
		default:
			body := strings.TrimSpace(text)
			if body == "" || (onBehalf && strings.TrimSpace(domain.CutQuotedReply(message.Body)) == "") {
				body = strings.TrimSpace(fromLine(message) + "\n\n" + mailTitle(message))
			}
			out, err := h.Catalogue.Invoke(ctx, addCommentName, actor, usecase.Input{
				"item_id": target.ID.String(), "body": cutBytes(body, domain.MaxBodyBytes),
			})
			if err != nil {
				return err
			}
			commentID, _ := shared.ParseID(out.String("id"))
			result = MailDeliveryResult{Outcome: mail.OutcomeComment, ItemID: target.ID, CommentID: commentID}
		}
		for _, file := range files {
			if _, err := h.Catalogue.Invoke(ctx, attachMediaName, actor, usecase.Input{
				"item_id": result.ItemID.String(), "media_id": file.String(),
			}); err != nil {
				if errors.Is(err, shared.ErrCapabilityNotSupported) {
					// A place that keeps no attachments takes the mail without its files; the
					// unattached uploads go with the media reconciliation.
					break
				}
				return err
			}
		}
		if onBehalf && h.Requesters != nil {
			// In the act's transaction: an entry filed for somebody is never without the record
			// that says whose it is and when it goes.
			return h.Requesters.RecordContact(ctx, strings.ToLower(senderWithin(message.Sender)),
				strings.TrimSpace(message.SenderName), result.ItemID, h.Clock.Now())
		}
		return nil
	})
	return result, err
}

// RequesterContacts is the record of requesters without an account.
type RequesterContacts interface {
	RecordContact(ctx context.Context, address, name string, itemID shared.ID, at time.Time) error
}

// toJumble stores the message as a jumble entry, with the place it was meant for and why it is here.
func (h ReceiveMail) toJumble(
	ctx context.Context, tenantID shared.ID, message ReceivedMail, files []shared.ID,
	target domain.MailTarget, reason mail.Reason,
) (MailDeliveryResult, error) {
	body := cutBytes(message.Body, domain.MaxBodyBytes)
	if message.Unparseable {
		body = cutBytes(printable(string(message.Raw)), domain.MaxBodyBytes)
	}
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: h.IDs.NewID(), TenantID: tenantID, Channel: domain.ChannelEmail,
		Sender:       senderWithin(message.Sender),
		RawSubject:   cut(message.Subject, domain.MaxSubjectLength),
		RawBody:      body,
		Attachments:  files,
		MeantFor:     target,
		IntakeReason: string(reason),
		Now:          h.Clock.Now(),
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
	return MailDeliveryResult{Outcome: mail.OutcomeJumble, Reason: reason, EntryID: entry.ID}, nil
}

func (h ReceiveMail) receipt(message ReceivedMail, result MailDeliveryResult) mail.Receipt {
	door := InboundDoor
	if message.Mailbox != "" {
		door = message.Mailbox
	}
	return mail.Receipt{
		ID: h.IDs.NewID(), MessageID: message.MessageID, ReceivedAt: h.Clock.Now(),
		Sender:  cut(senderWithin(message.Sender), 320),
		Subject: cut(message.Subject, domain.MaxSubjectLength),
		Outcome: result.Outcome, Reason: result.Reason,
		ItemID: result.ItemID, EntryID: result.EntryID, Door: door,
	}
}

func (h ReceiveMail) ingest(
	ctx context.Context, tenantID shared.ID, files []mediaservice.IngestedFile,
) ([]shared.ID, error) {
	if h.Media == nil || len(files) == 0 {
		return nil, nil
	}
	return h.Media.Execute(ctx, tenantID, files)
}

func mailGone() error { return shared.ErrNotFound.WithDetail("mail.address_not_found") }

// mailTitle is the subject without its routing, or the text's first line, or the sender - an entry
// needs a title, and none of these puts words in the workspace nobody wrote.
func mailTitle(message ReceivedMail) string {
	if subject := mail.Title(message.Subject); subject != "" {
		return cut(subject, domain.MaxSubjectLength)
	}
	for _, line := range strings.Split(domain.CutQuotedReply(message.Body), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return cut(line, domain.MaxSubjectLength)
		}
	}
	return cut(fromLine(message), domain.MaxSubjectLength)
}

// fromLine names the sender above what an account writes on their behalf.
func fromLine(message ReceivedMail) string {
	sender := senderWithin(message.Sender)
	if name := strings.TrimSpace(message.SenderName); name != "" {
		return "From: " + name + " <" + sender + ">"
	}
	return "From: " + sender
}
