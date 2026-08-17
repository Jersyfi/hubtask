// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) List(context.Context) ([]work.CapabilityProfile, error) { return nil, nil }

var _ CapabilityProfiles = double{}

// A profile list that came back empty is not the same as one that could not be read: the use case
// answers a manifest with no item types rather than an error, and a caller has to be able to tell
// the two apart.
func TestAnEmptyListIsNotAnError(t *testing.T) {
	profiles, err := double{}.List(t.Context())
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("profiles = %v", profiles)
	}
}
