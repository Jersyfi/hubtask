// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// AiProviders is the workspace's configured AI provider (J-02).
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
