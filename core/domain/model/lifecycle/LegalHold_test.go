// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

var (
	hubID        = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-00000000000c")
	otherHubID   = shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	taskID       = shared.MustParseID("0192f000-0000-7000-8000-000000000001")
	packageID    = shared.MustParseID("0192f000-0000-7000-8000-000000000002")
	accountID    = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	holdID       = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")

	otherAccountID = shared.MustParseID("0192f000-0000-7000-8000-00000000001d")
	placedAt       = time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
)

// A work package three levels down, as a purge would present it: the containers above it, and the
// entries above it including itself.
func target() lifecycle.Target {
	return lifecycle.Target{
		ItemID:          packageID,
		ContainerIDs:    []shared.ID{hubID, collectionID},
		AncestorItemIDs: []shared.ID{taskID, packageID},
		Contributors:    []shared.ID{accountID},
	}
}

// Which holds are in force on what: the account and the workspace are the two scopes that reach a
// person's own record, and a removal reads contributors only while an account hold stands.
func TestTheHoldsOnAnAccountAndOnTheWorkspaceAreFound(t *testing.T) {
	onAccount := hold(lifecycle.HoldAccount, accountID)
	onTenant := hold(lifecycle.HoldTenant, "")
	holds := lifecycle.Holds{hold(lifecycle.HoldContainer, hubID), onAccount}

	if !holds.AnyOnAccounts() {
		t.Error("an account hold in force was not noticed")
	}
	if (lifecycle.Holds{hold(lifecycle.HoldContainer, hubID), hold(lifecycle.HoldAccount, "")}).AnyOnAccounts() {
		t.Error("an account hold naming nobody counted")
	}
	if found, ok := holds.OnAccount(accountID); !ok || found.ScopeID != accountID {
		t.Errorf("the hold on the account came back as %+v, %v", found, ok)
	}
	if _, ok := holds.OnAccount(otherAccountID); ok {
		t.Error("another account was found held")
	}
	if _, ok := holds.OnAccount(""); ok {
		t.Error("nobody was found held")
	}
	if _, ok := holds.OnTenant(); ok {
		t.Error("a workspace hold was found where there is none")
	}
	if _, ok := append(holds, onTenant).OnTenant(); !ok {
		t.Error("the workspace hold was not found")
	}
}

func hold(scope lifecycle.HoldScope, id shared.ID) lifecycle.LegalHold {
	return lifecycle.LegalHold{ID: holdID, Scope: scope, ScopeID: id, Reason: "Pending litigation"}
}

// The precedence rule of data-retention.md §4: a legal hold is checked first and overrides
// everything. What it covers is a path rather than a row - a hold on a hub has to reach an entry
// three levels below it, or it would be a hold in name only.
func TestAHoldReachesEverythingBelowWhatItNames(t *testing.T) {
	for _, c := range []struct {
		name    string
		holds   lifecycle.Holds
		blocked bool
	}{
		{"nothing held", nil, false},
		{"the whole tenant", lifecycle.Holds{hold(lifecycle.HoldTenant, "")}, true},
		{"the hub above it", lifecycle.Holds{hold(lifecycle.HoldContainer, hubID)}, true},
		{"its own collection", lifecycle.Holds{hold(lifecycle.HoldContainer, collectionID)}, true},
		{"another hub", lifecycle.Holds{hold(lifecycle.HoldContainer, otherHubID)}, false},
		{"the entry above it", lifecycle.Holds{hold(lifecycle.HoldItem, taskID)}, true},
		{"the entry itself", lifecycle.Holds{hold(lifecycle.HoldItem, packageID)}, true},
		{"a sibling entry", lifecycle.Holds{hold(lifecycle.HoldItem, shared.MustParseID("0192f000-0000-7000-8000-000000000003"))}, false},
		// A hold on a person covers what they contributed: an entry they created, commented on or
		// attached a file to goes only with their data, so it is held (data-protection.md §4.1).
		{"an account that contributed", lifecycle.Holds{hold(lifecycle.HoldAccount, accountID)}, true},
		{"an account that did not", lifecycle.Holds{hold(lifecycle.HoldAccount, otherAccountID)}, false},
		{"an account hold naming nobody", lifecycle.Holds{hold(lifecycle.HoldAccount, "")}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			blocking, blocked := c.holds.Blocking(target())

			if blocked != c.blocked {
				t.Errorf("blocked %v, want %v", blocked, c.blocked)
			}
			if blocked && blocking.ID != holdID {
				t.Errorf("the blocking hold is %q, want the one that was placed", blocking.ID)
			}
		})
	}
}

// The hold itself is returned rather than a boolean, because a blocked run has to be able to say
// why: "a hold placed on the hub" is an answer an operator can act on where "blocked" is not.
func TestTheBlockingHoldIsNamed(t *testing.T) {
	held := hold(lifecycle.HoldContainer, hubID)
	holds := lifecycle.Holds{hold(lifecycle.HoldContainer, otherHubID), held}

	blocking, blocked := holds.Blocking(target())
	if !blocked {
		t.Fatal("a hold on the hub did not block")
	}
	if blocking.Scope != lifecycle.HoldContainer || blocking.ScopeID != hubID {
		t.Errorf("the answer names %s/%s, want the hold on the hub", blocking.Scope, blocking.ScopeID)
	}
	if blocking.Reason == "" {
		t.Error("the answer carries no reason for an operator to read")
	}
}

// A container's own purge asks the same question with no entry in the target.
func TestAContainerTargetIsJudgedByItsOwnPath(t *testing.T) {
	container := lifecycle.Target{ContainerIDs: []shared.ID{hubID, collectionID}}

	if _, blocked := (lifecycle.Holds{hold(lifecycle.HoldContainer, hubID)}).Blocking(container); !blocked {
		t.Error("a hold on the hub did not block its collection")
	}
	// An item hold names nothing in a container target, and an empty identifier must not match the
	// empty ItemID - which is what a naive equality would do.
	if _, blocked := (lifecycle.Holds{hold(lifecycle.HoldItem, "")}).Blocking(container); blocked {
		t.Error("a hold naming no entry blocked a container")
	}
}

// Placing one: what the model accepts, and the one scope it refuses.

func holdInput(change func(*lifecycle.NewHoldInput)) lifecycle.NewHoldInput {
	in := lifecycle.NewHoldInput{
		ID: holdID, Scope: lifecycle.HoldContainer, ScopeID: hubID,
		Reason: "Pending litigation, ref. 4 O 128/26", PlacedBy: accountID,
		Now: placedAt, Text: text.Composing{},
	}
	change(&in)
	return in
}

func holdCode(t *testing.T, err error) string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	return domainErr.DetailCode
}

func TestAHoldIsPlacedWithWhoAndWhy(t *testing.T) {
	hold, err := lifecycle.NewLegalHold(holdInput(func(*lifecycle.NewHoldInput) {}))
	if err != nil {
		t.Fatalf("a valid hold was refused: %v", err)
	}

	switch {
	case hold.PlacedBy != accountID:
		t.Errorf("the hold was placed by %s", hold.PlacedBy)
	case !hold.PlacedAt.Equal(placedAt):
		t.Errorf("the hold was placed at %s", hold.PlacedAt)
	case hold.Released():
		t.Error("a new hold is already released")
	}
}

func TestWhatAHoldCannotMean(t *testing.T) {
	cases := map[string]struct {
		change func(*lifecycle.NewHoldInput)
		code   string
	}{
		"a scope nobody defined": {
			func(in *lifecycle.NewHoldInput) { in.Scope = "EVERYTHING" },
			lifecycle.CodeHoldScopeInvalid,
		},
		"a workspace-wide hold that names something": {
			func(in *lifecycle.NewHoldInput) { in.Scope = lifecycle.HoldTenant },
			lifecycle.CodeHoldScopeIDMismatch,
		},
		"a container hold that names nothing": {
			func(in *lifecycle.NewHoldInput) { in.ScopeID = "" },
			lifecycle.CodeHoldScopeIDMismatch,
		},
		"no reason": {
			func(in *lifecycle.NewHoldInput) { in.Reason = "   " },
			lifecycle.CodeHoldReasonRequired,
		},
		"a reason longer than a reason": {
			func(in *lifecycle.NewHoldInput) { in.Reason = strings.Repeat("x", 2001) },
			lifecycle.CodeHoldReasonTooLong,
		},
		"nobody placing it": {
			func(in *lifecycle.NewHoldInput) { in.PlacedBy = "" },
			lifecycle.CodeHoldIncomplete,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := lifecycle.NewLegalHold(holdInput(test.change))
			if err == nil {
				t.Fatal("the hold was accepted")
			}
			if code := holdCode(t, err); code != test.code {
				t.Fatalf("refused with %s, want %s", code, test.code)
			}
		})
	}
}

// A hold on a person is accepted: every deletion path and the erasure honour it
// (data-protection.md §4.1), so it is no longer a hold nobody acts on.
func TestAnAccountHoldIsAccepted(t *testing.T) {
	hold, err := lifecycle.NewLegalHold(holdInput(func(in *lifecycle.NewHoldInput) {
		in.Scope, in.ScopeID = lifecycle.HoldAccount, accountID
	}))
	if err != nil {
		t.Fatalf("a hold on a person was refused: %v", err)
	}
	if hold.Scope != lifecycle.HoldAccount || hold.ScopeID != accountID {
		t.Errorf("the hold covers %s %s", hold.Scope, hold.ScopeID)
	}

	// It names somebody, like every scope but the workspace.
	_, err = lifecycle.NewLegalHold(holdInput(func(in *lifecycle.NewHoldInput) {
		in.Scope, in.ScopeID = lifecycle.HoldAccount, ""
	}))
	if code := holdCode(t, err); code != lifecycle.CodeHoldScopeIDMismatch {
		t.Errorf("a hold on nobody was refused with %s", code)
	}
}
func TestLiftingRecordsWhoAndWhyAndHappensOnce(t *testing.T) {
	hold, err := lifecycle.NewLegalHold(holdInput(func(*lifecycle.NewHoldInput) {}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}

	releasedAt := placedAt.Add(72 * time.Hour)
	released, err := hold.Release(otherAccountID, "The proceedings ended", nil, releasedAt)
	if err != nil {
		t.Fatalf("releasing: %v", err)
	}

	switch {
	case !released.Released():
		t.Error("a released hold does not say so")
	case released.ReleasedBy != otherAccountID:
		t.Errorf("it was lifted by %s", released.ReleasedBy)
	case !released.ReleasedAt.Equal(releasedAt):
		t.Errorf("it was lifted at %s", released.ReleasedAt)
	case released.ReleasedReason != "The proceedings ended":
		t.Errorf("the reason is %q", released.ReleasedReason)
	}
	// The placing is untouched: the record is both ends rather than the latest one.
	if released.PlacedBy != accountID || !released.PlacedAt.Equal(placedAt) {
		t.Error("lifting overwrote who placed it")
	}

	if _, err := released.Release(accountID, "again", nil, releasedAt); err == nil {
		t.Fatal("a hold was lifted twice")
	} else if code := holdCode(t, err); code != lifecycle.CodeHoldAlreadyReleased {
		t.Fatalf("the second lifting was refused with %s", code)
	}

	if _, err := hold.Release(otherAccountID, "  ", nil, releasedAt); err == nil {
		t.Fatal("a hold was lifted with no reason")
	}
}

// The reason is stored in normal form C, when a hold is placed and when it is lifted (i18n-l10n.md
// §5): it is read by an auditor, in one spelling.
func TestAHoldsReasonIsStoredInNormalFormC(t *testing.T) {
	hold, err := lifecycle.NewLegalHold(holdInput(func(in *lifecycle.NewHoldInput) {
		in.Reason = "Anha\u0308ngiges Verfahren"
	}))
	if err != nil {
		t.Fatalf("placing: %v", err)
	}
	if hold.Reason != "Anh\u00e4ngiges Verfahren" {
		t.Errorf("reason = %q, want the composed form", hold.Reason)
	}
	released, err := hold.Release(accountID, "Verfahren beendet, Akte geschlossen \u2013 Gru\u0308\u00dfe", text.Composing{}, placedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("lifting: %v", err)
	}
	if released.ReleasedReason != "Verfahren beendet, Akte geschlossen \u2013 Gr\u00fc\u00dfe" {
		t.Errorf("released reason = %q, want the composed form", released.ReleasedReason)
	}
	_, err = lifecycle.NewLegalHold(holdInput(func(in *lifecycle.NewHoldInput) { in.Reason, in.Text = "Anha\u0308ngig", nil }))
	if shared.AsError(err).DetailCode != "text.normalizer_missing" {
		t.Errorf("without a port the decomposed reason was accepted: %v", err)
	}
}
