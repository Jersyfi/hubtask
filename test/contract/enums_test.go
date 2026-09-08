// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"slices"
	"testing"

	catalogue "github.com/Jersyfi/hubtask/core/application/catalogue"
	"github.com/Jersyfi/hubtask/core/application/usecase"
)

// A closed set is written twice: once in api/openapi.yaml, where it is the contract, and once in
// the use case descriptor, where it is what the registry enforces before any handler runs. Nothing
// made the two agree, and the way they fail is silent: the descriptor refuses the value first, so
// the domain rule written for that exact value never runs and its purpose-written message code is
// unreachable. What the caller gets instead is a generic `usecase.field_not_in_enum`, which reads
// like ordinary input validation rather than like a defect (issue #427).
//
// The check is deliberately one-directional. A contract value the descriptor does not accept is
// the defect above. The opposite - a descriptor enum where the contract closes nothing, or a
// contract enum where the descriptor accepts anything - is not: a descriptor that declares no enum
// hands the value to the domain, which is how a named refusal such as `views.sharing_unknown` gets
// to speak at all.
//
// The comparison is by name. A descriptor field and a body property or query parameter that share
// a name are the same input - that is how every controller maps them.
func TestNoDescriptorEnumShadowsTheContract(t *testing.T) {
	spec := contractSpec(t)
	operations, err := spec.Operations()
	if err != nil {
		t.Fatalf("reading the specification's operations: %v", err)
	}

	registry, err := usecase.NewRegistry(nil, catalogue.Descriptors()...)
	if err != nil {
		t.Fatalf("the catalogue is not buildable: %v", err)
	}

	for _, descriptor := range registry.All() {
		t.Run(descriptor.Name, func(t *testing.T) {
			operation, ok := operations[descriptor.RESTOperation()]
			if !ok {
				// Parity is another gate's subject; this one has nothing to compare against.
				t.Skipf("the specification declares no operation %q", descriptor.RESTOperation())
			}
			declared, err := spec.InputEnums(operation)
			if err != nil {
				t.Fatalf("reading the declared enums: %v", err)
			}

			for _, field := range descriptor.Input {
				if len(field.Enum) == 0 {
					continue
				}
				for _, value := range declared[field.Name] {
					if !slices.Contains(field.Enum, value) {
						t.Errorf("the contract declares %q for %q and the descriptor accepts only "+
							"%v: the registry refuses that value before any rule written for it "+
							"can run", value, field.Name, field.Enum)
					}
				}
			}
		})
	}
}
