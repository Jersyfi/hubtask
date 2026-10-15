// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ScopeLevel is how far down a subscription listens: the whole workspace, one hub - its collections
// included - or one collection (automation.md §3.1), as a rule's scope does.
type ScopeLevel string

const (
	ScopeTenant     ScopeLevel = "TENANT"
	ScopeHub        ScopeLevel = "HUB"
	ScopeCollection ScopeLevel = "COLLECTION"
)

// WebhookScope is where a subscription listens. The zero value is the whole workspace.
type WebhookScope struct {
	Level ScopeLevel
	ID    shared.ID
}

// IsWorkspace reports whether the scope is the whole workspace - also the zero value.
func (s WebhookScope) IsWorkspace() bool { return s.Level == "" || s.Level == ScopeTenant }

// Path is the scope as the authoriser reads it; a collection is completed with its hub there.
func (s WebhookScope) Path() []identity.Scope {
	switch s.Level {
	case ScopeHub:
		return []identity.Scope{identity.TenantScope(), identity.HubScope(s.ID)}
	case ScopeCollection:
		return []identity.Scope{identity.TenantScope(), identity.CollectionScope(s.ID)}
	default:
		return []identity.Scope{identity.TenantScope()}
	}
}

// ValidWebhookScope checks that a scope names what its level needs. An empty level is the whole
// workspace, which is what a subscription without a scope has always been.
func ValidWebhookScope(scope WebhookScope) (WebhookScope, error) {
	switch scope.Level {
	case "", ScopeTenant:
		if !scope.ID.IsZero() {
			// Refused rather than ignored: a caller that named a container and the workspace means
			// one of the two.
			return WebhookScope{}, scopeError("/scope/id", "webhooks.scope_id_not_allowed")
		}
		return WebhookScope{Level: ScopeTenant}, nil
	case ScopeHub, ScopeCollection:
		if scope.ID.IsZero() {
			return WebhookScope{}, scopeError("/scope/id", "webhooks.scope_id_required")
		}
		return scope, nil
	default:
		return WebhookScope{}, scopeError("/scope/type", "webhooks.scope_type_unknown")
	}
}

func scopeError(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}
