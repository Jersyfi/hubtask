// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

func keyed(key string) servicedesk.ChallengeMAC {
	return func(message string) []byte {
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write([]byte(message))
		return mac.Sum(nil)
	}
}

func solve(t *testing.T, value string, need int) string {
	t.Helper()
	for n := 0; n < 1<<26; n++ {
		if candidate := strconv.Itoa(n); servicedesk.Solves(value, candidate, need) {
			return candidate
		}
	}
	t.Fatal("no solution found")
	return ""
}

func detail(err error) string {
	var coded *shared.Error
	if !errors.As(err, &coded) {
		return ""
	}
	return coded.DetailCode
}

// UC-SVC-13 check 4: a submission carries a solved challenge the server signed for this form, not
// expired; another form's, a forged, an unsolved and an expired one are refused.
func TestAChallengeIsSignedSolvedAndShortLived(t *testing.T) {
	now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	mac := keyed("installation")
	nonce := bytes.Repeat([]byte{3}, servicedesk.ChallengeNonceBytes)
	challenge, err := servicedesk.IssueChallenge("tenant/collection", nonce, now, mac)
	if err != nil {
		t.Fatal(err)
	}
	if challenge.Bits != servicedesk.PublicFormWorkBits || !challenge.ExpiresAt.Equal(now.Add(servicedesk.PublicFormChallengeLife)) {
		t.Fatalf("challenge %+v", challenge)
	}
	solution := solve(t, challenge.Value, servicedesk.PublicFormWorkBits)
	spent, err := servicedesk.VerifyWork("tenant/collection", challenge.Value, solution, now.Add(time.Minute), mac)
	if err != nil || spent == "" {
		t.Fatalf("a solved challenge was refused: %v", err)
	}
	unsolved := "0"
	if servicedesk.Solves(challenge.Value, unsolved, servicedesk.PublicFormWorkBits) {
		unsolved = "1"
	}
	parts := strings.Split(challenge.Value, ".")
	for name, tc := range map[string]struct {
		scope, value, solution string
		at                     time.Time
		mac                    servicedesk.ChallengeMAC
		want                   string
	}{
		"another form": {"tenant/other", challenge.Value, solution, now, mac, "public_forms.work_invalid"},
		"another key":  {"tenant/collection", challenge.Value, solution, now, keyed("elsewhere"), "public_forms.work_invalid"},
		"later expiry": {"tenant/collection", "9999999999." + parts[1] + "." + parts[2], solution, now, mac, "public_forms.work_invalid"},
		"unsolved":     {"tenant/collection", challenge.Value, unsolved, now, mac, "public_forms.work_invalid"},
		"not a number": {"tenant/collection", challenge.Value, solution + "x", now, mac, "public_forms.work_invalid"},
		"malformed":    {"tenant/collection", "nothing", solution, now, mac, "public_forms.work_invalid"},
		"expired":      {"tenant/collection", challenge.Value, solution, challenge.ExpiresAt, mac, "public_forms.work_expired"},
	} {
		if _, err := servicedesk.VerifyWork(tc.scope, tc.value, tc.solution, tc.at, tc.mac); detail(err) != tc.want {
			t.Errorf("%s: answered %v, want %s", name, err, tc.want)
		}
	}
	if _, err := servicedesk.IssueChallenge("tenant/collection", []byte{1}, now, mac); err == nil {
		t.Error("a short nonce made a challenge")
	}
}

// UC-SVC-13 check 6: who a visitor says they are becomes the intake's From line, which the
// erasure by address finds; a name or address that would write a line of its own is refused.
func TestARequesterIsTheIntakesFromLine(t *testing.T) {
	requester, err := servicedesk.NewRequester(" Frau Kaya ", " Kaya@Example.org ")
	if err != nil {
		t.Fatal(err)
	}
	if line := requester.FromLine(); line != "From: Frau Kaya <kaya@example.org>" {
		t.Fatalf("line %q", line)
	}
	if rest, found := privacy.StripIntakeLine(requester.FromLine()+"\n\nDrucker druckt leer", "kaya@example.org"); !found || rest != "Drucker druckt leer" {
		t.Fatalf("the erasure does not find the line: %q %v", rest, found)
	}
	anonymous, err := servicedesk.NewRequester("", "")
	if err != nil || !anonymous.IsZero() {
		t.Fatalf("an anonymous request: %+v %v", anonymous, err)
	}
	for name, tc := range map[string]struct{ name, address, want string }{
		"a second line":    {"Kaya\nFrom: boss@example.org", "kaya@example.org", "public_forms.requester_name_invalid"},
		"brackets":         {"Kaya <x@y.z>", "kaya@example.org", "public_forms.requester_name_invalid"},
		"too long":         {strings.Repeat("a", servicedesk.MaxRequesterNameLength+1), "kaya@example.org", "public_forms.requester_name_invalid"},
		"no at":            {"", "kaya.example.org", "public_forms.requester_address_invalid"},
		"two addresses":    {"", "a@b.org,c@d.org", "public_forms.requester_address_invalid"},
		"no domain":        {"", "kaya@localhost", "public_forms.requester_address_invalid"},
		"name, no address": {"Kaya", "", "public_forms.requester_address_missing"},
	} {
		if _, err := servicedesk.NewRequester(tc.name, tc.address); detail(err) != tc.want {
			t.Errorf("%s: answered %v, want %s", name, err, tc.want)
		}
	}
}

// UC-SVC-13 check 2: a public form offers the types whose form names PUBLIC and leaves it open;
// the portal's catalogue is not the public one.
func TestThePublicCatalogueOffersTheTypesPublicOpens(t *testing.T) {
	opens := func(channel work.FormChannel, closed bool) work.Form {
		return work.Form{Channels: []work.FormChannelRule{{Channel: channel, Closed: closed}}}
	}
	types := []work.EntryType{
		{Key: "feedback", Level: work.ItemTask, Form: opens(work.ChannelPublic, false)},
		{Key: "incident", Level: work.ItemTask, Form: opens(work.ChannelPortal, false)},
		{Key: "closed", Level: work.ItemTask, Form: opens(work.ChannelPublic, true)},
		{Key: "plain", Level: work.ItemTask},
	}
	public := servicedesk.CatalogueThrough(work.ChannelPublic, types, nil, nil, "")
	if len(public) != 1 || public[0].Key != "feedback" {
		t.Fatalf("the public catalogue is %+v", public)
	}
	if portal := servicedesk.RequestCatalogue(types, nil, nil, ""); len(portal) != 1 || portal[0].Key != "incident" {
		t.Fatalf("the portal's catalogue is %+v", portal)
	}
}
