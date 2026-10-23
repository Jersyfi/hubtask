// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"errors"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// oneArrival is the ordinary read of one jumble entry, as the asker reads it: no submitter in it.
type oneArrival struct{ sender string }

func (o oneArrival) Invoke(
	_ context.Context, name string, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if name != "GetJumbleEntry" || in["entry_id"] != targetID.String() {
		return nil, shared.ErrNotFound
	}
	return usecase.Output{"raw_subject": "Order 42", "raw_body": "Ship it on Friday.", "sender": o.sender}, nil
}

// UC-PRV-05 check 7: what a kept-out person captured into the jumble is theirs, as what they mailed
// in is - a colleague's ask about it sends nothing. A capture from anybody else is sent.
func TestAColleaguesAskSendsNothingAKeptOutPersonCaptured(t *testing.T) {
	kim := shared.MustParseID("0192f000-0000-7000-8000-0000000000b9")
	kept := integration.NewKeptOut([]integration.KeptPerson{{
		AccountID: kim, Reason: integration.KeptOutObjected, Names: []string{"Kim Keeper", "kim@example.org"},
	}})
	question := Question{TargetType: domain.TargetJumbleEntry, TargetID: targetID, PromptID: "suggest-fields"}

	for name, tc := range map[string]struct {
		submitter shared.ID
		sender    string
		sends     bool
	}{
		"a capture Kim made":         {submitter: kim, sends: false},
		"a mail Kim sent":            {sender: "kim@example.org", sends: false},
		"a capture a colleague made": {submitter: accountID, sends: true},
		"a mail from nobody kept":    {sender: "orders@example.org", sends: true},
	} {
		sources := CatalogueSources{
			Catalogue: oneArrival{sender: tc.sender}, Kept: keptReader{kept},
			Jumble: entryByID{targetID: jumble.Entry{ID: targetID, CreatedBy: tc.submitter, Sender: tc.sender}},
		}
		material, err := sources.Material(context.Background(), person(), question)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if sends := strings.TrimSpace(material.Content) != ""; sends != tc.sends {
			t.Errorf("%s: sends %v, want %v (%q)", name, sends, tc.sends, material.Content)
		}
		if len(material.Digest) == 0 {
			t.Errorf("%s: the material lost its fingerprint", name)
		}
	}
}

// The submitter is read only after the ordinary read said the asker may read the entry: an entry
// they may not read is answered as one that does not exist, whoever submitted it.
func TestTheSubmitterIsReadOnlyForAReadableEntry(t *testing.T) {
	sources := CatalogueSources{Catalogue: oneArrival{}, Jumble: entryByID{}}
	_, err := sources.Material(context.Background(), person(), Question{
		TargetType: domain.TargetJumbleEntry, TargetID: shared.MustParseID("0192f000-0000-7000-8000-0000000000ee"),
		PromptID: "suggest-fields",
	})
	if !errors.Is(err, shared.ErrNotFound) || shared.AsError(err).DetailCode != "suggestions.not_found" {
		t.Errorf("an unreadable entry answered %v", err)
	}
}
