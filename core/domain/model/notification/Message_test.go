// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/notification"
)

func TestAMessageCarriesOnlyWhatItHas(t *testing.T) {
	full := notification.Message{
		SubjectCode: "email.comment.subject", BodyCode: "email.comment.body",
		ActorName: "Anna", Title: "Review the quote", Link: "https://hub.example/items/1",
	}
	want := map[string]string{
		"actor": "Anna", "title": "Review the quote", "link": "https://hub.example/items/1",
	}
	if got := full.Params(); !reflect.DeepEqual(got, want) {
		t.Errorf("params %v, want %v", got, want)
	}

	// A withheld title is an absent parameter rather than an empty one: the catalogue leaves a
	// placeholder with no parameter standing, so an empty string would print as nothing at all and
	// a missing one shows up as missing.
	withheld := full
	withheld.Title = ""
	if _, present := withheld.Params()["title"]; present {
		t.Error("a withheld title still reaches the parameters")
	}

	if len(notification.Message{}.Params()) != 0 {
		t.Error("an empty message invents parameters")
	}
}

// The rule of data-protection.md §9, checked at the one door a title can come through.
func TestOnlyADecisionCanPutATitleInAMessage(t *testing.T) {
	base := notification.Message{SubjectCode: "email.comment.subject"}

	allowed := base.WithTitle("Review the quote", notification.Decision{
		Send: true, IncludeTitle: true,
	})
	if allowed.Title != "Review the quote" {
		t.Errorf("title %q - a permitted title has to travel", allowed.Title)
	}

	withheld := base.WithTitle("Review the quote", notification.Decision{Send: true})
	if withheld.Title != "" {
		t.Errorf("a withheld title travelled anyway: %q", withheld.Title)
	}

	// And the type itself is the enforcement: there is no field a note body could be put in.
	fields := map[string]bool{}
	messageType := reflect.TypeOf(notification.Message{})
	for i := range messageType.NumField() {
		fields[strings.ToLower(messageType.Field(i).Name)] = true
	}
	for _, forbidden := range []string{"notes", "note", "body", "text", "comment", "description"} {
		if fields[forbidden] {
			t.Errorf("Message has a %q field - content beyond the title can now reach an email",
				forbidden)
		}
	}
}

func TestASubjectIsCutRatherThanRefused(t *testing.T) {
	short := "Review the quote"
	if got := notification.TrimSubject("  " + short + "  "); got != short {
		t.Errorf("trimmed to %q, want %q", got, short)
	}

	exact := strings.Repeat("a", notification.SubjectMaxLength)
	if got := notification.TrimSubject(exact); got != exact {
		t.Errorf("a title of exactly the limit was cut to %d code points",
			utf8.RuneCountInString(got))
	}

	// Code points rather than bytes: cutting UTF-8 by byte produces a replacement character, and a
	// subject ending in one looks like a bug in the sender rather than a long title (I-W7).
	long := strings.Repeat("é", notification.SubjectMaxLength+50)
	cut := notification.TrimSubject(long)
	if utf8.RuneCountInString(cut) != notification.SubjectMaxLength {
		t.Errorf("cut to %d code points, want %d",
			utf8.RuneCountInString(cut), notification.SubjectMaxLength)
	}
	if !strings.HasSuffix(cut, notification.Ellipsis) {
		t.Errorf("the cut is not marked: %q", cut)
	}
	if strings.ContainsRune(cut, '�') {
		t.Errorf("the cut split a code point: %q", cut)
	}

	// A cut that lands on a space would leave a trailing gap before the ellipsis.
	spaced := strings.Repeat("a", notification.SubjectMaxLength-1) + "  tail"
	if got := notification.TrimSubject(spaced); strings.HasSuffix(got, " "+notification.Ellipsis) {
		t.Errorf("the cut left a trailing space: %q", got)
	}
}
