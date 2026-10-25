// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package voice is the store of a workspace's voice settings (ADR-0093).
package voice

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/voice"
)

// Settings reads and writes a workspace's voice settings, aliases included.
type Settings interface {
	// Find answers the settings; a workspace that never set any answers the zero settings, off.
	Find(ctx context.Context) (domain.Settings, error)
	// Save replaces the settings and the aliases in one transaction.
	Save(ctx context.Context, settings domain.Settings, by shared.ID, at time.Time) error
}
