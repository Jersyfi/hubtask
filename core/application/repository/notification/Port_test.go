// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic - the doubles prove both interfaces can be implemented by a fake,
// which is what the notification tests depend on.
type recordDouble struct{}

func (recordDouble) Insert(context.Context, domain.Notification) (bool, error) { return true, nil }
func (recordDouble) Find(context.Context, shared.ID) (domain.Notification, error) {
	return domain.Notification{}, shared.ErrNotFound
}
func (recordDouble) Save(context.Context, domain.Notification) error { return nil }
func (recordDouble) DeleteExpired(context.Context, time.Time, int) (int, error) {
	return 0, nil
}
func (recordDouble) CountExpired(context.Context, time.Time, int) (int, error) { return 0, nil }

var _ Notifications = recordDouble{}

type preferenceDouble struct{}

func (preferenceDouble) Find(
	context.Context, shared.ID, domain.Category, domain.Channel,
) (domain.Preference, error) {
	return domain.Preference{}, shared.ErrNotFound
}
func (preferenceDouble) Save(context.Context, domain.Preference) error { return nil }

var _ Preferences = preferenceDouble{}

// The behavioural promise worth pinning at this level: a missing record is the shared not-found,
// so every caller's errors.Is check reads the same answer.
func TestAMissingRecordIsTheSharedNotFound(t *testing.T) {
	if _, err := (recordDouble{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing record is reported as %v", err)
	}
}

// And an account that has said nothing about being told is not-found rather than the default. The
// default is the domain's, and a repository that applied it would be a second place it is written
// down - which is how "on" and "off" come to disagree.
func TestSayingNothingIsNotFoundRatherThanTheDefault(t *testing.T) {
	preference, err := (preferenceDouble{}).Find(
		t.Context(), "", domain.CategoryComment, domain.ChannelEmail)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an account that has said nothing is reported as %v", err)
	}
	if preference.Enabled {
		t.Error("a not-found preference came back switched on - a caller ignoring the error " +
			"would read the default from the wrong place")
	}
}
