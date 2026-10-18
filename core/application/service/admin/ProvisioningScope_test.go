// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-INS-06 check 6, a case per control-plane operation: with `admin:provisioning` alone, the verbs
// that declare it pass the scope check and every other operation behind an `admin:*` scope is
// refused naming the scope it needs.
//
// Each descriptor's own handler is invoked as the catalogue builds it - without its collaborators -
// with an input of the declared shape. An admitted operation then reaches its first collaborator,
// and the panic of the missing one is the proof that it got past the scope; a refused one never gets
// that far.

const (
	platformTenant  = shared.ID("0192f000-0000-7000-8000-0000000000a1")
	platformAccount = shared.ID("0192f000-0000-7000-8000-0000000000a2")
)

func platformActor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorServiceAccount, TenantID: platformTenant, AccountID: platformAccount,
		AccountName: "Shop", Scopes: scopes,
	}
}

// shapedInput is a value of every declared field's kind, so that a handler's parsing is not what
// answers.
func shapedInput(descriptor usecase.Descriptor) usecase.Input {
	in := usecase.Input{}
	for _, field := range descriptor.Input {
		switch field.Kind {
		case usecase.KindID:
			in[field.Name] = string(platformTenant)
		case usecase.KindIDList:
			in[field.Name] = []any{string(platformTenant)}
		case usecase.KindBool:
			in[field.Name] = false
		case usecase.KindInt:
			in[field.Name] = 1
		case usecase.KindObject:
			in[field.Name] = map[string]any{}
		case usecase.KindList:
			in[field.Name] = []any{}
		default:
			// Most strings here name something by id; a moment is the other shape that is parsed
			// before the scope is asked.
			in[field.Name] = string(platformTenant)
			if strings.HasSuffix(field.Name, "_at") || strings.Contains(field.Name, "point") {
				in[field.Name] = "2026-10-17T00:00:00Z"
			}
		}
	}
	return in
}

// invoke runs the handler and answers its error, or errPastTheScope where it reached a collaborator
// the catalogue's copy does not have. A handler that validates its input before the scope may still
// refuse the shaped input's value; it is then asked again with none, so the answer is the scope's
// wherever the scope is what refuses.
func invoke(descriptor usecase.Descriptor, actor appshared.ActorContext) error {
	err := invokeWith(descriptor, actor, shapedInput(descriptor))
	if shared.AsError(err) != nil && errors.Is(err, shared.ErrValidation) {
		if second := invokeWith(descriptor, actor, usecase.Input{}); !errors.Is(second, shared.ErrValidation) {
			return second
		}
	}
	return err
}

func invokeWith(descriptor usecase.Descriptor, actor appshared.ActorContext, in usecase.Input) (err error) {
	defer func() {
		if recover() != nil {
			err = errPastTheScope
		}
	}()
	_, err = descriptor.Handler.Invoke(context.Background(), actor, in)
	return err
}

var errPastTheScope = errors.New("past the scope check")

func refusedFor(err error, scope string) bool {
	refusal := shared.AsError(err)
	return refusal != nil && refusal.DetailCode == "access.insufficient_scope" && refusal.Params["scope"] == scope
}

func TestTheProvisioningScopeIsAdmittedOrRefusedPerOperation(t *testing.T) {
	admitted, refused := 0, 0
	for _, descriptor := range catalogue.Descriptors() {
		if !strings.HasPrefix(descriptor.TokenScope, "admin:") {
			continue
		}
		t.Run(descriptor.Name, func(t *testing.T) {
			err := invoke(descriptor, platformActor("admin:provisioning"))
			if descriptor.AlsoScope == "admin:provisioning" {
				admitted++
				if refusedFor(err, descriptor.TokenScope) || refusedFor(err, "admin:provisioning") {
					t.Fatalf("the platform's verb refused the narrow scope: %v", err)
				}
				// And a credential with neither is told the narrowest scope that would do.
				if err := invoke(descriptor, platformActor("items:read")); !refusedFor(err, "admin:provisioning") {
					t.Errorf("without either scope it answered %v, want the refusal naming admin:provisioning", err)
				}
				return
			}
			refused++
			if !refusedFor(err, descriptor.TokenScope) {
				t.Fatalf("answered %v, want access.insufficient_scope naming %s", err, descriptor.TokenScope)
			}
		})
	}
	if admitted != 20 || refused == 0 {
		t.Errorf("%d operations admitted and %d refused; the platform's verbs are twenty", admitted, refused)
	}
}
