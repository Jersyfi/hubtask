// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Preset answers the preset a key names as the actor may choose it, held to the installation:
// one of Hubtask's, the workspace's, or the actor's own. Another person's own preset is unknown,
// exactly like one that never existed (T-04).
func (s *layerReading) Preset(
	ctx context.Context, actor appshared.ActorContext, key domain.PresetKey,
) (domain.Preset, error) {
	bound, err := s.Bound(ctx)
	if err != nil {
		return domain.Preset{}, err
	}
	id, saved := key.SavedID()
	if !saved || s.resolver.Presets == nil {
		return domain.FindPreset(key, bound)
	}
	preset, err := s.resolver.Presets.Find(ctx, id)
	if errors.Is(err, shared.ErrNotFound) || (err == nil && !mayChoose(actor, preset)) {
		return domain.FindPreset(key, bound)
	}
	if err != nil {
		return domain.Preset{}, err
	}
	return preset.Offered(bound), nil
}

// mayChoose reports whether the actor is offered the saved preset: the workspace's are everyone's,
// an own one its owner's alone.
func mayChoose(actor appshared.ActorContext, preset domain.Preset) bool {
	return preset.Origin == domain.PresetOriginWorkspace ||
		(preset.Origin == domain.PresetOriginOwn && preset.OwnerID == actor.AccountID)
}

// presetFrom reads the preset an input names: one of Hubtask's by its key, or a saved one by its
// id - never both.
func presetFrom(in usecase.Input, keyName, idName string) (domain.PresetKey, error) {
	key, id := in.String(keyName), in.String(idName)
	if key != "" && id != "" {
		return "", shared.ErrValidation.WithDetail("presets.both_named").
			WithFields(shared.FieldError{Path: "/" + idName, Code: "presets.both_named"})
	}
	if id == "" {
		return domain.PresetKey(key), nil
	}
	parsed, err := in.ID(idName)
	if err != nil {
		return "", err
	}
	return domain.SavedPresetKey(parsed), nil
}

// presetIDField is the input naming a saved preset beside a field naming one of Hubtask's.
func presetIDField(name, beside string) usecase.Field {
	return usecase.Field{
		Name: name, Kind: usecase.KindID,
		Description: "A saved preset instead of " + beside + ": the workspace's, or the caller's own. " +
			"Naming both is refused as presets.both_named.",
	}
}

// keySplit answers a key as the contract carries it: one of Hubtask's under the key's name, a
// saved one's id under the id's name. Each absent one is nil.
func keySplit(key domain.PresetKey) (builtIn, savedID any) {
	if key == "" {
		return nil, nil
	}
	if id, saved := key.SavedID(); saved {
		return nil, id.String()
	}
	return string(key), nil
}

// The audit codes and target of the saved presets.
const (
	PresetSavedAction     audit.Action = "presets.saved"
	PresetChangedAction   audit.Action = "presets.changed"
	PresetWithdrawnAction audit.Action = "presets.withdrawn"
)

// authorizePreset asks the question a saved preset decides: a workspace preset is the workspace's
// shape, needing STRUCTURE at the workspace; an own one is self-service, like a person's own view,
// bound only by the token's scope.
func authorizePreset(
	ctx context.Context, authorizer Authorizer, actor appshared.ActorContext, origin domain.PresetOrigin,
	action audit.Action, target shared.ID,
) error {
	if origin != domain.PresetOriginWorkspace {
		return actor.RequireScope(containersWrite)
	}
	return authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: workspaceManageScope,
		TargetType: presetTarget,
		TargetID:   target,
	})
}

// presetAudit is the evidence of a write to a saved preset. The name is the person's content and
// stays out (rule 10); the origin says whose preset it was.
func presetAudit(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, preset domain.Preset,
	at time.Time, changes ...audit.Change,
) audit.Entry {
	return audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: presetTarget,
		TargetID:   preset.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(append([]audit.Change{{
			Field: "origin", Classification: audit.Open, To: string(preset.Origin),
		}}, changes...)...),
	}
}
