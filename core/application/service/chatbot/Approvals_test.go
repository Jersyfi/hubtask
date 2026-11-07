// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	roomport "github.com/Jersyfi/hubtask/core/port/chatbot"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var waitingApproval = shared.ID("0190a1b2-c3d4-7e5f-8a9b-0c1d099a8057")

type link struct {
	integration.ChatLink
	code    []byte
	expires time.Time
}

type links struct{ rows []*link }

func (l *links) Start(_ context.Context, id, botID, accountID shared.ID, code []byte, expires, _ time.Time) error {
	for _, r := range l.rows {
		if r.BotID == botID && r.AccountID == accountID {
			r.code, r.expires = code, expires
			return nil
		}
	}
	l.rows = append(l.rows, &link{ChatLink: integration.ChatLink{ID: id, BotID: botID, AccountID: accountID}, code: code, expires: expires})
	return nil
}
func (l *links) Claim(_ context.Context, botID shared.ID, code []byte, user, address string, at time.Time) (shared.ID, error) {
	for _, r := range l.rows {
		if r.BotID == botID && r.code != nil && bytes.Equal(r.code, code) && r.expires.After(at) {
			for _, other := range l.rows {
				if other.ChatUser == user {
					other.ChatUser, other.Address = "", ""
				}
			}
			r.code, r.ChatUser, r.Address = nil, user, address
			return r.AccountID, nil
		}
	}
	return "", shared.ErrNotFound
}
func (l *links) ByUser(_ context.Context, botID shared.ID, user string) (integration.ChatLink, error) {
	for _, r := range l.rows {
		if r.BotID == botID && r.ChatUser == user && user != "" {
			return r.ChatLink, nil
		}
	}
	return integration.ChatLink{}, shared.ErrNotFound
}
func (l *links) OfAccount(_ context.Context, account shared.ID) ([]integration.ChatLink, error) {
	var out []integration.ChatLink
	for _, r := range l.rows {
		if r.AccountID == account && r.ChatUser != "" {
			out = append(out, r.ChatLink)
		}
	}
	return out, nil
}
func (l *links) Find(_ context.Context, id shared.ID) (integration.ChatLink, error) {
	for _, r := range l.rows {
		if r.ID == id && r.ChatUser != "" {
			return r.ChatLink, nil
		}
	}
	return integration.ChatLink{}, shared.ErrNotFound
}
func (l *links) Delete(_ context.Context, botID, account shared.ID) error {
	for i, r := range l.rows {
		if r.BotID == botID && r.AccountID == account {
			l.rows = append(l.rows[:i], l.rows[i+1:]...)
			return nil
		}
	}
	return shared.ErrNotFound
}

// desk stands in for the approvals: one waits for Anna's yes; a decline needs a reason.
type desk struct {
	*invoker
	decided []string
}

func (d *desk) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	switch name {
	case listApprovals:
		rows := []usecase.Output{}
		if actor.AccountID == anna && len(d.decided) == 0 {
			rows = append(rows, usecase.Output{"id": waitingApproval.String(), "item_title": "Laptop for Jonas"})
		}
		return usecase.Output{"data": rows}, nil
	case decideApproval:
		if actor.AccountID != anna || actor.Kind != appshared.ActorUser {
			return nil, shared.ErrNotFound.WithDetail("approvals.not_found")
		}
		if in.String("verdict") == string(servicedesk.VerdictDecline) && in.String("reason") == "" {
			return nil, shared.ErrValidation.WithDetail("approvals.reason_required")
		}
		d.decided = append(d.decided, in.String("approval_id")+":"+in.String("verdict")+":"+in.String("reason"))
		return usecase.Output{}, nil
	}
	return d.invoker.Invoke(ctx, name, actor, in)
}

func linking(f *fixture) (*links, *desk) {
	l, d := &links{}, &desk{invoker: f.invoker}
	f.bots.Links, f.bots.Entropy, f.bots.Invoker = l, clock.FixedEntropy{Seed: 1}, d
	return l, d
}

func code(t *testing.T, f fixture, who shared.ID, bot shared.ID) string {
	t.Helper()
	out, err := LinkChatAccount{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(who), usecase.Input{"bot_id": bot.String()})
	if err != nil {
		t.Fatal(err)
	}
	if out.String("command") != "!link "+out.String("code") {
		t.Fatalf("out = %v", out)
	}
	return out.String("code")
}

func say(t *testing.T, f fixture, p *platform, bot shared.ID, sender, text string) string {
	t.Helper()
	p.asks = roomport.Inbound{Asks: roomport.AsksCapture, Room: "D0ANNA", Text: text, Bare: true, Sender: sender, Address: sender}
	before := len(p.answered)
	if _, err := deliver(f, bot, "sig"); err != nil {
		t.Fatal(err)
	}
	if len(p.answered) != before+1 || len(p.answered[before]) != 1 {
		t.Fatalf("answered = %v", p.answered)
	}
	return p.answered[before][0].Headline
}

// UC-SVC-09 check 6: a person links their chat account by a code they say to the bot - in its
// conversation with them, another room than the bot's -, approves and declines there as
// themselves through DecideApproval, a decline without a reason refused as in the app; somebody
// unlinked, or linked and no approver, decides nothing.
func TestALinkedPersonDecidesTheirApprovalFromChat(t *testing.T) {
	f, p, bot := slackFixture(t)
	_, d := linking(&f)
	ref := integration.ApprovalRef(waitingApproval)

	if got := say(t, f, p, bot, "U0ANNA", "approve "+ref); got != ":chat.bot_not_linked:" {
		t.Fatalf("unlinked = %q", got)
	}
	if got := say(t, f, p, bot, "U0ANNA", "link AAAAAAAA"); got != ":chat.bot_link_refused:" {
		t.Fatalf("a wrong code = %q", got)
	}
	c := code(t, f, anna, bot)
	if got := say(t, f, p, bot, "U0ANNA", "link "+strings.ToLower(c)); got != ":chat.bot_linked:" {
		t.Fatalf("link = %q", got)
	}
	if got := say(t, f, p, bot, "U0ANNA", "link "+c); got != ":chat.bot_link_refused:" {
		t.Fatalf("a spent code = %q", got)
	}
	if got := say(t, f, p, bot, "U0ANNA", "decline "+ref); got != ":chat.bot_reason_required:"+ref {
		t.Fatalf("a decline without reason = %q", got)
	}
	if got := say(t, f, p, bot, "U0ANNA", "approve 00000000"); got != ":chat.bot_approval_not_found:00000000" {
		t.Fatalf("another reference = %q", got)
	}
	// Bert links too, but nothing waits for his yes: he is told so, and nothing is decided.
	if got := say(t, f, p, bot, "U0BERT", "link "+code(t, f, bert, bot)); got != ":chat.bot_linked:" {
		t.Fatalf("Bert's link = %q", got)
	}
	if got := say(t, f, p, bot, "U0BERT", "approve "+ref); got != ":chat.bot_approval_not_found:"+ref || len(d.decided) != 0 {
		t.Fatalf("a non-approver = %q, %v", got, d.decided)
	}
	if got := say(t, f, p, bot, "U0ANNA", "!decline "+ref+" Over this quarter's budget."); got != ":chat.bot_declined:"+ref {
		t.Fatalf("decline = %q", got)
	}
	if len(d.decided) != 1 || d.decided[0] != waitingApproval.String()+":DECLINE:Over this quarter's budget." {
		t.Fatalf("decided = %v", d.decided)
	}
	// A title that merely starts with the word is still captured.
	p.asks = roomport.Inbound{Asks: roomport.AsksCapture, Room: "C0123ABCD", Text: "approve the budget", Bare: true, Sender: "U0ANNA", Address: "U0ANNA"}
	if _, err := deliver(f, bot, "sig"); err != nil || !strings.Contains(strings.Join(f.invoker.calls, ","), createEntry) {
		t.Fatalf("capture = %v %v", err, f.invoker.calls)
	}
	out, err := ListChatBots{f.bots}.Descriptor().Handler.Invoke(context.Background(), actorOf(anna), usecase.Input{"hub_id": hub.String()})
	if err != nil || out["data"].([]usecase.Output)[0]["linked"] != true {
		t.Fatalf("list = %v %v", out, err)
	}
}

// UC-SVC-09 check 6: a level that begins counting a linked person queues a card for each of their
// chat accounts - not for whoever asked - and the card names the entry's title and the reference
// where only they read it; a Matrix room, which others share, gets the reference alone.
func TestAnApprovalWaitingIsPostedToTheLinkedApprover(t *testing.T) {
	f, p, bot := slackFixture(t)
	l, _ := linking(&f)
	_ = say(t, f, p, bot, "U0ANNA", "link "+code(t, f, anna, bot))
	cards := ApprovalCards{Links: l, Jobs: f.jobs}
	approval := servicedesk.Approval{ID: waitingApproval, TenantID: tenant}
	before := len(f.jobs.requests)
	if err := cards.ApprovalAsked(context.Background(), approval, []shared.ID{anna, bert}, bert); err != nil {
		t.Fatal(err)
	}
	if err := cards.ApprovalAsked(context.Background(), approval, []shared.ID{anna}, anna); err != nil {
		t.Fatal(err)
	}
	queued := f.jobs.requests[before:]
	if len(queued) != 1 || queued[0].Kind != queue.KindChatApprovalCard || queued[0].Payload["approval_id"] != waitingApproval.String() {
		t.Fatalf("queued = %+v", queued)
	}
	linkID := shared.ID(queued[0].Payload["link_id"].(string))
	if err := f.bots.TellApproval(context.Background(), tenant, linkID, waitingApproval, "k"); err != nil {
		t.Fatal(err)
	}
	ref := integration.ApprovalRef(waitingApproval)
	if len(p.told) != 1 || p.told[0] != "U0ANNA|:chat.bot_approval_waiting:"+ref+"|Laptop for Jonas" {
		t.Fatalf("told = %v", p.told)
	}

	m := newFixture()
	ml, _ := linking(&m)
	seated, _ := seat(t, m, actorOf(anna))
	matrix := shared.ID(seated["id"].(string))
	if _, err := m.bots.Run(context.Background(), tenant, matrix); err != nil {
		t.Fatal(err)
	}
	m.room.messages = []roomport.Message{{EventID: "$9", Sender: "@anna:example.org", Body: "!link " + code(t, m, anna, matrix)}}
	if _, err := m.bots.Run(context.Background(), tenant, matrix); err != nil {
		t.Fatal(err)
	}
	if len(m.room.posted) != 1 || m.room.posted[0] != "hubtask-$9|de:chat.bot_linked:" {
		t.Fatalf("matrix link = %v", m.room.posted)
	}
	if err := m.bots.TellApproval(context.Background(), tenant, ml.rows[0].ID, waitingApproval, "k"); err != nil {
		t.Fatal(err)
	}
	if len(m.room.posted) != 2 || strings.Contains(m.room.posted[1], "Laptop") ||
		!strings.HasPrefix(m.room.posted[1], "hubtask-approval-k|@anna:example.org: de:chat.bot_approval_waiting:"+ref) {
		t.Fatalf("matrix card = %v", m.room.posted)
	}
}
