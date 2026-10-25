// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package voice

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
)

type double struct{ saved domain.Settings }

func (d *double) Find(context.Context) (domain.Settings, error) { return d.saved, nil }
func (d *double) Save(_ context.Context, settings domain.Settings, _ shared.ID, _ time.Time) error {
	d.saved = settings
	return nil
}

var _ Settings = (*double)(nil)

// A workspace that never set anything reads as off, with nothing named.
func TestTheZeroSettingsAreOff(t *testing.T) {
	store := &double{}
	settings, err := store.Find(context.Background())
	if err != nil || settings.Enabled || len(settings.Aliases) != 0 {
		t.Errorf("the zero settings are %+v, %v", settings, err)
	}
}
