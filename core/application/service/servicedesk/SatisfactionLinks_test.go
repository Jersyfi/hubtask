// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// linkStore keeps links by the token's whole secret, standing in for its keyed hash.
type linkStore struct {
	rows map[string]domain.SatisfactionLink
}

func (s *linkStore) Put(_ context.Context, link domain.SatisfactionLink, token integration.SatisfactionLinkToken) error {
	for secret, held := range s.rows {
		if held.RequesterID == link.RequesterID && held.ItemID == link.ItemID {
			delete(s.rows, secret)
		}
	}
	s.rows[token.Secret()] = link
	return nil
}

func (s *linkStore) FindByToken(_ context.Context, token integration.SatisfactionLinkToken) (domain.SatisfactionLink, error) {
	link, ok := s.rows[token.Secret()]
	if !ok || link.TenantID != token.TenantID() {
		return domain.SatisfactionLink{}, shared.ErrNotFound
	}
	return link, nil
}

func (s *linkStore) Use(_ context.Context, token integration.SatisfactionLinkToken, now time.Time) (bool, error) {
	link, ok := s.rows[token.Secret()]
	if !ok || link.UsedAt != nil || !now.Before(link.ExpiresAt) {
		return false, nil
	}
	link.UsedAt = &now
	s.rows[token.Secret()] = link
	return true, nil
}

type base string

func (b base) Of(context.Context) (string, error) { return string(b), nil }

type trailOf struct{ entries []audit.Entry }

func (t *trailOf) Append(_ context.Context, e audit.Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	t.entries = append(t.entries, e)
	return nil
}

// linksRender renders the body as its five links, one per line, so a test reads them back.
type linksRender struct{}

func (linksRender) Render(locale, code string, params map[string]string) string {
	if code == satisfactionQuestionSubject {
		return locale + "|" + params["key"] + "|" + params["title"]
	}
	return strings.Join([]string{params["link_1"], params["link_2"], params["link_3"], params["link_4"], params["link_5"]}, "\n")
}

var (
	kayaID  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000b01")
	lindaID = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000b02")
)

func resolvedEntry() *itemStore {
	return &itemStore{item: work.WorkItem{
		ID: itemID, TenantID: tenantID, CollectionID: collection, Title: "Drucker\ndruckt leer",
		Completion: work.Completion{IsCompleted: true},
	}}
}

func questionSender(items *itemStore, book requesterBook, links *linkStore, sent *mailbag) SendSatisfactionQuestion {
	return SendSatisfactionQuestion{
		Items: items, Requesters: book, Keys: keys{}, Links: links, Address: base("https://acme.hub.example/"),
		Replies: entryAddress{}, Workspaces: german{}, FallbackLocale: "en", Entropy: &fixedEntropy{},
		Clock: at(monday), Mail: sent, Renderer: linksRender{}, Background: inline{},
	}
}

// tokenIn reads the token and score back out of a mailed link.
func tokenIn(t *testing.T, link string) (integration.SatisfactionLinkToken, string) {
	t.Helper()
	_, fragment, found := strings.Cut(link, "/rate#")
	raw, score, dotted := strings.Cut(fragment, ".")
	if !found || !dotted {
		t.Fatalf("not a rating link: %q", link)
	}
	token, err := integration.ParseSatisfactionLinkToken(raw)
	if err != nil {
		t.Fatal(err)
	}
	return token, score
}

// UC-SVC-07 check 6: a completion of an entry filed for requesters by mail queues one question per
// completion, however often its event arrives; an entry nobody filed by mail queues none.
func TestAResolutionOfAMailedRequestQueuesItsQuestionOnce(t *testing.T) {
	completed := event.Envelope{
		ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000b10"), Type: event.ItemCompleted,
		TenantID: tenantID, Payload: map[string]any{"id": itemID.String(), "title": "Drucker"},
	}
	jobs := &queued{}
	questions := SatisfactionQuestions{Requesters: requesterBook{itemID: {{ID: kayaID, Address: "kaya@shop.test"}}}, Jobs: jobs}
	for range 2 {
		if err := questions.Deliver(t.Context(), completed); err != nil {
			t.Fatal(err)
		}
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindRequesterSatisfaction ||
		jobs.requests[0].Payload["item_id"] != itemID.String() || len(jobs.requests[0].Payload) != 1 {
		t.Fatalf("queued %+v, want one job naming the entry alone", jobs.requests)
	}
	if !questions.Wants(event.ItemCompleted) || questions.Wants(event.CommentCreated) {
		t.Error("the subscriber wants the wrong events")
	}
	none := SatisfactionQuestions{Requesters: requesterBook{}, Jobs: &queued{}}
	if err := none.Deliver(t.Context(), completed); err != nil || len(none.Jobs.(*queued).requests) != 0 {
		t.Errorf("an entry nobody filed by mail queued a question (%v)", err)
	}
}

// UC-SVC-07 check 6: each requester gets their own mail with five links of their own, in the
// workspace's language, the ticket key in the subject; an entry reopened or trashed meanwhile, or
// a workspace without an address, mails nothing.
func TestTheQuestionIsMailedToEachRequesterWithLinksOfTheirOwn(t *testing.T) {
	book := requesterBook{itemID: {{ID: kayaID, Address: "kaya@shop.test"}, {ID: lindaID, Address: "linda@shop.test"}}}
	links := &linkStore{rows: map[string]domain.SatisfactionLink{}}
	sent := &mailbag{}
	if err := questionSender(resolvedEntry(), book, links, sent).Send(t.Context(), tenantID, itemID); err != nil {
		t.Fatal(err)
	}
	if len(sent.sent) != 2 || sent.sent[0].To != "kaya@shop.test" || sent.sent[1].To != "linda@shop.test" {
		t.Fatalf("sent %+v, want one mail to each requester", sent.sent)
	}
	if sent.sent[0].Subject != "de|IT-42|Drucker druckt leer" || sent.sent[0].ReplyTo != "acme+it-42@in.example.test" {
		t.Errorf("the mail is %+v", sent.sent[0])
	}
	kaya, _ := tokenIn(t, strings.Split(sent.sent[0].Body, "\n")[0])
	linda, _ := tokenIn(t, strings.Split(sent.sent[1].Body, "\n")[0])
	if kaya.Secret() == linda.Secret() || len(links.rows) != 2 {
		t.Fatalf("the requesters share a link: %d links", len(links.rows))
	}
	for i, line := range strings.Split(sent.sent[0].Body, "\n") {
		token, score := tokenIn(t, line)
		if !strings.HasPrefix(line, "https://acme.hub.example/rate#") || token.Secret() != kaya.Secret() || score != string(rune('1'+i)) {
			t.Errorf("link %d is %q", i+1, line)
		}
	}
	if held := links.rows[kaya.Secret()]; held.RequesterID != kayaID || held.ItemID != itemID || !held.ExpiresAt.Equal(monday.Add(domain.SatisfactionLinkLife)) {
		t.Errorf("the link kept is %+v", held)
	}

	gone := monday
	reopened, trashed := resolvedEntry(), resolvedEntry()
	reopened.item.Completion.IsCompleted = false
	trashed.item.DeletedAt = &gone
	for name, items := range map[string]*itemStore{"reopened": reopened, "trashed": trashed} {
		sent := &mailbag{}
		if err := questionSender(items, book, &linkStore{rows: map[string]domain.SatisfactionLink{}}, sent).Send(t.Context(), tenantID, itemID); err != nil || len(sent.sent) != 0 {
			t.Errorf("an entry %s was asked about: %+v (%v)", name, sent.sent, err)
		}
	}
	nowhere := questionSender(resolvedEntry(), book, &linkStore{rows: map[string]domain.SatisfactionLink{}}, &mailbag{})
	nowhere.Address = base("")
	if err := nowhere.Send(t.Context(), tenantID, itemID); err != nil || len(nowhere.Mail.(*mailbag).sent) != 0 {
		t.Errorf("a workspace without an address mailed a link (%v)", err)
	}
}

// UC-SVC-07 check 6: the link records the request's score once, within its life; a second answer,
// an expired link, a replaced one, a reopened request and another workspace's token are refused,
// and a score off the scale spends nothing.
func TestASatisfactionLinkRatesTheRequestOnce(t *testing.T) {
	book := requesterBook{itemID: {{ID: kayaID, Address: "kaya@shop.test"}}}
	links := &linkStore{rows: map[string]domain.SatisfactionLink{}}
	items := resolvedEntry()
	sent := &mailbag{}
	if err := questionSender(items, book, links, sent).Send(t.Context(), tenantID, itemID); err != nil {
		t.Fatal(err)
	}
	token, _ := tokenIn(t, strings.Split(sent.sent[0].Body, "\n")[0])
	ratings := &ratingStore{rows: map[shared.ID]domain.Rating{}}
	trail := &trailOf{}
	answer := func(when time.Time) AnswerSatisfactionLink {
		return AnswerSatisfactionLink{
			Links: links, Items: items, Ratings: ratings, Audit: trail, IDs: &seqIDs{}, Clock: at(when), UnitOfWork: inline{},
		}
	}
	detail := func(err error) string {
		var typed *shared.Error
		if errors.As(err, &typed) {
			return typed.DetailCode
		}
		return ""
	}

	if err := answer(monday.Add(time.Hour)).Execute(t.Context(), token, 9); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a score off the scale: %v", err)
	}
	items.item.Completion.IsCompleted = false
	if err := answer(monday.Add(time.Hour)).Execute(t.Context(), token, 2); detail(err) != "satisfaction.request_open" {
		t.Fatalf("a reopened request: %v", err)
	}
	items.item.Completion.IsCompleted = true
	if err := answer(monday.Add(time.Hour)).Execute(t.Context(), token, 2); err != nil {
		t.Fatal(err)
	}
	if rating := ratings.rows[itemID]; rating.Score != 2 {
		t.Errorf("the rating is %+v", rating)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != RequestRatedByLinkAction || trail.entries[0].ActorID != "" {
		t.Errorf("the trail is %+v", trail.entries)
	}
	if err := answer(monday.Add(2*time.Hour)).Execute(t.Context(), token, 5); detail(err) != "satisfaction.link_used" {
		t.Errorf("a second answer: %v", err)
	}
	if ratings.rows[itemID].Score != 2 {
		t.Error("a second answer replaced the score")
	}

	sent = &mailbag{}
	again := questionSender(items, book, links, sent)
	again.Entropy = &fixedEntropy{n: 100}
	if err := again.Send(t.Context(), tenantID, itemID); err != nil {
		t.Fatal(err)
	}
	fresh, _ := tokenIn(t, strings.Split(sent.sent[0].Body, "\n")[0])
	if err := answer(monday.Add(time.Hour)).Execute(t.Context(), token, 4); detail(err) != "satisfaction.link_not_found" {
		t.Errorf("a replaced link: %v", err)
	}
	if err := answer(monday.Add(domain.SatisfactionLinkLife)).Execute(t.Context(), fresh, 4); detail(err) != "satisfaction.link_expired" {
		t.Errorf("an expired link: %v", err)
	}
	elsewhere, err := integration.NewSatisfactionLinkToken(shared.MustParseID("01936f2a-7c1e-7000-8000-000000000bff"), []byte(strings.Repeat("x", integration.SatisfactionLinkTokenSecretBytes)))
	if err != nil {
		t.Fatal(err)
	}
	if err := answer(monday.Add(time.Hour)).Execute(t.Context(), elsewhere, 4); detail(err) != "satisfaction.link_not_found" {
		t.Errorf("another workspace's token: %v", err)
	}
}
