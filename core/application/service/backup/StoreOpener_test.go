// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// The seam the audit export writes through (E-09). What is under test is that a caller who needs
// somewhere to put bytes gets a store and never a credential, and that a target nobody may write
// to is refused here rather than at the target.

func (h *harness) storeOpener() StoreOpener {
	return StoreOpener{
		Targets: h.targets, Opener: h.opener, Encryptor: h.encryptor, UnitOfWork: h.uow,
	}
}

// stored puts one target in place, with its credential sealed the way the writer would have.
func (h *harness) stored(t *testing.T, enabled bool) domain.Target {
	t.Helper()

	target, err := (CreateBackupTarget{Writer: h.writer()}).
		Execute(context.Background(), caller(), command())
	if err != nil {
		t.Fatalf("creating the target: %v", err)
	}
	h.targets.stored[0].Enabled = enabled
	// Creating a target probes it, which opens the adapter once. What this file is about is what
	// happens afterwards, so the record starts here.
	h.opener.opened = nil
	return target
}

func TestOpeningATargetHandsOverAStoreAndNoCredential(t *testing.T) {
	h := newHarness()
	target := h.stored(t, true)

	store, err := h.storeOpener().OpenTarget(context.Background(), tenantID, target.ID)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	if store == nil {
		t.Fatal("no store came back")
	}

	// The adapter is opened with the credential unsealed under the target's own purpose, which is
	// what keeps a ciphertext lifted from another row from opening here.
	if len(h.opener.opened) != 1 {
		t.Fatalf("the adapter was opened %d times", len(h.opener.opened))
	}
	spec := h.opener.opened[0]
	if spec.Kind != domain.KindS3 {
		t.Errorf("the adapter was opened for %s", spec.Kind)
	}
	if spec.Credentials["access_key"].Reveal() != "AKIAEXAMPLE" {
		t.Error("the credential did not reach the adapter")
	}

	// A read rather than a write transaction: opening a target changes nothing.
	if h.uow.reads == 0 {
		t.Error("the target was read outside a read transaction")
	}
}

func TestOpeningADisabledTargetIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	h := newHarness()
	target := h.stored(t, false)

	if _, err := h.storeOpener().OpenTarget(context.Background(), tenantID, target.ID); err == nil {
		t.Fatal("a disabled target was opened")
	}
	if len(h.opener.opened) != 0 {
		t.Error("the adapter was opened for a target nobody may write to")
	}
}

func TestOpeningATargetThatIsNotThereFails(t *testing.T) {
	h := newHarness()

	_, err := h.storeOpener().OpenTarget(context.Background(), tenantID,
		shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"))
	if err == nil {
		t.Fatal("a target that does not exist was opened")
	}
}

// A target with no credential at all - a local directory - opens with none rather than failing.
func TestATargetWithoutACredentialOpensWithNone(t *testing.T) {
	h := newHarness()
	target := h.stored(t, true)
	h.targets.credential = sealedNothing()

	if _, err := h.storeOpener().OpenTarget(context.Background(), tenantID, target.ID); err != nil {
		t.Fatalf("opening a target with no credential: %v", err)
	}
	if len(h.opener.opened[0].Credentials) != 0 {
		t.Errorf("the adapter was handed %v", h.opener.opened[0].Credentials)
	}
}

// sealedNothing is what a target without a credential stores: nothing at all.
func sealedNothing() (empty crypto.Sealed) { return empty }
