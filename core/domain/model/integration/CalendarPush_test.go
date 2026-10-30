// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	pushTenant   = shared.ID("00000000-0000-7000-8000-0000000000a1")
	pushOwner    = shared.ID("00000000-0000-7000-8000-0000000000a2")
	pushStranger = shared.ID("00000000-0000-7000-8000-0000000000a3")
	pushNow      = time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
)

func pushFeed() CalendarFeed {
	return CalendarFeed{ID: "00000000-0000-7000-8000-0000000000b1", TenantID: pushTenant, AccountID: pushOwner,
		ViewID: "00000000-0000-7000-8000-0000000000b2", Kind: CalendarKind, CreatedAt: pushNow}
}

// UC-INT-16 check 1: a push connects its owner's own working calendar feed, nobody else's, and
// never a data feed or a revoked one.
func TestAPushConnectsItsOwnersCalendarFeedOnly(t *testing.T) {
	revoked := pushFeed()
	revoked.RevokedAt = pushNow
	data := pushFeed()
	data.Kind = DataKind
	for name, c := range map[string]struct {
		feed    CalendarFeed
		account shared.ID
	}{
		"somebody else's": {pushFeed(), pushStranger},
		"revoked":         {revoked, pushOwner},
		"a data feed":     {data, pushOwner},
	} {
		if _, err := NewCalendarPush("p", pushTenant, c.account, c.feed, PushGoogle, "", pushNow); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s: err = %v, want not found", name, err)
		}
	}
	push, err := NewCalendarPush("p", pushTenant, pushOwner, pushFeed(), PushMicrosoft, " ada@example.org ", pushNow)
	if err != nil || push.State != PushActive || push.Address != "ada@example.org" || push.FeedID != pushFeed().ID {
		t.Fatalf("push = %+v, %v", push, err)
	}
	if _, err := ParsePushProvider("dropbox"); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("an unknown provider: %v", err)
	}
	for _, provider := range []PushProvider{PushGoogle, PushMicrosoft} {
		for _, scope := range provider.PushScopes() {
			if scope == "https://mail.google.com/" || scope == "https://outlook.office.com/IMAP.AccessAsUser.All" {
				t.Errorf("%s asks for mail", provider)
			}
		}
	}
}

// UC-INT-16 check 2: a run creates what is new, updates what changed, removes what the feed no
// longer serves and leaves the rest alone.
func TestARunWritesOnlyWhatChanged(t *testing.T) {
	same := PushEvent{EntryID: "e1", Summary: "Same", Start: pushNow, URL: "https://x/e1"}
	changed := PushEvent{EntryID: "e2", Summary: "Now later", Start: pushNow.Add(time.Hour), URL: "https://x/e2"}
	fresh := PushEvent{EntryID: "e3", Summary: "New", Start: pushNow, AllDay: true, URL: "https://x/e3"}
	written := []PushedEvent{
		{EntryID: "e1", RemoteID: "r1", Digest: same.Digest()},
		{EntryID: "e2", RemoteID: "r2", Digest: "before"},
		{EntryID: "e4", RemoteID: "r4", Digest: "gone"},
	}
	plan := PlanPush([]PushEvent{same, changed, fresh, fresh}, written)
	if len(plan.Create) != 1 || plan.Create[0].EntryID != "e3" {
		t.Errorf("create = %v", plan.Create)
	}
	if len(plan.Update) != 1 || plan.Update[0].RemoteID != "r2" {
		t.Errorf("update = %v", plan.Update)
	}
	if len(plan.Delete) != 1 || plan.Delete[0].RemoteID != "r4" {
		t.Errorf("delete = %v", plan.Delete)
	}
}

// UC-INT-16 check 4: a refusal for good stops the push; a passing one keeps it and says why.
func TestAPushStopsOnlyOnARefusalForGood(t *testing.T) {
	push, _ := NewCalendarPush("p", pushTenant, pushOwner, pushFeed(), PushGoogle, "", pushNow)
	push.Pushed(pushNow, "calendar_push.provider_unreachable", false)
	if push.State != PushActive || push.Failure == "" {
		t.Errorf("after a passing failure: %+v", push)
	}
	push.Pushed(pushNow.Add(time.Minute), "", false)
	if push.Failure != "" || !push.PushedAt.Equal(pushNow.Add(time.Minute)) {
		t.Errorf("after a run: %+v", push)
	}
	push.Pushed(pushNow, "calendar_push.grant_revoked", true)
	if push.State != PushFailed {
		t.Errorf("after a revoked grant: %+v", push)
	}
}
