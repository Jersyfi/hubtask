// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// Scopes answers which token scope a use case declares.
//
// The suggestion job grants its actor exactly that scope, per call, and no other - which is the
// decision `automation.Scopes` already made for a rule, for a reason that holds here word for word:
// **a job presents no credential.** The scope bound exists to let a *token* be narrower than its
// owner, and there is no token by the time a job runs - the one that asked was checked when it
// asked, at `SuggestFromJumbleEntry`. Granting the use case's own scope is what makes the role the
// thing that decides, which is what "checked by the authoriser exactly as a person's would be"
// means (ADR-0005).
//
// It exists because it was missing. `AiSuggestion` built the asking person's actor with no scopes
// at all, so every read the job made was refused `access.insufficient_scope` - which means no
// suggestion could ever be produced against a running installation. Nothing caught it: the use case
// tests hand `Produce` an actor they built themselves, and the worker's own actor was never put in
// front of a real registry until J-16's end-to-end session asked for a suggestion and waited a
// minute for one that was never coming.
type Scopes interface {
	ForUseCase(name string) (string, bool)
}

// Fields answers the input names a use case declares.
//
// `Produce` reads it to narrow a proposal to what the use case that would apply it can take - see
// `payloadFrom`, and the suggestion nobody could accept that made it necessary.
type Fields interface {
	InputsOf(name string) ([]string, bool)
}

// ScopedCatalogue grants the scope of whatever it is about to invoke.
//
// A wrapper rather than a line at each call site, because there are five of them and a sixth added
// later would be a read that fails in a job and nowhere else - which is precisely the failure this
// type exists to have had once.
type ScopedCatalogue struct {
	Catalogue Catalogue
	Scopes    Scopes
}

var _ Catalogue = ScopedCatalogue{}

// Invoke calls the use case with an actor carrying its declared scope.
//
// The actor is otherwise untouched: the tenant, the account and the kind are the asking person's,
// so the permission question - which is the one that decides whether this suggestion may see the
// entry at all - is asked about them exactly as it would be for a request they made themselves.
func (c ScopedCatalogue) Invoke(
	ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if c.Scopes != nil {
		if scope, known := c.Scopes.ForUseCase(name); known && scope != "" {
			actor.Scopes = []string{scope}
		}
	}
	return c.Catalogue.Invoke(ctx, name, actor, in)
}
