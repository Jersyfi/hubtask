// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the use
// case tests depend on, and it is where the shape's one deliberate awkwardness is written down:
// the credential is a method rather than a field.
type double struct{}

func (double) Insert(context.Context, domain.Target, crypto.Sealed) error { return nil }
func (double) List(context.Context) ([]domain.Target, error)              { return nil, nil }

func (double) Find(context.Context, shared.ID) (domain.Target, error) {
	return domain.Target{}, shared.ErrNotFound.WithDetail("backup.target_not_found")
}

func (double) Credential(context.Context, shared.ID) (crypto.Sealed, error) {
	return crypto.Sealed{}, nil
}

func (double) RecordTest(context.Context, shared.ID, time.Time, bool, string) error { return nil }

func (double) Coverage(context.Context) (Coverage, error) { return Coverage{}, nil }

var _ Targets = double{}

// The shape the whole "never returned by any read" requirement rests on: a target that comes back
// from a read has no field a credential could be in, and reading one is a call somebody had to
// make on purpose.
func TestACredentialIsNotAFieldOfATarget(t *testing.T) {
	target, err := (double{}).Find(t.Context(), "")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing target is reported as %v", err)
	}
	// The aggregate names the key the credential was sealed under - which says which key, and
	// nothing about the value - and has nowhere to put the value itself.
	if target.CredentialKeyID != "" {
		t.Error("the zero target claims a credential key")
	}

	sealed, err := (double{}).Credential(t.Context(), "")
	if err != nil {
		t.Fatalf("reading a credential: %v", err)
	}
	if !sealed.IsZero() {
		t.Error("a target with no credential answered one")
	}
}

// An empty listing and an empty count are answers rather than errors: a tenant that has configured
// nothing is the common case, and it is what the health surface asks about.
func TestTheEmptyAnswersAreNotErrors(t *testing.T) {
	targets, err := (double{}).List(t.Context())
	if err != nil || len(targets) != 0 {
		t.Fatalf("an empty shelf answered %v, %v", targets, err)
	}

	coverage, err := (double{}).Coverage(t.Context())
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if coverage.Configured != 0 || coverage.Unencrypted != 0 {
		t.Fatalf("an empty shelf counts %v", coverage)
	}
}
