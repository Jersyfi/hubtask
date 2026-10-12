// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portai "github.com/Jersyfi/hubtask/core/port/ai"
)

// AiSelection is the answer to "which configuration may this workspace's AI call, if any" - the
// decision the resolver acts on, made here because which source the rule allows is the
// application's question and not an adapter's (ADR-0072 §2, §5). Refusal is `ai.unavailable` for
// nothing configured or consented, `ai.source_not_allowed` where the rule in force no longer allows
// what the workspace chose.
type AiSelection = portai.Selection

var _ portai.SourceSelector = AiRules{}

// Refusals the resolver answers with.
const (
	RefusalUnavailable = "ai.unavailable"
	RefusalNotAllowed  = "ai.source_not_allowed"
)

// Select decides, inside the caller's read-only transaction in the workspace's scope.
//
// The own configuration is read only where the choice is the own model, and the offered key only
// where the choice is an offered one: a workspace's key is not opened on the way to deciding not to
// use it, and neither is the installation's. Nothing here writes - narrowing stops calls and
// deletes nothing (ADR-0072 §5).
func (r AiRules) Select(ctx context.Context, tenantID shared.ID) (AiSelection, error) {
	own := r.Own
	state, err := r.Resolve(ctx, tenantID)
	if err != nil {
		return AiSelection{}, err
	}
	switch state.Choice.Source {
	case domain.AiSourceNone:
		return AiSelection{Refusal: RefusalUnavailable}, nil
	case domain.AiSourceOffered:
		if !state.Rule.Permits(domain.AiSourceOffered) || state.Choice.OfferedProviderID.IsZero() {
			return AiSelection{Refusal: RefusalNotAllowed}, nil
		}
		offered, sealed, err := r.Offered.FindWithKey(ctx, state.Choice.OfferedProviderID)
		if errors.Is(err, shared.ErrNotFound) {
			return AiSelection{Refusal: RefusalNotAllowed}, nil
		}
		if err != nil {
			return AiSelection{}, err
		}
		return AiSelection{
			Provider: offered, Sealed: sealed, Purpose: OfferedAiKeyPurpose(offered.ID),
			EmbeddingSpace: domain.OfferedEmbeddingSpace(offered.ID, offered.EmbeddingModel),
		}, nil
	default:
		if !state.Rule.Permits(domain.AiSourceOwn) {
			return AiSelection{Refusal: RefusalNotAllowed}, nil
		}
		if own == nil {
			// Rules wired without the own store can call nothing of the workspace's own.
			return AiSelection{Refusal: RefusalUnavailable}, nil
		}
		configured, sealed, err := own.FindWithKey(ctx)
		if errors.Is(err, shared.ErrNotFound) {
			return AiSelection{Refusal: RefusalUnavailable}, nil
		}
		if err != nil {
			return AiSelection{}, err
		}
		if configured.Kind == domain.AiNoop || !configured.ProcessingAllowed {
			return AiSelection{Refusal: RefusalUnavailable}, nil
		}
		if state.Rule.AdmitsOwn(configured.Jurisdiction) != nil {
			return AiSelection{Refusal: RefusalNotAllowed}, nil
		}
		return AiSelection{
			Provider: configured, Sealed: sealed, Purpose: AiKeyPurpose(tenantID),
			EmbeddingSpace: configured.EmbeddingModel,
		}, nil
	}
}
