// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	tenant  = shared.ID("018f2a1b-0000-7000-8000-0000000000ab")
	owner   = shared.ID("018f2a1b-0000-7000-8000-0000000000c1")
	view    = shared.ID("018f2a1b-0000-7000-8000-0000000000d1")
	feedID  = shared.ID("018f2a1b-0000-7000-8000-0000000000e1")
	nowText = "2026-09-07T09:00:00Z"
)

func at(t *testing.T, text string) time.Time {
	t.Helper()
	moment, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatalf("the fixture is not a moment: %v", err)
	}
	return moment
}

func mintFeed(t *testing.T) CalendarFeed {
	t.Helper()
	feed, err := NewCalendarFeed(NewCalendarFeedInput{
		ID: feedID, TenantID: tenant, AccountID: owner, ViewID: view, Now: at(t, nowText),
	})
	if err != nil {
		t.Fatalf("minting the feed failed: %v", err)
	}
	return feed
}

func TestAFeedIsMintedOverAViewForAnOwner(t *testing.T) {
	feed := mintFeed(t)

	if feed.AccountID != owner || feed.ViewID != view || feed.TenantID != tenant {
		t.Errorf("the feed came out as %+v", feed)
	}
	if feed.IsRevoked() || !feed.ServesAView() {
		t.Error("a fresh feed is revoked or serves nothing")
	}
	if !feed.CreatedAt.Equal(at(t, nowText)) {
		t.Errorf("created at %v", feed.CreatedAt)
	}
}

func TestWhatAFeedRefuses(t *testing.T) {
	cases := []struct {
		name  string
		in    NewCalendarFeedInput
		code  string
		field string
	}{
		{
			name: "no view to serve",
			in:   NewCalendarFeedInput{ID: feedID, TenantID: tenant, AccountID: owner},
			code: "calendar.view_required", field: "/view_id",
		},
		{
			name: "no owner to read as",
			in:   NewCalendarFeedInput{ID: feedID, TenantID: tenant, ViewID: view},
			code: "calendar.feed_owner_missing",
		},
		{
			name: "no tenant",
			in:   NewCalendarFeedInput{ID: feedID, AccountID: owner, ViewID: view},
			code: "calendar.feed_incomplete",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewCalendarFeed(c.in)
			refusal := shared.AsError(err)
			if refusal == nil || refusal.DetailCode != c.code {
				t.Fatalf("refused as %v, want %s", err, c.code)
			}
			if c.field == "" {
				return
			}
			if len(refusal.Fields) != 1 || refusal.Fields[0].Path != c.field {
				t.Errorf("the refusal points at %v", refusal.Fields)
			}
		})
	}
}

// Revocation is immediate and does not move once it happened: the moment a token stopped working
// is a fact somebody looks up after a laptop goes missing.
func TestRevokingIsIdempotentAndKeepsTheFirstMoment(t *testing.T) {
	feed := mintFeed(t)
	tuesday := at(t, "2026-09-08T11:00:00Z")

	revoked, changed := feed.Revoked(tuesday)
	if !changed || !revoked.IsRevoked() || !revoked.RevokedAt.Equal(tuesday) {
		t.Fatalf("the first revocation came out as %+v (changed=%v)", revoked, changed)
	}

	again, changed := revoked.Revoked(at(t, "2026-09-09T11:00:00Z"))
	if changed {
		t.Error("revoking twice reported a change")
	}
	if !again.RevokedAt.Equal(tuesday) {
		t.Errorf("the moment moved to %v", again.RevokedAt)
	}
}

// A view deleted underneath a feed leaves the feed standing with nothing to serve.
func TestAFeedWhoseViewIsGoneServesNothing(t *testing.T) {
	feed := mintFeed(t)
	feed.ViewID = ""

	if feed.ServesAView() {
		t.Error("a feed with no view claims to serve one")
	}
	if feed.IsRevoked() {
		t.Error("losing the view revoked the token, and those are two different states")
	}
}

func TestAMintedTokenParsesBackToItsTenant(t *testing.T) {
	token := mintToken(t, tenant)

	if !strings.HasPrefix(token.Secret(), FeedTokenPrefix) {
		t.Errorf("the token does not carry the scanning prefix: %q", token.Secret())
	}
	parsed, err := ParseFeedToken(token.Secret())
	if err != nil {
		t.Fatalf("a token this package minted did not parse: %v", err)
	}
	if parsed.TenantID() != tenant {
		t.Errorf("the token names tenant %s", parsed.TenantID())
	}
	if parsed.Secret() != token.Secret() {
		t.Error("parsing changed the credential")
	}
}

// Every malformed shape is one answer. A parser that said which check failed would tell somebody
// guessing how far their guess got.
func TestEveryMalformedTokenIsTheSameAnswer(t *testing.T) {
	good := mintToken(t, tenant).Secret()

	cases := map[string]string{
		"empty":          "",
		"another prefix": strings.Replace(good, FeedTokenPrefix, "hbt_pat_", 1),
		"no prefix":      strings.TrimPrefix(good, FeedTokenPrefix),
		"no separator":   strings.Replace(good, "_", "", 2),
		"a short tenant": FeedTokenPrefix + "0123_" + strings.Split(good, "_")[3],
		"a short secret": good[:len(good)-1],
		"a long secret":  good + "a",
		"a tenant in hex it is not": FeedTokenPrefix + strings.Repeat("z", 32) + "_" +
			strings.Split(good, "_")[3],
		"a secret outside the alphabet": good[:len(good)-1] + "!",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseFeedToken(raw)
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Fatalf("refused as %v", err)
			}
			if refusal := shared.AsError(err); refusal == nil ||
				refusal.DetailCode != "calendar.token_malformed" {
				t.Errorf("the refusal is %v, and every malformed token answers the same", err)
			}
		})
	}
}

func TestMintingRefusesWhatCannotBeACredential(t *testing.T) {
	if _, err := NewFeedToken("", make([]byte, FeedTokenSecretBytes)); err == nil {
		t.Error("a token was minted without a tenant")
	}
	if _, err := NewFeedToken(tenant, make([]byte, 8)); err == nil {
		t.Error("a token was minted from eight bytes of entropy")
	}
}

// The one thing this type exists to prevent: a feed token reaching a log line, an error message
// or a JSON body. Every way of printing it is masked, because %v over a struct would otherwise
// print the unexported field in full - and this credential travels in a URL, so a copy in an
// access log is a copy of the whole authorisation (T-21).
func TestATokenDoesNotPrintItself(t *testing.T) {
	token := mintToken(t, tenant)
	// MarshalText is asked directly rather than through an encoder: the domain does no
	// serialisation, and what matters is that the encoders' hook answers the mask.
	encoded, err := token.MarshalText()
	if err != nil {
		t.Fatalf("marshalling failed: %v", err)
	}
	printed := fmt.Sprintf("%v %s %+v %#v %s", token, token, token, token, encoded)

	if strings.Contains(printed, token.Secret()[len(FeedTokenPrefix):]) {
		t.Errorf("the credential printed itself: %s", printed)
	}
}

func mintToken(t *testing.T, tenantID shared.ID) FeedToken {
	t.Helper()
	secret := make([]byte, FeedTokenSecretBytes)
	for i := range secret {
		secret[i] = byte(i)
	}
	token, err := NewFeedToken(tenantID, secret)
	if err != nil {
		t.Fatalf("minting failed: %v", err)
	}
	return token
}
