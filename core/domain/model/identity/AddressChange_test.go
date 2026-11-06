// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-19 check 6: a lasting way in is a password, a sign-in name or a provider, plus an app factor
// or recovery codes where a factor is required; a passkey is none.
func TestWaysHeldLacking(t *testing.T) {
	cases := []struct {
		name string
		held identity.WaysHeld
		want string
	}{
		{"a password", identity.WaysHeld{Password: true}, ""},
		{"a sign-in name", identity.WaysHeld{SignInName: true}, ""},
		{"a provider", identity.WaysHeld{Provider: true}, ""},
		{"a passkey only", identity.WaysHeld{Passkeys: true}, identity.LackingWayIn},
		{"nothing", identity.WaysHeld{}, identity.LackingWayIn},
		{"a passkey as the required factor", identity.WaysHeld{
			Password: true, Passkeys: true, FactorRequired: true}, identity.LackingFactor},
		{"a password without the required factor", identity.WaysHeld{
			Password: true, FactorRequired: true}, identity.LackingFactor},
		{"the app as the required factor", identity.WaysHeld{
			Password: true, App: true, FactorRequired: true}, ""},
		{"recovery codes as the required factor", identity.WaysHeld{
			Provider: true, RecoveryCodes: true, FactorRequired: true}, ""},
		{"a factor nobody requires", identity.WaysHeld{Password: true, Passkeys: true}, ""},
		{"the app without a way in", identity.WaysHeld{App: true, RecoveryCodes: true}, identity.LackingWayIn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.held.Lacking(); got != tc.want {
				t.Fatalf("Lacking() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRefuseLockoutNamesWhoseAndWhat(t *testing.T) {
	var refused *shared.Error
	if !errors.As(identity.RefuseLockout(true, identity.LackingWayIn), &refused) ||
		refused.DetailCode != "admin.address_lockout_self" || refused.Params["reason"] != "way_in" {
		t.Fatalf("self: %#v", refused)
	}
	if !errors.As(identity.RefuseLockout(false, identity.LackingFactor), &refused) ||
		refused.DetailCode != "admin.address_lockout_owner" || refused.Params["reason"] != "factor" {
		t.Fatalf("owner: %#v", refused)
	}
}

// UC-INS-19 check 2 and UC-INS-20 check 4: the new address is refused at the field as provisioning
// refuses one.
func TestNewAddress(t *testing.T) {
	current := identity.TenantAddress{Slug: "meier", Host: "meier.tasks.example.net"}
	cases := []struct {
		name string
		in   identity.NewTenantAddressInput
		want identity.TenantAddress
		code string
		path string
	}{
		{name: "a new short name", in: identity.NewTenantAddressInput{RawSlug: " Familie-Meier "},
			want: identity.TenantAddress{Slug: "familie-meier", Host: "familie-meier.tasks.example.net"}},
		{name: "an invalid short name", in: identity.NewTenantAddressInput{RawSlug: "-x"},
			code: "admin.slug_invalid", path: "/slug"},
		{name: "a short name the product reserves", in: identity.NewTenantAddressInput{RawSlug: "admin"},
			code: "admin.slug_reserved", path: "/slug"},
		{name: "a short name the operator reserves",
			in:   identity.NewTenantAddressInput{RawSlug: "billing", Reserved: []string{"billing"}},
			code: "admin.slug_reserved", path: "/slug"},
		{name: "no short name", in: identity.NewTenantAddressInput{}, code: "admin.slug_required", path: "/slug"},
		{name: "a host with subdomains", in: identity.NewTenantAddressInput{RawSlug: "x", RawHost: "a.b.c"},
			code: "admin.host_not_assigned", path: "/host"},
		{name: "the same address", in: identity.NewTenantAddressInput{RawSlug: "meier"},
			code: "admin.address_unchanged", path: "/slug"},
		{name: "the same name under a changed installation host",
			in:   identity.NewTenantAddressInput{RawSlug: "meier", Current: identity.TenantAddress{Slug: "meier", Host: "meier.old.example.net"}},
			want: identity.TenantAddress{Slug: "meier", Host: "meier.tasks.example.net"}},
		{name: "a new assigned host", in: identity.NewTenantAddressInput{Assigned: true, RawHost: "Tasks-HR.corp.example"},
			want: identity.TenantAddress{Slug: "meier", Host: "tasks-hr.corp.example"}},
		{name: "the installation's own host", in: identity.NewTenantAddressInput{Assigned: true, RawHost: "tasks.example.net"},
			code: "admin.host_installation", path: "/host"},
		{name: "an invalid host", in: identity.NewTenantAddressInput{Assigned: true, RawHost: "nohost"},
			code: "admin.host_invalid", path: "/host"},
		{name: "a short name with assigned hosts", in: identity.NewTenantAddressInput{Assigned: true, RawHost: "a.example", RawSlug: "x"},
			code: "admin.slug_not_changed_here", path: "/slug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			in.InstallationHost = "tasks.example.net"
			if in.Current == (identity.TenantAddress{}) {
				in.Current = current
			}
			got, err := identity.NewTenantAddress(in)
			if tc.code == "" {
				if err != nil || got != tc.want {
					t.Fatalf("NewTenantAddress() = %+v, %v; want %+v", got, err, tc.want)
				}
				return
			}
			var refused *shared.Error
			if !errors.As(err, &refused) || refused.DetailCode != tc.code {
				t.Fatalf("NewTenantAddress() error = %v, want %s", err, tc.code)
			}
			if len(refused.Fields) != 1 || refused.Fields[0].Path != tc.path {
				t.Fatalf("fields = %+v, want one at %s", refused.Fields, tc.path)
			}
		})
	}
}
