// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// GoalSet is a goal period marked as one a template brought to the workspace (configuration.md
// §6), with what its block in the workspace's document names. The days are calendar days at
// midnight UTC.
type GoalSet struct {
	Key      string
	PeriodID shared.ID
	Name     string
	StartOn  time.Time
	EndOn    time.Time
}

// GoalSets keeps the marks in the transaction's workspace.
type GoalSets interface {
	// Mark marks the period under key; false where the workspace marks the key already.
	Mark(ctx context.Context, period shared.ID, key string, at time.Time) (bool, error)
	// List answers the workspace's marked periods, by key.
	List(ctx context.Context) ([]GoalSet, error)
}
