// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The register's bound on a personal access token, from the side of the route it then reaches
// (UC-INS-03 check 5): a token carrying the control plane's scope whose holder the register does
// not name keeps every other scope, and an administrative route says the register is the reason.
func TestATokenOutsideTheRegisterIsRefusedNamingTheRegister(t *testing.T) {
	cases := []struct {
		name     string
		register *operatorRegister
		want     string
		category shared.Category
		kept     bool
	}{
		{
			name:     "named in the register",
			register: &operatorRegister{accounts: map[shared.ID]bool{account: true}},
			kept:     true,
		},
		{
			name:     "not in the register",
			register: &operatorRegister{accounts: map[shared.ID]bool{}},
			want:     "access.operator_register_required",
			category: shared.CategoryForbidden,
		},
		{
			// Fail closed: a register that cannot be read must not leave the widest scope in place.
			name:     "the register cannot be read",
			register: &operatorRegister{unreadable: shared.ErrUnavailable.WithDetail("postgres.query_failed")},
			want:     "access.operator_register_unreadable",
			category: shared.CategoryUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, credential := mintCredential(t)
			credential.Token.Scopes = []string{"items:read", "admin:tenants"}
			handler := handlerFor(&tokens{credential: credential}, &unitOfWork{})
			handler.Operators = tc.register

			actor, err := handler.Execute(t.Context(), AuthenticateTokenCommand{
				Credential: raw, FallbackLocale: "en", FallbackTimeZone: "UTC",
			})
			if err != nil {
				t.Fatalf("authentication failed: %v", err)
			}
			if !actor.HasScope("items:read") {
				t.Errorf("the token lost a scope the register does not bound: %v", actor.Scopes)
			}
			if actor.HasScope("admin:tenants") != tc.kept {
				t.Fatalf("admin:tenants kept = %v, want %v", actor.HasScope("admin:tenants"), tc.kept)
			}

			err = actor.RequireScope("admin:tenants")
			if tc.kept {
				if err != nil {
					t.Fatalf("a registered holder was refused: %v", err)
				}
				return
			}
			var domainErr *shared.Error
			if !errors.As(err, &domainErr) || domainErr.DetailCode != tc.want ||
				domainErr.Category != tc.category {
				t.Fatalf("refusal = %v, want %s (%s)", err, tc.want, tc.category)
			}
			if domainErr.Params["scope"] != "admin:tenants" {
				t.Errorf("the refusal does not name the scope: %v", domainErr.Params)
			}
		})
	}
}

// A scope the credential never named is still the ordinary refusal: the register's sentence is for
// the token that asked for the control plane, not for every token that reaches it.
func TestAScopeNeverAskedForIsTheOrdinaryRefusal(t *testing.T) {
	raw, credential := mintCredential(t)
	handler := handlerFor(&tokens{credential: credential}, &unitOfWork{})
	handler.Operators = &operatorRegister{accounts: map[shared.ID]bool{}}

	actor, err := handler.Execute(t.Context(), AuthenticateTokenCommand{
		Credential: raw, FallbackLocale: "en", FallbackTimeZone: "UTC",
	})
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}
	var domainErr *shared.Error
	if err := actor.RequireScope("admin:tenants"); !errors.As(err, &domainErr) ||
		domainErr.DetailCode != "access.insufficient_scope" {
		t.Fatalf("refusal = %v, want access.insufficient_scope", err)
	}
}
