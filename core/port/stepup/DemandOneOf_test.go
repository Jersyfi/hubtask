// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package stepup_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

type methodVerifier struct {
	method   stepup.Method
	held     []stepup.Method
	consumed int
}

func (v *methodVerifier) Available() bool { return true }
func (v *methodVerifier) Satisfied(context.Context, shared.ID, string) (bool, error) {
	return true, nil
}
func (v *methodVerifier) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return v.held, nil
}
func (v *methodVerifier) SatisfiedBy(context.Context, shared.ID, string) (stepup.Method, bool, error) {
	v.consumed++
	return v.method, true, nil
}

// UC-INS-19 check 5: a passkey does not confirm a change of address; the password does, and the
// demand names only the accepted methods the account holds.
func TestDemandOneOf(t *testing.T) {
	held := []stepup.Method{stepup.MethodPassword, stepup.MethodTotp, stepup.MethodPasskey}
	accepted := []stepup.Method{stepup.MethodPassword, stepup.MethodProvider}

	var refused *shared.Error
	err := stepup.DemandOneOf(context.Background(), &methodVerifier{held: held}, "t", "a", "", "change_address", accepted...)
	if !errors.As(err, &refused) || refused.DetailCode != stepup.CodeRequired ||
		refused.Params["methods"] != "PASSWORD" || refused.Params["purpose"] != "change_address" {
		t.Fatalf("no proof: %v %#v", err, refused)
	}

	passkey := &methodVerifier{held: held, method: stepup.MethodPasskey}
	err = stepup.DemandOneOf(context.Background(), passkey, "t", "a", "proof", "change_address", accepted...)
	if !errors.As(err, &refused) || refused.DetailCode != stepup.CodeMethodRefused || passkey.consumed != 1 {
		t.Fatalf("passkey: %v %#v", err, refused)
	}

	password := &methodVerifier{held: held, method: stepup.MethodPassword}
	if err := stepup.DemandOneOf(context.Background(), password, "t", "a", "proof", "", accepted...); err != nil {
		t.Fatalf("password: %v", err)
	}
	if err := stepup.DemandOneOf(context.Background(), nil, "t", "a", "proof", "", accepted...); err == nil {
		t.Fatal("a missing verifier let the act through")
	}
}
