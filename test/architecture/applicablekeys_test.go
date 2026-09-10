// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"sort"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// The gate this issue exists for: **every key a suggestion may carry is one the acceptance can
// apply.**
//
// K-01's gate beside this one compares what a prompt asks a provider for with what the code keeps.
// It cannot see the second narrowing. A proposal is cut down again, at the moment it is produced,
// to what the use case that would apply it declares - because the registry refuses an input a
// descriptor does not declare - and until this test nothing compared those two.
//
// What that cost: `suggest-fields` asked for `notes`, a due date and labels about a jumble entry,
// `ConvertJumbleEntry` declares none of the three, and every jumble suggestion since J-06 paid a
// provider for three answers no code read. The allow list and the prompt agreed with each other
// the whole time, so K-01's gate stayed green, and the drop is silent by construction - the
// narrowing is a security filter and dropping a key is the *right* behaviour, which is why nothing
// looked twice.
//
// It reads the registry, the allow lists, the pair declaration and the acceptance table, and needs
// no database. It would have been red on the day J-16 landed, and on every day before it.
func TestEveryKeyASuggestionKeepsIsOneTheAcceptanceCanApply(t *testing.T) {
	registry := useCaseCatalogue(t)
	inputsOf := func(name string) ([]string, bool) {
		descriptor, known := registry.Lookup(name)
		if !known {
			return nil, false
		}
		declared := make([]string, 0, len(descriptor.Input))
		for _, field := range descriptor.Input {
			declared = append(declared, field.Name)
		}
		return declared, true
	}
	keys := suggestion.AnswerKeys()

	for promptID, targets := range suggestion.PromptTargets() {
		for target, kind := range targets {
			applier, walk, refusal, served := suggestion.AcceptedBy(target, kind)
			if !served {
				t.Errorf("%s is asked about a %s, and accepting a %s %s is not declared at all:\n"+
					"add the shape to `acceptance` in "+
					"core/application/service/suggestion/Suggestions.go, or stop asking the "+
					"question in `promptTargets`", promptID, target, kind, target)
				continue
			}
			// A shape nothing accepts applies no key at all, and one the acceptance walks has its
			// shape fixed by the walk rather than by a descriptor's inputs. Both are declared
			// rather than inferred, which is what makes them checkable at all.
			if refusal != "" || walk {
				continue
			}

			applicable, known := suggestion.Applicable(target, kind, inputsOf)
			if !known {
				t.Errorf("%s is applied by %q, which the catalogue does not carry",
					promptID, applier)
				continue
			}
			var dropped []string
			for _, key := range keys[promptID] {
				if !applicable[key] {
					dropped = append(dropped, key)
				}
			}
			if len(dropped) == 0 {
				continue
			}
			sort.Strings(dropped)
			t.Errorf("%s asks a provider for [%s] about a %s, and accepting one is %s, which "+
				"declares none of them and grows none of them.\n"+
				"Every one is a provider paid for an answer no code reads. Fix the half that is "+
				"wrong: the answer shape in infrastructure/ai/prompts/%s.v*.md and its allow list "+
				"in core/application/service/suggestion/Producing.go, the inputs %s declares, or "+
				"`grown` where the acceptance applies the key itself.",
				promptID, strings.Join(dropped, ", "), target, applier, promptID, applier)
		}
	}
}

// And the other half of the same rule, for the kinds no prompt produces.
//
// `SuggestDuplicates` builds its payload in code and asks no provider, so it passes through no
// allow list and no narrowing at all (K-04). A shape like that must not be one the acceptance
// merges into a use case's input: there would be nothing between what the payload holds and what
// the use case is called with. It is the one kind whose payload names *other entries*, which is
// where that matters most.
func TestAPayloadNoAllowListNarrowsIsNeverMergedIntoAnInput(t *testing.T) {
	produced := map[applierPair]bool{}
	for _, targets := range suggestion.PromptTargets() {
		for target, kind := range targets {
			produced[applierPair{target, kind}] = true
		}
	}

	for _, target := range []domain.TargetType{
		domain.TargetWorkItem, domain.TargetJumbleEntry, domain.TargetContainer,
	} {
		for _, kind := range []domain.Kind{
			domain.KindFields, domain.KindDecomposition, domain.KindDuplicates,
		} {
			if produced[applierPair{target, kind}] {
				continue
			}
			applier, walk, refusal, served := suggestion.AcceptedBy(target, kind)
			if !served || refusal != "" || walk {
				continue
			}
			t.Errorf("a %s %s reaches no prompt, so no allow list narrows its payload - and "+
				"accepting one hands that payload to %s. Declare a refusal, or a walk, or give "+
				"the shape a prompt whose allow list this gate can read.", kind, target, applier)
		}
	}
}

type applierPair struct {
	target domain.TargetType
	kind   domain.Kind
}

// A compile-time reminder that the gate reads the registry's own field names.
var _ = usecase.Field{}
