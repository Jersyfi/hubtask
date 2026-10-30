// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package chatbot

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/chatbot"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type homeserver struct {
	answers  []httpport.Response
	requests []httpport.Request
}

func (h *homeserver) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	h.requests = append(h.requests, req)
	answer := h.answers[0]
	h.answers = h.answers[1:]
	return answer, nil
}

const hs = "https://matrix.example.org"

// UC-INT-18 check 1: the account is named, the room joined, and a first read answers the position
// alone so the room's history is never answered.
func TestABotIsNamedJoinsAndStartsAtTheRoomsPresent(t *testing.T) {
	fake := &homeserver{answers: []httpport.Response{
		{Status: 200, Body: []byte(`{"user_id":"@hubtask:example.org"}`)},
		{Status: 200, Body: []byte(`{"room_id":"!r:example.org"}`)},
		{Status: 200, Body: []byte(`{"next_batch":"s1","rooms":{"join":{"!r:example.org":{"timeline":{"events":[
			{"type":"m.room.message","event_id":"$old","sender":"@ada:example.org","content":{"msgtype":"m.text","body":"!task old"}}]}}}}}`)},
	}}
	m := Matrix{Client: fake}
	ctx, token := context.Background(), secret.New("tok")
	if who, err := m.WhoAmI(ctx, hs, token); err != nil || who != "@hubtask:example.org" {
		t.Fatalf("whoami = %q %v", who, err)
	}
	if err := m.Join(ctx, hs, token, "!r:example.org"); err != nil {
		t.Fatal(err)
	}
	if fake.requests[1].URL != hs+"/_matrix/client/v3/join/%21r:example.org" {
		t.Fatalf("join asked %s", fake.requests[1].URL)
	}
	next, messages, err := m.Read(ctx, hs, token, "!r:example.org", "")
	if err != nil || next != "s1" || len(messages) != 0 {
		t.Fatalf("first read = %q %v %v", next, messages, err)
	}
	if fake.requests[0].Header["Authorization"][0] != "Bearer tok" {
		t.Fatal("the token was not sent as a bearer")
	}
}

// UC-INT-18 checks 2 and 3: a read answers the room's texts alone, filtered to the room, and a
// notice is posted once per transaction; a revoked token says so.
func TestABotReadsTextsAndPostsNotices(t *testing.T) {
	fake := &homeserver{answers: []httpport.Response{
		{Status: 200, Body: []byte(`{"next_batch":"s2","rooms":{"join":{"!r:example.org":{"timeline":{"events":[
			{"type":"m.room.message","event_id":"$1","sender":"@ada:example.org","content":{"msgtype":"m.text","body":"!task Renew"}},
			{"type":"m.room.message","event_id":"$2","sender":"@ada:example.org","content":{"msgtype":"m.image","body":"pic.png"}},
			{"type":"m.room.message","event_id":"$3","sender":"@bob:example.org","content":{"msgtype":"m.notice","body":"a bot"}}]}}}}}`)},
		{Status: 200, Body: []byte(`{"event_id":"$n"}`)},
		{Status: http.StatusUnauthorized},
	}}
	m := Matrix{Client: fake}
	ctx, token := context.Background(), secret.New("tok")
	next, messages, err := m.Read(ctx, hs, token, "!r:example.org", "s1")
	if err != nil || next != "s2" || len(messages) != 1 || messages[0].Body != "!task Renew" || messages[0].Sender != "@ada:example.org" {
		t.Fatalf("read = %q %+v %v", next, messages, err)
	}
	u, _ := url.Parse(fake.requests[0].URL)
	if u.Query().Get("since") != "s1" || !strings.Contains(u.Query().Get("filter"), `"rooms":["!r:example.org"]`) || u.Query().Get("timeout") != "0" {
		t.Fatalf("sync asked %s", fake.requests[0].URL)
	}
	if err := m.Post(ctx, hs, token, "!r:example.org", "hubtask-$1", port.Notice{Body: "B", HTML: "<b>B</b>"}); err != nil {
		t.Fatal(err)
	}
	post := fake.requests[1]
	if post.Method != http.MethodPut || !strings.HasSuffix(post.URL, "/send/m.room.message/hubtask-$1") || !strings.Contains(string(post.Body), `"msgtype":"m.notice"`) {
		t.Fatalf("post = %s %s %s", post.Method, post.URL, post.Body)
	}
	var e *shared.Error
	if _, _, err := m.Read(ctx, hs, token, "!r:example.org", "s2"); !errors.As(err, &e) || e.DetailCode != "chatbot.token_revoked" {
		t.Fatalf("revoked = %v", err)
	}
}
