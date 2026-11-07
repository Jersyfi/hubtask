// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strings"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// RequesterRepliesConsumer identifies the subscriber; stable, because renaming it makes every
// event it has seen look new.
const RequesterRepliesConsumer = "servicedesk-requester-replies"

// The message codes of a reply mailed to a requester (UC-SVC-12): the subject carries the ticket
// key, which routes their answer back to the entry (UC-JUM-12 check 2).
const (
	requesterReplySubject = "email.requester_reply.subject"
	requesterReplyBody    = "email.requester_reply.body"
)

// RequesterAddresses answers who an entry was filed for by mail - the requester repository.
type RequesterAddresses interface {
	AddressesOn(ctx context.Context, itemID shared.ID) ([]domain.RequesterAddress, error)
}

// MergeLinks answers an entry's links from both ends - workrepo.EntryLinks.Of.
type MergeLinks interface {
	Of(ctx context.Context, entryID shared.ID, limit int) ([]work.EntryLink, error)
}

// maxLinksForMerges bounds the links read to find the requests merged into an entry.
const maxLinksForMerges = 500

// addressesWithMerged answers the requesters by mail of an entry and of every request merged into
// it (UC-SVC-03 check 8), each address once; nil links reads the entry's own alone.
func addressesWithMerged(
	ctx context.Context, requesters RequesterAddresses, links MergeLinks, itemID shared.ID,
) ([]domain.RequesterAddress, error) {
	addresses, err := requesters.AddressesOn(ctx, itemID)
	if err != nil || links == nil {
		return addresses, err
	}
	touching, err := links.Of(ctx, itemID, maxLinksForMerges)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(addresses))
	for _, address := range addresses {
		seen[strings.ToLower(address.Address)] = true
	}
	for _, source := range work.MergedInto(touching, itemID) {
		merged, err := requesters.AddressesOn(ctx, source)
		if err != nil {
			return nil, err
		}
		for _, address := range merged {
			if key := strings.ToLower(address.Address); !seen[key] {
				seen[key] = true
				addresses = append(addresses, address)
			}
		}
	}
	return addresses, nil
}

// ReplyJobs queues the mail of one reply.
type ReplyJobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// RequesterReplies queues the mail of a public reply on an entry filed for a requester without an
// account (UC-SVC-12 check 1). One job per comment, keyed by it, so an event delivered twice
// queues one mail; the mail itself is sent outside the dispatcher's transaction. It never takes a
// replay: a restore's comments were answered when they were written.
type RequesterReplies struct {
	Requesters RequesterAddresses
	Jobs       ReplyJobs
	// Links finds the requests merged into the entry, whose requesters are mailed too; nil mails
	// the entry's own alone.
	Links MergeLinks
}

var _ eventbus.Subscriber = RequesterReplies{}

// Name identifies the subscriber.
func (r RequesterReplies) Name() string { return RequesterRepliesConsumer }

// Wants reports whether an event can be a reply.
func (r RequesterReplies) Wants(eventType event.Type) bool { return eventType == event.CommentCreated }

// Deliver queues the reply's mail where the entry has requesters by mail.
func (r RequesterReplies) Deliver(ctx context.Context, envelope event.Envelope) error {
	if envelope.Type != event.CommentCreated || envelope.Payload["internal"] == true ||
		envelope.Payload["by_requester"] == true {
		return nil
	}
	itemID := shared.ID(textAt(envelope.Payload, "item_id"))
	commentID := shared.ID(textAt(envelope.Payload, "id"))
	if itemID.IsZero() || commentID.IsZero() {
		return nil
	}
	addresses, err := addressesWithMerged(ctx, r.Requesters, r.Links, itemID)
	if err != nil || len(addresses) == 0 {
		// The common case: an entry nobody filed by mail, and none merged into it filed so.
		return err
	}
	_, err = r.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindRequesterReply, TenantID: envelope.TenantID, DedupeKey: commentID.String(),
		// The comment's identifier alone: the addresses and the words are read when it is sent
		// (rule 10), so an erased requester is mailed nothing.
		Payload: map[string]any{"comment_id": commentID.String()},
	})
	return err
}

// TicketKeys answers an entry's ticket key - the mail intake's places.
type TicketKeys interface {
	KeyOf(ctx context.Context, itemID shared.ID) (maildomain.Key, error)
}

// ReplyAddress answers the address an answer to the entry goes to; empty for none.
type ReplyAddress interface {
	ReplyAddress(ctx context.Context, tenantID, itemID shared.ID) (string, error)
}

// WorkspaceLanguage answers the workspace's own default language.
type WorkspaceLanguage interface {
	Find(ctx context.Context) (identity.Workspace, error)
}

// SendRequesterReply mails one public reply to the requesters an entry was filed for by mail
// (UC-SVC-12 checks 1 to 3): one mail per address, so no requester learns another's; in the
// workspace's language, since a requester without an account chose none; the subject with the
// ticket key and the Reply-To the entry's address, so the answer becomes a comment there.
//
// Read in one transaction, sent outside any. Safe to run twice: a retry after a send whose outcome
// was lost mails again, at-least-once as every notification is.
type SendRequesterReply struct {
	Comments   workrepo.Comments
	Items      workrepo.Items
	Requesters RequesterAddresses
	// Links finds the requests merged into the entry (UC-SVC-03 check 8); nil mails the entry's
	// own requesters alone.
	Links MergeLinks
	Keys  TicketKeys
	// Accounts names who wrote the reply; nil leaves the name out.
	Accounts identityrepo.Accounts
	// Replies answers the entry's address; nil sends no Reply-To.
	Replies ReplyAddress
	// Workspaces answers the workspace's language; nil uses FallbackLocale.
	Workspaces     WorkspaceLanguage
	FallbackLocale string
	Mail           mail.Sender
	Renderer       i18n.Renderer
	Background     persistence.UnitOfWork
}

type replyRead struct {
	comment   work.Comment
	item      work.WorkItem
	key       maildomain.Key
	to        []domain.RequesterAddress
	author    string
	locale    string
	replyTo   string
	sendsNone bool
}

// Send mails the reply the job names. Nothing to send - the comment, entry or requesters gone,
// the comment no longer a public reply - is done, not failed.
func (s SendRequesterReply) Send(ctx context.Context, tenantID, commentID shared.ID) error {
	read, err := s.read(ctx, tenantID, commentID)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && read.sendsNone) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.Replies != nil {
		if read.replyTo, err = s.Replies.ReplyAddress(ctx, tenantID, read.item.ID); err != nil {
			// The Reply-To is a convenience; the ticket key in the subject routes the answer too.
			read.replyTo = ""
		}
	}
	params := map[string]string{
		"key": read.key.String(), "title": oneLine(read.item.Title), "author": read.author, "reply": read.comment.Body,
	}
	subject := s.Renderer.Render(read.locale, requesterReplySubject, params)
	body := s.Renderer.Render(read.locale, requesterReplyBody, params)
	for _, to := range read.to {
		if err := s.Mail.Send(ctx, mail.Message{To: to.Address, Subject: subject, Body: body, ReplyTo: read.replyTo}); err != nil {
			return err
		}
	}
	return nil
}

func (s SendRequesterReply) read(ctx context.Context, tenantID, commentID shared.ID) (replyRead, error) {
	var read replyRead
	err := s.Background.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		var err error
		if read.comment, err = s.Comments.Find(ctx, commentID); err != nil {
			return err
		}
		// Asked again at sending: a comment deleted while its mail waited is not mailed. The
		// requester's own words never reach here, the subscriber leaves them out.
		if !domain.MailsRequesters(read.comment, false) {
			read.sendsNone = true
			return nil
		}
		if read.item, err = s.Items.Find(ctx, read.comment.ItemID); err != nil {
			return err
		}
		if read.item.DeletedAt != nil {
			read.sendsNone = true
			return nil
		}
		if read.to, err = addressesWithMerged(ctx, s.Requesters, s.Links, read.item.ID); err != nil {
			return err
		}
		if len(read.to) == 0 {
			read.sendsNone = true
			return nil
		}
		if read.key, err = s.Keys.KeyOf(ctx, read.item.ID); err != nil {
			return err
		}
		if s.Accounts != nil {
			author, err := s.Accounts.Find(ctx, read.comment.AuthorID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			read.author = author.DisplayName
		}
		read.locale = s.FallbackLocale
		if s.Workspaces != nil {
			workspace, err := s.Workspaces.Find(ctx)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			if workspace.DefaultLocale != "" {
				read.locale = workspace.DefaultLocale
			}
		}
		return nil
	})
	return read, err
}

// oneLine keeps a title to the subject's one line; the mail adapter refuses a line break there.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
