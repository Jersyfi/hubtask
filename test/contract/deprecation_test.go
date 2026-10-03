// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"strings"
	"testing"
	"time"
)

// SC-28: a deprecated field says so. Every member the contract marks deprecated carries the day it
// was and the major version it goes with - what the manifest lists and the header is set from.
func TestEveryDeprecatedFieldSaysWhenAndUntilWhen(t *testing.T) {
	spec := contractSpec(t)
	found := 0
	for name, schema := range spec.Components.Schemas {
		for field, property := range schema.Properties {
			if property == nil || !property.Deprecated {
				continue
			}
			found++
			if _, err := time.Parse(time.DateOnly, property.Since); err != nil {
				t.Errorf("%s.%s is deprecated without a day in x-deprecated-since (%q)", name, field, property.Since)
			}
			if !strings.HasPrefix(property.RemovedIn, "v") {
				t.Errorf("%s.%s is deprecated without the major version in x-removed-in (%q)", name, field, property.RemovedIn)
			}
		}
	}
	if found == 0 {
		t.Fatal("no deprecated field found - the reading of the specification is broken")
	}
}
