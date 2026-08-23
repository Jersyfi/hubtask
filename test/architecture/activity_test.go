// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"sort"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
)

// The acceptance criterion of B-11, in the shape the parity gate has: every mutating
// work-management use case in the catalogue either writes a step of an item's history or is on an
// explicit exemption list with a reason.
//
// It is a declaration check rather than a behaviour check, and deliberately so. Whether a use case
// that declares a verb actually writes the entry is proved where the writing is - by the service
// tests, and end to end by test/integration/activity_test.go. What no test at that level can catch
// is the use case that was written and never asked the question at all, which is exactly what a
// catalogue gate is for.

// workManagementTargets are the audit target types of the work-management context. A use case
// whose target is one of these is a use case about somebody's work, and therefore one this gate
// has an opinion about; identity, meta and the rest are not its business.
var workManagementTargets = map[string]bool{
	"item": true, "container": true, "bucket": true, "label": true, "trash": true,
	"comment": true,
}

// exemptUseCases is the list. It is pinned here as well as declared on the descriptor, so that
// exempting a use case takes two edits in two places rather than one line nobody reviews - the
// reason travels with the use case, and the decision travels through this gate.
var exemptUseCases = []string{
	"ArchiveContainer", "AutoAssignWorkItem", "CreateBucket", "CreateContainer", "CreateLabel",
	"DeleteBucket", "DeleteLabel", "EmptyTrash", "MoveContainer", "PurgeWorkItem",
	"RenameContainer", "ReorderBucket", "RestoreContainer", "TrashContainer",
	"UnarchiveContainer", "UpdateBucket", "UpdateContainerPolicies", "UpdateLabel",
}

func TestEveryMutatingWorkUseCaseWritesTheHistoryOrSaysWhyNot(t *testing.T) {
	var exempted []string

	for _, descriptor := range useCaseCatalogue(t).All() {
		if descriptor.ReadOnly || !workManagementTargets[descriptor.Audit.TargetType] {
			continue
		}

		t.Run(descriptor.Name, func(t *testing.T) {
			declaration := descriptor.Activity
			switch {
			case declaration.Verb != "" && declaration.Exempt != "":
				t.Errorf("%s both writes %s and explains why it writes nothing",
					descriptor.Name, declaration.Verb)
			case declaration.Verb != "":
				if !declaration.Verb.Valid() {
					t.Errorf("%s writes %q, which is not a verb the history knows",
						descriptor.Name, declaration.Verb)
				}
			case len(strings.Fields(declaration.Exempt)) >= 5:
				exempted = append(exempted, descriptor.Name)
			case declaration.Exempt != "":
				// A reason of three words is a label, not a reason. The next person to read it has
				// to be able to tell whether it still holds.
				t.Errorf("%s is exempt with %q - too short to be a reason",
					descriptor.Name, declaration.Exempt)
			default:
				t.Errorf("%s changes %s and declares neither a history verb nor a reason for "+
					"writing none", descriptor.Name, descriptor.Audit.TargetType)
			}
		})
	}

	sort.Strings(exempted)
	if strings.Join(exempted, ",") != strings.Join(exemptUseCases, ",") {
		t.Errorf("the exemption list has moved:\n  declared: %v\n  expected: %v\n"+
			"exempting a use case is a decision - make it here as well as on the descriptor",
			exempted, exemptUseCases)
	}
}

// Two use cases sharing a verb would render as the same sentence in a history that has to
// distinguish them - "archived" and "unarchived" are the pair that makes the point.
func TestNoTwoUseCasesShareAHistoryVerb(t *testing.T) {
	written := map[activity.Verb]string{}

	for _, descriptor := range useCaseCatalogue(t).All() {
		verb := descriptor.Activity.Verb
		if verb == "" {
			continue
		}
		if first, taken := written[verb]; taken {
			t.Errorf("%s and %s both write %s", first, descriptor.Name, verb)
			continue
		}
		written[verb] = descriptor.Name
	}
}

// The other direction: a verb the model declares and no use case writes is either a verb that
// arrived early or one whose use case was lost. Reported rather than failed, the way an unused
// message code is - a verb may legitimately be prepared before the use case that writes it lands.
func TestVerbsNothingWritesAreReported(t *testing.T) {
	written := map[activity.Verb]bool{}
	for _, descriptor := range useCaseCatalogue(t).All() {
		written[descriptor.Activity.Verb] = true
	}

	for _, verb := range activity.Verbs() {
		if !written[verb] {
			t.Logf("note: no use case writes %s", verb)
		}
	}
}

// A declaration on a use case the registry would refuse never reaches a running installation, so
// the gate proves the refusal happens rather than trusting the field.
func TestTheRegistryRefusesAnIncoherentDeclaration(t *testing.T) {
	sound := usecase.Descriptor{
		Name: "DoSomething", Summary: "Does something.", Handler: stubHandler{},
	}

	cases := map[string]usecase.ActivityDeclaration{
		"both a verb and a reason":         {Verb: activity.ItemCreated, Exempt: "because"},
		"a verb the history does not know": {Verb: "item.folded"},
	}

	for name, declaration := range cases {
		t.Run(name, func(t *testing.T) {
			descriptor := sound
			descriptor.Activity = declaration

			if _, err := usecase.NewRegistry(nil, descriptor); err == nil {
				t.Error("the registry accepted it")
			}
		})
	}
}

type stubHandler struct{}

func (stubHandler) Invoke(
	context.Context, appshared.ActorContext, usecase.Input,
) (usecase.Output, error) {
	return usecase.Output{}, nil
}
