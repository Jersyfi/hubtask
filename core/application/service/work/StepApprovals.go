// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// StepApprovals opens the approval a step waits for and withdraws one its entry left behind
// (UC-SVC-08); the service desk implements it, because who decides is its concern.
type StepApprovals interface {
	// Open opens the step's approval on the entry, inside the step's transaction.
	Open(ctx context.Context, actor appshared.ActorContext, item domain.WorkItem, collection domain.Container,
		workflow string, step domain.WorkflowStep) error
	// Moved withdraws an approval the entry waits on, because another step moved it.
	Moved(ctx context.Context, item domain.WorkItem) error
}

type approvedKey struct{}

// Approved marks a step as taken by its approval, which the step's own approval does not hold
// up again. Only the service desk's decision sets it, in the application layer.
func Approved(ctx context.Context) context.Context {
	return context.WithValue(ctx, approvedKey{}, true)
}

// TakenByApproval reports whether a step is taken by its approval.
func TakenByApproval(ctx context.Context) bool {
	approved, _ := ctx.Value(approvedKey{}).(bool)
	return approved
}
