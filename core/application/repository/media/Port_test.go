// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic - the doubles prove both interfaces can be implemented by a fake,
// which is what the use case tests depend on.
type objectDouble struct{}

func (objectDouble) Insert(context.Context, media.Object) error { return nil }
func (objectDouble) Find(context.Context, shared.ID) (media.Object, error) {
	return media.Object{}, shared.ErrNotFound
}
func (objectDouble) Seal(context.Context, media.Object) error             { return nil }
func (objectDouble) AdjustRefCount(context.Context, shared.ID, int) error { return nil }
func (objectDouble) MarkDeleted(context.Context, shared.ID, time.Time) (bool, error) {
	return false, nil
}
func (objectDouble) Recount(context.Context) error { return nil }
func (objectDouble) MarkOrphans(context.Context, time.Time, time.Time) (int, error) {
	return 0, nil
}
func (objectDouble) TakeOrphans(context.Context, time.Time, int) ([]Orphan, error) {
	return nil, nil
}
func (objectDouble) RemoveRows(context.Context, []shared.ID) (int, error) { return 0, nil }
func (objectDouble) ReferencingItems(context.Context, shared.ID) ([]ItemRef, error) {
	return nil, nil
}
func (objectDouble) ListForItem(context.Context, shared.ID, repository.Page) (ObjectPage, error) {
	return ObjectPage{}, nil
}

var _ Objects = objectDouble{}

type attachmentDouble struct{}

func (attachmentDouble) Add(context.Context, shared.ID, shared.ID) (bool, error) {
	return false, nil
}
func (attachmentDouble) Remove(context.Context, shared.ID, shared.ID) (bool, error) {
	return false, nil
}
func (attachmentDouble) MediaIDs(context.Context, shared.ID) ([]shared.ID, error) {
	return nil, nil
}

var _ Attachments = attachmentDouble{}

// The one behavioural promise worth pinning at this level: a missing object is the shared
// not-found, so every caller's errors.Is check reads the same answer.
func TestAMissingObjectIsTheSharedNotFound(t *testing.T) {
	if _, err := (objectDouble{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing object is reported as %v", err)
	}
}
