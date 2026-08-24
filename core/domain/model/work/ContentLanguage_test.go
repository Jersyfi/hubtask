// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The language an entry is written in (C-08). What it decides is the text search configuration the
// entry is indexed under, which is why it is a field of the entry rather than of the account that
// wrote it (i18n-l10n.md §5, ADR-0034).

func TestAnEntryCarriesTheLanguageItWasWrittenIn(t *testing.T) {
	in := taskInput()
	in.ContentLanguage = "  de-AT  "

	item, err := NewWorkItem(in)
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.ContentLanguage != "de-AT" {
		t.Errorf("the language is %q, want the trimmed tag", item.ContentLanguage)
	}
}

// Empty is "nobody said", and it is a state rather than a failure: the use case defaults it to the
// creator's locale, and an entry that reaches the domain without one is indexed word by word.
func TestAnEntryMayStateNoLanguageAtAll(t *testing.T) {
	item, err := NewWorkItem(taskInput())
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if item.ContentLanguage != "" {
		t.Errorf("an unstated language became %q", item.ContentLanguage)
	}
}

func TestAMalformedLanguageIsRefusedByName(t *testing.T) {
	in := taskInput()
	in.ContentLanguage = "German, mostly"

	_, err := NewWorkItem(in)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a malformed tag answered %v", err)
	}

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "items.content_language_invalid" {
		t.Fatalf("the detail code is not items.content_language_invalid: %v", err)
	}
	// The field path is what lets a client mark the input rather than the form.
	if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != "/content_language" {
		t.Errorf("the error does not name the field: %+v", domainErr.Fields)
	}
}

// A language this installation cannot index is still a language. Which configurations exist is what
// the installation's PostgreSQL has (ADR-0034), and an entry in Welsh is indexed word by word -
// which is an answer, where a refusal would not be.
func TestALanguageNothingCanIndexIsStillAccepted(t *testing.T) {
	in := taskInput()
	in.ContentLanguage = "cy"

	item, err := NewWorkItem(in)
	if err != nil {
		t.Fatalf("Welsh was refused: %v", err)
	}
	if item.ContentLanguage != "cy" {
		t.Errorf("the language is %q", item.ContentLanguage)
	}
}

func TestChangingTheLanguageIsReportedAsAFieldThatMoved(t *testing.T) {
	before := updatable(t)
	before.ContentLanguage = "en"

	after, changes, err := before.Updated(
		ItemAttributes{ContentLanguage: text("de")}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("updating: %v", err)
	}
	if after.ContentLanguage != "de" {
		t.Errorf("the language is %q, want de", after.ContentLanguage)
	}
	if len(changes) != 1 || changes[0].Field != FieldContentLanguage {
		t.Fatalf("the change set is %+v, want the language alone", changes)
	}
	if changes[0].From != "en" || changes[0].To != "de" {
		t.Errorf("the change reads %q → %q", changes[0].From, changes[0].To)
	}
}

// Both sides of "leave it alone": the same tag again is not a change, and the empty string clears
// the statement rather than being ignored as an absent value would be.
func TestTheLanguageFollowsTheSamePresenceRuleAsTheNotes(t *testing.T) {
	before := updatable(t)
	before.ContentLanguage = "en"

	if _, changes, err := before.Updated(
		ItemAttributes{ContentLanguage: text("en")}, taskProfile(), laterOn,
	); err != nil || len(changes) != 0 {
		t.Errorf("writing the same language again reported %+v (%v)", changes, err)
	}

	cleared, changes, err := before.Updated(
		ItemAttributes{ContentLanguage: text("")}, taskProfile(), laterOn)
	if err != nil {
		t.Fatalf("clearing: %v", err)
	}
	if cleared.ContentLanguage != "" || len(changes) != 1 {
		t.Errorf("clearing left %q and reported %+v", cleared.ContentLanguage, changes)
	}

	if (ItemAttributes{ContentLanguage: text("")}).IsEmpty() {
		t.Error("clearing the language reports itself as an empty update")
	}
}

func TestAMalformedLanguageIsRefusedOnAnUpdateToo(t *testing.T) {
	_, _, err := updatable(t).Updated(
		ItemAttributes{ContentLanguage: text("de_AT")}, taskProfile(), laterOn)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "items.content_language_invalid" {
		t.Fatalf("the detail code is not items.content_language_invalid: %v", err)
	}
}
