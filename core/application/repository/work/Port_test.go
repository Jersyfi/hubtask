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
func (double) List(context.Context, ContainerQuery) (ContainerPage, error) {
	return ContainerPage{}, nil
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

// The same for items. A separate double rather than one type implementing both ports: two
// repositories that happened to share a fake would let a use case be wired to the wrong one and
// still compile.
type itemDouble struct{}

func (itemDouble) Find(context.Context, shared.ID) (work.WorkItem, error) {
	return work.WorkItem{}, shared.ErrNotFound
}
func (itemDouble) List(context.Context, ItemQuery) (ItemPage, error) { return ItemPage{}, nil }
func (itemDouble) ChildCompletion(context.Context, shared.ID) (work.ChildCompletion, error) {
	return work.ChildCompletion{}, nil
}
func (itemDouble) SetCompletion(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) LastOrderKey(context.Context, shared.ID, shared.ID) (string, error) {
	return "", nil
}
func (itemDouble) Insert(context.Context, work.WorkItem) error { return nil }

var _ Items = itemDouble{}

// The sibling level is decided by two identifiers, not one: the same parent in another collection
// is not a sibling, and a port that took only the parent could not express the level directly
// under a collection at all.
func TestTheSiblingLevelOfAnItemNeedsBothIdentifiers(t *testing.T) {
	if _, err := (itemDouble{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing item is reported as %v", err)
	}

	key, err := (itemDouble{}).LastOrderKey(t.Context(), "collection", "")
	if err != nil {
		t.Fatalf("an empty level is reported as an error: %v", err)
	}
	if key != "" {
		t.Errorf("an empty level answered %q rather than nothing", key)
	}
}
