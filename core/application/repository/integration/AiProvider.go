// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// AiProviders is the workspace's configured AI provider.
//
// Reading the configuration and reading its key are two methods, deliberately, and for the reason
// IdentityProviders splits the same pair: the ordinary read is what a person and an auditor get and
// cannot spill the key because the key is not in what it answers, while the adapter that makes the
// call asks for the envelope by name - which makes opening it a decision somebody wrote down rather
// than a field that happened to be in a struct.
type AiProviders interface {
	// Upsert sets the configuration whole and answers it as stored. A workspace has one provider,
	// so this is a write with no identifier: the row's key is the workspace.
	//
	// A nil `sealed` means "no key", which is a real configuration - a local model usually needs
	// none - and clears whatever envelope was stored. To keep a stored key untouched, use
	// UpsertKeepingKey: "store nothing" and "keep what is there" are different intentions and one
	// argument cannot carry both.
	Upsert(ctx context.Context, provider integration.AiProvider, sealed *crypto.Sealed, now time.Time) (integration.AiProvider, error)

	// UpsertKeepingKey is the same write with the stored envelope left where it is, for a caller
	// who changed a model and sent no key.
	UpsertKeepingKey(ctx context.Context, provider integration.AiProvider, now time.Time) (integration.AiProvider, error)

	// Find answers the configuration without its key, or an error wrapping shared.ErrNotFound when
	// the workspace has configured none.
	Find(ctx context.Context) (integration.AiProvider, error)

	// FindWithKey answers it with the sealed API key, for the adapter that makes the call and for
	// nothing else. The envelope is nil where the provider needs no key.
	FindWithKey(ctx context.Context) (integration.AiProvider, *crypto.Sealed, error)

	// Delete removes the configuration and its sealed key. False is "there was none", which is not
	// an error - a caller asking for it to be gone got what they asked for.
	Delete(ctx context.Context) (bool, error)

	// RewrapKey moves the envelope to the active key without touching the configuration, and
	// answers whether a row moved. The version deliberately does not rise: an operator rotating
	// the installation's keys has not changed anybody's provider (ADR-0045).
	RewrapKey(ctx context.Context, sealed crypto.Sealed, expectedKeyID string) (bool, error)
}

// OfferedAiProviders is the models the installation offers to every workspace (ADR-0072 §1).
//
// The reads run in any scope: the read policy admits these rows to every workspace, because the
// adapter of a workspace that chose one needs it for the call. The writes succeed only in the
// installation's own scope, where the installation_write policy admits them. What a workspace is
// *answered* of an offered model is narrowed by the use case, never by this port.
type OfferedAiProviders interface {
	// List answers every offered model, without its key, ordered by name.
	List(ctx context.Context) ([]integration.AiProvider, error)

	// Find answers one, or an error wrapping shared.ErrNotFound with `ai.offered_model_unknown`.
	Find(ctx context.Context, id shared.ID) (integration.AiProvider, error)

	// FindWithKey answers one with its sealed key, for the adapter that makes the call.
	FindWithKey(ctx context.Context, id shared.ID) (integration.AiProvider, *crypto.Sealed, error)

	// Insert offers a new model. A nil `sealed` stores no key.
	Insert(ctx context.Context, offered integration.AiProvider, sealed *crypto.Sealed, now time.Time) (integration.AiProvider, error)

	// Update sets an offered model whole, replacing its key (nil clears it). Not found when there
	// is no such offered model.
	Update(ctx context.Context, offered integration.AiProvider, sealed *crypto.Sealed, now time.Time) (integration.AiProvider, error)

	// UpdateKeepingKey is the same with the stored envelope left where it is.
	UpdateKeepingKey(ctx context.Context, offered integration.AiProvider, now time.Time) (integration.AiProvider, error)

	// Delete withdraws an offered model. Every workspace's choice of it then points at nothing.
	// False is "there was none".
	Delete(ctx context.Context, id shared.ID) (bool, error)
}

// AiChoices is the workspace's choice of AI source. No method takes a tenant: the workspace is the
// row's key, under row level security.
type AiChoices interface {
	// Find answers the stored choice, or the default (the workspace's own model) where none was
	// ever made - never not found, because "never chose" is a choice with a meaning.
	Find(ctx context.Context, tenantID shared.ID) (integration.AiChoice, error)

	// Save stores the choice whole and answers it as stored, with its version risen.
	Save(ctx context.Context, choice integration.AiChoice) (integration.AiChoice, error)
}
