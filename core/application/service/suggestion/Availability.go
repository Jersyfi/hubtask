// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
)

// Availability answers "may this workspace's content be sent at all", which is the question
// ai-first.md §2 asks before every call and the one a caller has to be able to ask *before* it
// queues anything.
//
// It is the resolver's answer read as a yes or a no. The resolver already produces NoopAi for a
// workspace that configured nothing, chose NOOP, or has not consented - so "can this provider
// complete" is exactly the question, and asking it here means no second copy of the consent rule.
type Availability struct {
	Providers Providers
}

// CanSuggest reports whether asking would reach a model.
func (a Availability) CanSuggest(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	if a.Providers == nil {
		return false, nil
	}
	provider, err := a.Providers.For(ctx, actor)
	if err != nil {
		return false, err
	}
	return provider.Capabilities().Completion, nil
}
