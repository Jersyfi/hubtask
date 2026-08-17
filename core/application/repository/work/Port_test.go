// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Find(context.Context, shared.ID) (work.Container, error) {
	return work.Container{}, shared.ErrNotFound
}
func (double) LastOrderKey(context.Context, shared.ID) (string, error) { return "", nil }
func (double) Insert(context.Context, work.Container) error            { return nil }

var _ Containers = double{}

// Two answers the use case tells apart, so both have to be expressible: a container that is not
// there, and a level that has no containers yet.
func TestTheTwoEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing container is reported as %v", err)
	}

	key, err := (double{}).LastOrderKey(t.Context(), "")
	if err != nil {
		t.Fatalf("an empty level is reported as an error: %v", err)
	}
	if key != "" {
		t.Errorf("an empty level answered %q rather than nothing", key)
	}
}
