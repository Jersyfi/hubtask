// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

type unreadableLevel struct{ identityrepo.InstanceSettings }

func (unreadableLevel) Read(context.Context) (identityrepo.InstanceLevel, error) {
	return identityrepo.InstanceLevel{}, errors.New("the database is away")
}

// The health report names a multi-tenant installation without an accessibility statement, and
// never a private one, one that set it, or one whose level could not be read.
func TestTheHealthReportNamesAProviderWithoutAnAccessibilityStatement(t *testing.T) {
	withLink := newInstanceStore()
	withLink.level.Legal.Links = identity.LegalLinks{AccessibilityURL: "https://provider.example/a11y"}
	withoutLink := newInstanceStore()
	withoutLink.level.Legal.Links = identity.LegalLinks{ImprintURL: "https://provider.example/imprint"}

	for _, tc := range []struct {
		name     string
		tenancy  env.TenancyMode
		settings identityrepo.InstanceSettings
		want     bool
	}{
		{"a provider without one", env.TenancyMulti, withoutLink, true},
		{"a provider with one", env.TenancyMulti, withLink, false},
		{"a private installation without one", env.TenancySingle, withoutLink, false},
		{"a level that cannot be read", env.TenancyMulti, unreadableLevel{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			warnings := AccessibilityWarnings{
				Tenancy: tc.tenancy, Settings: tc.settings, UnitOfWork: &unitOfWork{},
			}.Warnings(t.Context())
			if got := len(warnings) == 1 && warnings[0].Code == "config.accessibility_statement_missing"; got != tc.want {
				t.Errorf("warnings %+v, want the hint %v", warnings, tc.want)
			}
			if tc.want && warnings[0].Severity != "info" {
				t.Errorf("the hint is %q, want info: an installation may owe no statement", warnings[0].Severity)
			}
		})
	}
}
