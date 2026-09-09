// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func workspace(t *testing.T) identity.Workspace {
	t.Helper()
	id, err := shared.ParseID("018f0000-0000-7000-8000-000000000001")
	if err != nil {
		t.Fatalf("the fixture identifier does not parse: %v", err)
	}
	return identity.Workspace{
		Tenant: identity.Tenant{
			ID: id, Slug: "acme", DisplayName: "Acme", Status: identity.TenantActive,
			DefaultLocale: "en", DefaultTimeZone: "UTC",
		},
		Version: 3,
	}
}

func text(value string) *string { return &value }
func flag(value bool) *bool     { return &value }

// The patch is a merge-patch: an absent key moves nothing, a present one moves its field, and a
// present one carrying the value already held moves nothing either - which is what keeps a client
// that sends the whole form back from writing an audit entry per save.
func TestWithAppliesOnlyWhatMoved(t *testing.T) {
	cases := []struct {
		name   string
		change identity.WorkspaceChange
		want   []identity.FieldChange
	}{
		{"an empty patch changes nothing", identity.WorkspaceChange{}, nil},
		{
			"the name moves",
			identity.WorkspaceChange{DisplayName: text("Acme GmbH")},
			[]identity.FieldChange{{Field: "display_name", From: "Acme", To: "Acme GmbH"}},
		},
		{
			"the same name, spaced, does not",
			identity.WorkspaceChange{DisplayName: text("  Acme  ")},
			nil,
		},
		{
			"the enforcement switch moves",
			identity.WorkspaceChange{RequireAdminTotp: flag(true)},
			[]identity.FieldChange{{Field: "require_admin_totp", From: "false", To: "true"}},
		},
		{
			"switching off what is already off does not",
			identity.WorkspaceChange{RequireAdminTotp: flag(false)},
			nil,
		},
		{
			"three at once come back in a stable order",
			identity.WorkspaceChange{
				DisplayName: text("Acme GmbH"), DefaultTimeZone: text("Europe/Berlin"),
				DefaultLocale: text("de"),
			},
			[]identity.FieldChange{
				{Field: "default_locale", From: "en", To: "de"},
				{Field: "default_time_zone", From: "UTC", To: "Europe/Berlin"},
				{Field: "display_name", From: "Acme", To: "Acme GmbH"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed, moved, err := workspace(t).With(c.change)
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if len(moved) != len(c.want) {
				t.Fatalf("moved %v, want %v", moved, c.want)
			}
			for i, want := range c.want {
				if moved[i] != want {
					t.Errorf("change %d is %v, want %v", i, moved[i], want)
				}
			}
			// What the patch says it changed is what the answer carries.
			for _, change := range moved {
				switch change.Field {
				case "display_name":
					if changed.DisplayName != change.To {
						t.Errorf("display name %q, want %q", changed.DisplayName, change.To)
					}
				case "default_locale":
					if changed.DefaultLocale != change.To {
						t.Errorf("locale %q, want %q", changed.DefaultLocale, change.To)
					}
				case "default_time_zone":
					if changed.DefaultTimeZone != change.To {
						t.Errorf("zone %q, want %q", changed.DefaultTimeZone, change.To)
					}
				case "require_admin_totp":
					if changed.Settings.RequireAdminTotp != (change.To == "true") {
						t.Errorf("the enforcement switch did not follow its change")
					}
				}
			}
		})
	}
}

// A value the workspace cannot hold is refused as a field error, by the same rule provisioning
// uses - which is the reason the three validators are shared rather than written twice.
func TestWithRefusesWhatTheWorkspaceCannotHold(t *testing.T) {
	cases := []struct {
		name     string
		change   identity.WorkspaceChange
		wantCode string
		wantPath string
	}{
		{"an empty name", identity.WorkspaceChange{DisplayName: text("   ")},
			"admin.display_name_invalid", "/display_name"},
		{"a control character in the name", identity.WorkspaceChange{DisplayName: text("Acme\u0007")},
			"admin.display_name_invalid", "/display_name"},
		{"a locale that is not a tag", identity.WorkspaceChange{DefaultLocale: text("not a tag")},
			"admin.locale_invalid", "/default_locale"},
		{"an empty locale, which has no default here", identity.WorkspaceChange{DefaultLocale: text("")},
			"admin.locale_invalid", "/default_locale"},
		{"a zone nothing can load", identity.WorkspaceChange{DefaultTimeZone: text("Mars/Olympus")},
			"admin.time_zone_invalid", "/default_time_zone"},
		{"an empty zone, which has no default here", identity.WorkspaceChange{DefaultTimeZone: text("")},
			"admin.time_zone_invalid", "/default_time_zone"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := workspace(t).With(c.change)
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("error %v is not a domain error", err)
			}
			if domainErr.DetailCode != c.wantCode {
				t.Errorf("detail code %q, want %q", domainErr.DetailCode, c.wantCode)
			}
			if len(domainErr.Fields) != 1 || domainErr.Fields[0].Path != c.wantPath {
				t.Errorf("fields %v, want one at %q", domainErr.Fields, c.wantPath)
			}
		})
	}
}
