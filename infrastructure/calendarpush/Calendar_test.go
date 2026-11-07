// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package calendarpush

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

type provider struct {
	answers  []httpport.Response
	requests []httpport.Request
}

func (p *provider) Do(_ context.Context, req httpport.Request) (httpport.Response, error) {
	p.requests = append(p.requests, req)
	answer := p.answers[0]
	p.answers = p.answers[1:]
	return answer, nil
}

var due = time.Date(2026, 11, 2, 14, 0, 0, 0, time.UTC)

// UC-INT-16 check 2: an event is written into the person's own calendar with the title, the moment
// and the link alone, shown as free and without a reminder.
func TestAnEventHoldsTheTitleTheMomentAndTheLinkAlone(t *testing.T) {
	for _, c := range []struct {
		provider integration.PushProvider
		address  string
		want     []string
	}{
		{integration.PushGoogle, googleEvents, []string{`"summary":"Ship it"`, `"dateTime":"2026-11-02T14:00:00Z"`, `"dateTime":"2026-11-02T14:30:00Z"`, `"transparency":"transparent"`, `"useDefault":false`}},
		{integration.PushMicrosoft, graphEvents, []string{`"subject":"Ship it"`, `"dateTime":"2026-11-02T14:00:00"`, `"showAs":"free"`, `"isReminderOn":false`}},
	} {
		fake := &provider{answers: []httpport.Response{{Status: http.StatusCreated, Body: []byte(`{"id":"evt-1"}`)}}}
		id, err := Calendar{Client: fake}.Put(context.Background(), c.provider, secret.New("access"), "",
			integration.PushEvent{EntryID: "e1", Summary: "Ship it", Start: due, URL: "https://hubtask.example/items/e1"})
		if err != nil || id != "evt-1" {
			t.Fatalf("%s: %v, %v", c.provider, id, err)
		}
		req := fake.requests[0]
		if req.Method != http.MethodPost || req.URL != c.address || req.Header["Authorization"][0] != "Bearer access" || req.TargetClass != TargetClass {
			t.Errorf("%s: request %s %s", c.provider, req.Method, req.URL)
		}
		for _, want := range c.want {
			if !strings.Contains(string(req.Body), want) {
				t.Errorf("%s: body %s lacks %s", c.provider, req.Body, want)
			}
		}
		var fields map[string]any
		_ = json.Unmarshal(req.Body, &fields)
		for _, never := range []string{"attendees", "location"} {
			if _, ok := fields[never]; ok {
				t.Errorf("%s: the event carries %s", c.provider, never)
			}
		}
	}
}

// UC-INT-16 check 2: an all-day entry is a day's event; a changed one is patched in place, and one
// its owner removed at the provider is written anew.
func TestAChangedEventIsPatchedAndARemovedOneWrittenAnew(t *testing.T) {
	fake := &provider{answers: []httpport.Response{{Status: http.StatusNotFound}, {Status: http.StatusCreated, Body: []byte(`{"id":"evt-2"}`)}}}
	id, err := Calendar{Client: fake}.Put(context.Background(), integration.PushGoogle, secret.New("a"), "evt-1",
		integration.PushEvent{Summary: "Day", Start: due, AllDay: true})
	if err != nil || id != "evt-2" {
		t.Fatalf("%v, %v", id, err)
	}
	if fake.requests[0].Method != http.MethodPatch || !strings.HasSuffix(fake.requests[0].URL, "/evt-1") || fake.requests[1].Method != http.MethodPost {
		t.Errorf("requests %v %v", fake.requests[0].Method, fake.requests[1].Method)
	}
	if !strings.Contains(string(fake.requests[0].Body), `"end":{"date":"2026-11-03"}`) {
		t.Errorf("all day: %s", fake.requests[0].Body)
	}
	gone := &provider{answers: []httpport.Response{{Status: http.StatusGone}}}
	if err := (Calendar{Client: gone}).Delete(context.Background(), integration.PushMicrosoft, secret.New("a"), "evt-9"); err != nil {
		t.Errorf("deleting what is gone: %v", err)
	}
}

// UC-INT-16 check 4: a token the provider does not take is a grant revoked, a throttle passes.
func TestARefusalIsCoded(t *testing.T) {
	for status, want := range map[int]string{http.StatusUnauthorized: "calendar_push.grant_revoked", http.StatusTooManyRequests: "calendar_push.provider_unreachable"} {
		fake := &provider{answers: []httpport.Response{{Status: status}}}
		_, err := Calendar{Client: fake}.Put(context.Background(), integration.PushMicrosoft, secret.New("a"), "", integration.PushEvent{Summary: "x", Start: due})
		var typed *shared.Error
		if !errors.As(err, &typed) || typed.DetailCode != want {
			t.Errorf("%d: %v, want %s", status, err, want)
		}
	}
}
