// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"

	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// addressStore stands in for the requester record and the intake of one address.
type addressStore struct {
	held   int
	erased []string
	full   bool
}

func (a *addressStore) CountAddress(context.Context, string) (int, error) { return a.held, nil }

func (a *addressStore) EraseAddress(_ context.Context, address string, full bool) (int, error) {
	a.erased, a.full = append(a.erased, address), full
	n := a.held
	a.held = 0
	return n, nil
}

func addressCase(mode domain.ErasureMode) domain.Request {
	request := erasureCase(mode)
	request.SubjectAccountID, request.SubjectEmail = "", "Kunde@Shop.test"
	return request
}

// accepted.md B: a requester without an account has an erasure path - a case by address erases
// the requester record, the receipts and the intake of the address, in either mode.
func TestACaseByAddressErasesTheRequesterWithoutAnAccount(t *testing.T) {
	for _, mode := range []domain.ErasureMode{domain.ModeFullDelete, domain.ModeAnonymize} {
		h := newErasureHarness()
		addresses := &addressStore{held: 4}
		eraser := h.eraser()
		eraser.Addresses = addresses
		erased, err := eraser.Erase(context.Background(), actor(), addressCase(mode))
		if err != nil {
			t.Fatalf("%s: erasing: %v", mode, err)
		}
		if len(addresses.erased) != 1 || addresses.erased[0] != "kunde@shop.test" || addresses.full != (mode == domain.ModeFullDelete) {
			t.Errorf("%s: erased %v (full %v)", mode, addresses.erased, addresses.full)
		}
		if erased.Intake != 4 || erased.AccountRemoved || len(erased.Kept) != 0 {
			t.Errorf("%s: the erasure did %+v", mode, erased)
		}
		if len(h.audit.entries) == 0 {
			t.Errorf("%s: the erasure was not recorded", mode)
		}
	}
}

// A hold on the workspace keeps what an address sent, recorded as kept for the remainder; no
// narrower hold does.
func TestAWorkspaceHoldKeepsWhatAnAddressSent(t *testing.T) {
	h := newErasureHarness()
	h.holds.holds = lifecycle.Holds{{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000f1"), Scope: lifecycle.HoldTenant, Reason: "Litigation"}}
	addresses := &addressStore{held: 3}
	eraser := h.eraser()
	eraser.Addresses = addresses
	erased, err := eraser.Erase(context.Background(), actor(), addressCase(domain.ModeFullDelete))
	if err != nil {
		t.Fatalf("erasing: %v", err)
	}
	if len(addresses.erased) != 0 || len(erased.Kept) != 1 || erased.Kept[0].Intake != 3 {
		t.Errorf("under a hold erased %v and kept %+v", addresses.erased, erased.Kept)
	}
}
