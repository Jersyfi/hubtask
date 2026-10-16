// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/plan"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RemoveOutcome is what removing a plan came to.
type RemoveOutcome string

const (
	PlanRemoved  RemoveOutcome = "REMOVED"
	PlanInUse    RemoveOutcome = "IN_USE"
	PlanNotFound RemoveOutcome = "NOT_FOUND"
)

// Plans are the installation's plans (multi-tenancy.md §4.2). No method takes a tenant: `plan`
// carries no policy, like `instance_setting`, and what bounds a write is the control plane's use
// case behind `admin:tenants` and the register.
type Plans interface {
	// List answers every plan not removed, oldest first, with what each decides.
	List(ctx context.Context) ([]plan.Plan, error)

	// Find answers one plan not removed by its short name, or an error wrapping
	// shared.ErrNotFound.
	Find(ctx context.Context, name string) (plan.Plan, error)

	// Workspaces answers how many workspaces are on each plan, by short name. It reads through
	// the control plane's enumerator, so it runs under the installation's read scope.
	Workspaces(ctx context.Context) (map[string]int64, error)

	// Insert writes a new plan with its values. A short name taken, also by a removed plan, is a
	// conflict error with code `admin.plan_name_taken`.
	Insert(ctx context.Context, created plan.Plan, by shared.ID) error

	// Replace writes a plan's display name and its values whole, raising its version. `expected`
	// nil writes whatever stands; otherwise false means the stored version is another one, or the
	// plan was removed - somebody acted first.
	Replace(ctx context.Context, changed plan.Plan, expected *int, by shared.ID, now time.Time) (bool, error)

	// Remove ends a plan unless a workspace is on it; its short name stays taken.
	Remove(ctx context.Context, id shared.ID, by shared.ID, now time.Time) (RemoveOutcome, error)
}
