// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"strings"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	maildomain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var replyID = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f6")

type requesterBook map[shared.ID][]domain.RequesterAddress

func (b requesterBook) AddressesOn(_ context.Context, item shared.ID) ([]domain.RequesterAddress, error) {
	return b[item], nil
}

type queued struct{ requests []queue.Request }

func (q *queued) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	for _, r := range q.requests {
		if r.Kind == request.Kind && r.DedupeKey == request.DedupeKey {
			return shared.ID(""), nil
		}
	}
	q.requests = append(q.requests, request)
	return shared.ID(""), nil
}

func commentEvent(payload map[string]any) event.Envelope {
	payload["id"], payload["item_id"] = replyID.String(), itemID.String()
	return event.Envelope{Type: event.CommentCreated, TenantID: tenantID, Payload: payload}
}

// UC-SVC-12 check 1: a public reply on an entry filed by mail queues one mail per comment; an
// internal note, the requester's own words and an entry nobody filed by mail queue none.
func TestAPublicReplyOnAMailedRequestQueuesItsMailOnce(t *testing.T) {
	book := requesterBook{itemID: {{Address: "kunde@shop.test", Name: "Kunde"}}}
	jobs := &queued{}
	replies := RequesterReplies{Requesters: book, Jobs: jobs}

	for _, payload := range []map[string]any{
		{"internal": true, "author_id": handlerID.String()},
		{"by_requester": true, "author_id": handlerID.String()},
	} {
		if err := replies.Deliver(t.Context(), commentEvent(payload)); err != nil {
			t.Fatal(err)
		}
	}
	if len(jobs.requests) != 0 {
		t.Fatalf("an internal note or the requester's own mail queued %+v", jobs.requests)
	}
	for range 2 {
		if err := replies.Deliver(t.Context(), commentEvent(map[string]any{"author_id": handlerID.String()})); err != nil {
			t.Fatal(err)
		}
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindRequesterReply ||
		jobs.requests[0].Payload["comment_id"] != replyID.String() || len(jobs.requests[0].Payload) != 1 {
		t.Fatalf("queued %+v, want one job naming the comment alone", jobs.requests)
	}

	none := RequesterReplies{Requesters: requesterBook{}, Jobs: &queued{}}
	if err := none.Deliver(t.Context(), commentEvent(map[string]any{})); err != nil {
		t.Fatal(err)
	}
	if len(none.Jobs.(*queued).requests) != 0 {
		t.Error("an entry nobody filed by mail queued a mail")
	}
}

type oneComment struct {
	workrepo.Comments
	comment work.Comment
}

func (c oneComment) Find(_ context.Context, id shared.ID) (work.Comment, error) {
	if id != c.comment.ID {
		return work.Comment{}, shared.ErrNotFound
	}
	return c.comment, nil
}

type keys struct{}

func (keys) KeyOf(context.Context, shared.ID) (maildomain.Key, error) {
	return maildomain.Key{Handle: "it", Number: 42}, nil
}

type entryAddress struct{}

func (entryAddress) ReplyAddress(context.Context, shared.ID, shared.ID) (string, error) {
	return "acme+it-42@in.example.test", nil
}

type german struct{}

func (german) Find(context.Context) (identity.Workspace, error) {
	return identity.Workspace{DefaultLocale: "de"}, nil
}

type mailbag struct{ sent []mail.Message }

func (o *mailbag) Send(_ context.Context, m mail.Message) error {
	o.sent = append(o.sent, m)
	return nil
}

// echo renders a code as the locale, the code and its parameters, so a test reads what went in.
type echo struct{}

func (echo) Render(locale, code string, params map[string]string) string {
	return locale + "|" + code + "|" + params["key"] + "|" + params["title"] + "|" + params["reply"]
}

func sender(comment work.Comment, book requesterBook, sent *mailbag) SendRequesterReply {
	return SendRequesterReply{
		Comments: oneComment{comment: comment},
		Items: &itemStore{item: work.WorkItem{
			ID: itemID, TenantID: tenantID, CollectionID: collection, Title: "Printer\nprints blank", CreatedBy: requesterID,
		}},
		Requesters: book, Keys: keys{}, Replies: entryAddress{}, Workspaces: german{}, FallbackLocale: "en",
		Mail: sent, Renderer: echo{}, Background: inline{},
	}
}

// UC-SVC-12 checks 2 and 3: one mail per requester, in the workspace's language, the ticket key
// in the subject and the entry's address as Reply-To; a comment that became no public reply while
// its mail waited is mailed to nobody, and an internal note never is.
func TestTheReplyIsMailedToEachRequesterWithItsTicketKey(t *testing.T) {
	book := requesterBook{itemID: {{Address: "a@shop.test"}, {Address: "b@shop.test"}}}
	reply := work.Comment{ID: replyID, ItemID: itemID, AuthorID: handlerID, Body: "Replaced the toner.", Kind: work.CommentByUser}
	sent := &mailbag{}
	if err := sender(reply, book, sent).Send(t.Context(), tenantID, replyID); err != nil {
		t.Fatal(err)
	}
	if len(sent.sent) != 2 || sent.sent[0].To != "a@shop.test" || sent.sent[1].To != "b@shop.test" {
		t.Fatalf("sent %+v, want one mail to each requester", sent.sent)
	}
	got := sent.sent[0]
	if got.Subject != "de|email.requester_reply.subject|IT-42|Printer prints blank|Replaced the toner." ||
		got.ReplyTo != "acme+it-42@in.example.test" || !strings.Contains(got.Body, "Replaced the toner.") {
		t.Errorf("the mail is %+v", got)
	}

	gone := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	for _, comment := range []work.Comment{
		{ID: replyID, ItemID: itemID, AuthorID: handlerID, Body: "Ask Lena.", Internal: true},
		{ID: replyID, ItemID: itemID, AuthorID: handlerID, DeletedAt: &gone},
	} {
		sent := &mailbag{}
		if err := sender(comment, book, sent).Send(t.Context(), tenantID, replyID); err != nil {
			t.Fatal(err)
		}
		if len(sent.sent) != 0 {
			t.Errorf("%+v was mailed: %+v", comment, sent.sent)
		}
	}
	erased := &mailbag{}
	if err := sender(reply, requesterBook{}, erased).Send(t.Context(), tenantID, replyID); err != nil || len(erased.sent) != 0 {
		t.Errorf("an erased requester was mailed %+v (%v)", erased.sent, err)
	}
	trashed := &mailbag{}
	inTrash := sender(reply, book, trashed)
	inTrash.Items = &itemStore{item: work.WorkItem{ID: itemID, TenantID: tenantID, DeletedAt: &gone}}
	if err := inTrash.Send(t.Context(), tenantID, replyID); err != nil || len(trashed.sent) != 0 {
		t.Errorf("an entry in the trash was mailed %+v (%v)", trashed.sent, err)
	}
	missing := &mailbag{}
	if err := sender(reply, book, missing).Send(t.Context(), tenantID, itemID); err != nil || len(missing.sent) != 0 {
		t.Errorf("a comment that is gone sent %+v (%v)", missing.sent, err)
	}
}
