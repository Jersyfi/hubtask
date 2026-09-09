// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion_test

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The scope a suggestion job's reads are made with (J-16).
//
// This exists because it was missing: `AiSuggestion` built the asking person's actor with no scopes
// at all, so every read the job made was refused - which means no suggestion could ever be produced
// against a running installation. The use case tests could not see it, because they hand `Produce`
// an actor they built themselves.

type recordingCatalogue struct {
	acted []appshared.ActorContext
	names []string
}

func (c *recordingCatalogue) Invoke(
	_ context.Context, name string, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	c.names = append(c.names, name)
	c.acted = append(c.acted, actor)
	return usecase.Output{}, nil
}

type declaredScopes map[string]string

func (s declaredScopes) ForUseCase(name string) (string, bool) {
	scope, known := s[name]
	return scope, known
}

func jobActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	}
}

// Each call carries the scope its own use case declares, and only that one. A job presents no
// credential, so the scope bound has nothing to narrow - granting the use case's own scope is what
// leaves the role as the thing that decides.
func TestEachCallCarriesTheScopeItsUseCaseDeclares(t *testing.T) {
	inner := &recordingCatalogue{}
	scoped := suggestion.ScopedCatalogue{
		Catalogue: inner,
		Scopes:    declaredScopes{"GetWorkItem": "items:read", "AcceptSuggestion": "items:write"},
	}

	for _, name := range []string{"GetWorkItem", "AcceptSuggestion"} {
		if _, err := scoped.Invoke(t.Context(), name, jobActor(), usecase.Input{}); err != nil {
			t.Fatalf("invoking %s: %v", name, err)
		}
	}

	want := []string{"items:read", "items:write"}
	for i, actor := range inner.acted {
		if len(actor.Scopes) != 1 || actor.Scopes[0] != want[i] {
			t.Errorf("%s was called with %v, want exactly [%s]", inner.names[i], actor.Scopes, want[i])
		}
	}
}

// Everything else about the actor is the asking person's: the permission question - which decides
// whether this suggestion may see the entry at all - is asked about them, exactly as it would be
// for a request they made themselves.
func TestNothingButTheScopesIsChanged(t *testing.T) {
	inner := &recordingCatalogue{}
	scoped := suggestion.ScopedCatalogue{
		Catalogue: inner, Scopes: declaredScopes{"GetWorkItem": "items:read"},
	}

	asking := jobActor()
	if _, err := scoped.Invoke(t.Context(), "GetWorkItem", asking, usecase.Input{}); err != nil {
		t.Fatalf("invoking: %v", err)
	}

	acted := inner.acted[0]
	if acted.TenantID != asking.TenantID || acted.AccountID != asking.AccountID {
		t.Errorf("the job acted as %s/%s, want the person who asked", acted.TenantID, acted.AccountID)
	}
	if acted.Kind != asking.Kind {
		t.Errorf("the job acted as a %s", acted.Kind)
	}
	// And the caller's own actor is untouched: a wrapper that widened the value it was handed
	// would widen it for whatever ran next.
	if len(asking.Scopes) != 0 {
		t.Errorf("the caller's actor was widened: %v", asking.Scopes)
	}
}

// A use case the catalogue does not know, or one that declares no scope, is passed through
// unchanged rather than granted something invented.
func TestAUseCaseThatDeclaresNoScopeIsNotGrantedOne(t *testing.T) {
	inner := &recordingCatalogue{}
	scoped := suggestion.ScopedCatalogue{
		Catalogue: inner, Scopes: declaredScopes{"ReadCapabilities": ""},
	}

	for _, name := range []string{"ReadCapabilities", "SomethingNobodyRegistered"} {
		if _, err := scoped.Invoke(t.Context(), name, jobActor(), usecase.Input{}); err != nil {
			t.Fatalf("invoking %s: %v", name, err)
		}
	}
	for i, actor := range inner.acted {
		if len(actor.Scopes) != 0 {
			t.Errorf("%s was granted %v", inner.names[i], actor.Scopes)
		}
	}
}

// A build with no scope source behaves as it did before, which is what makes this safe to wire
// incrementally rather than all at once.
func TestWithoutAScopeSourceNothingIsGranted(t *testing.T) {
	inner := &recordingCatalogue{}
	scoped := suggestion.ScopedCatalogue{Catalogue: inner}

	if _, err := scoped.Invoke(t.Context(), "GetWorkItem", jobActor(), usecase.Input{}); err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if len(inner.acted[0].Scopes) != 0 {
		t.Errorf("a build with no scope source granted %v", inner.acted[0].Scopes)
	}
}

// A proposal is narrowed to what the use case that would apply it declares (J-16).
//
// `suggest-fields` proposes a title, notes, a due date and labels; a proposal about a jumble entry
// is applied by `ConvertJumbleEntry`, which declares `title` and not the other three - and the
// registry refuses an input a descriptor does not declare. Without this the record was produced,
// stored and listed, and every acceptance of it answered `validation_failed`.
func TestAProposalIsNarrowedToWhatItsApplierTakes(t *testing.T) {
	for name, c := range map[string]struct {
		allowed, applicable, want map[string]bool
	}{
		"the applier takes fewer": {
			allowed:    map[string]bool{"title": true, "notes": true, "labels": true},
			applicable: map[string]bool{"entry_id": true, "collection_id": true, "title": true},
			want:       map[string]bool{"title": true},
		},
		"the applier takes all of them": {
			allowed:    map[string]bool{"title": true, "notes": true},
			applicable: map[string]bool{"title": true, "notes": true, "due_date": true},
			want:       map[string]bool{"title": true, "notes": true},
		},
		// An applier this build cannot ask about narrows nothing rather than everything: a
		// suggestion with no fields is not stored, and answering "the model proposed nothing" for
		// a lookup that failed would be a lie about the model.
		"nothing is known about the applier": {
			allowed:    map[string]bool{"title": true, "notes": true},
			applicable: nil,
			want:       map[string]bool{"title": true, "notes": true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := suggestion.Narrowed(c.allowed, c.applicable)
			if len(got) != len(c.want) {
				t.Fatalf("narrowed to %v, want %v", got, c.want)
			}
			for field := range c.want {
				if !got[field] {
					t.Errorf("narrowed to %v, which drops %q", got, field)
				}
			}
		})
	}
}
