// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// UC-ID-21 check 6: a fresh proof with a passkey or security key, through the registry.

func newKeyStepUpFixture(t *testing.T) (*keyFactorFixture, *usecase.Registry) {
	t.Helper()
	f := attachKeys(stepUpFixture(now))
	registry, err := usecase.NewRegistry(nil,
		StartPasskeyStepUp{Writer: f.session.writer}.Descriptor(), StepUp{Writer: f.session.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return f, registry
}

func keyStepUpOptions(t *testing.T, registry *usecase.Registry, actor appshared.ActorContext, host string) (usecase.Output, error) {
	t.Helper()
	return registry.Invoke(t.Context(), StartPasskeyStepUpName, actor, usecase.Input{"host": host})
}

func keyStepUp(t *testing.T, registry *usecase.Registry, challenge string, key []byte) (usecase.Output, error) {
	t.Helper()
	return registry.Invoke(t.Context(), StepUpName, signedInActor(), usecase.Input{
		"raw_id": base64.RawURLEncoding.EncodeToString(key), "client_data_json": challenge,
		"authenticator_data": "AQ", "signature": "AQ",
	})
}

func TestAKeyProvesTheCallerAfresh(t *testing.T) {
	f, registry := newKeyStepUpFixture(t)
	options, err := keyStepUpOptions(t, registry, signedInActor(), workspaceHost)
	if err != nil {
		t.Fatalf("the options were refused: %v", err)
	}
	publicKey := options["public_key"].(usecase.Output)
	if publicKey.String("rpId") != workspaceHost || len(publicKey["allowCredentials"].([]any)) != 1 {
		t.Errorf("options %v", publicKey)
	}
	issued := f.proofs.issued
	if len(issued) != 1 || issued[0].challenge.Purpose != domain.PendingPasskeyStepUp ||
		issued[0].challenge.SessionID != sessionRowID {
		t.Fatalf("issued %+v, want a fresh proof's challenge bound to the session", issued)
	}
	grant, err := keyStepUp(t, registry, publicKey.String("challenge"), bertsKey)
	if err != nil {
		t.Fatalf("a key without the person verified was refused: %v", err)
	}
	if grant.String("method") != string(domain.StepUpPasskey) || grant.String("step_up_token") == "" {
		t.Errorf("grant %v", grant)
	}
	labelled := false
	for _, entry := range f.session.audit.entries {
		if entry.Action == StepUpAction && strings.Contains(fmt.Sprint(entry.Changes), "PASSKEY") {
			labelled = true
		}
	}
	if !labelled {
		t.Error("the proof was not audited as a key's")
	}
}

// A key that does not prove the account is the step-up's refusal, counted on its own subject; a
// challenge issued to another session, or to the sign-in's step, proves nothing.
func TestARefusedKeyIsTheStepUpsRefusal(t *testing.T) {
	f, registry := newKeyStepUpFixture(t)
	options, err := keyStepUpOptions(t, registry, signedInActor(), workspaceHost)
	if err != nil {
		t.Fatal(err)
	}
	challenge := options["public_key"].(usecase.Output).String("challenge")
	if _, err := keyStepUp(t, registry, challenge, strangeKey); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("another account's key answered %v", err)
	}
	if f.session.attempts.standing[stepUpSubject(account)].Failures != 1 {
		t.Errorf("not counted on the step-up's subject: %+v", f.session.attempts.standing)
	}

	f.proofs.issued[0].challenge.SessionID = refreshRowID
	if _, err := keyStepUp(t, registry, challenge, bertsKey); err == nil {
		t.Error("another session's challenge proved this one")
	}
	f.proofs.issued[0].challenge.SessionID = sessionRowID
	f.proofs.issued[0].challenge.Purpose = domain.PendingPasskeyFactor
	if _, err := keyStepUp(t, registry, challenge, bertsKey); err == nil {
		t.Error("a sign-in step's challenge proved a fresh proof")
	}
}

// The options: only a session, only a key that can sign at the host asked from.
func TestTheStepUpsKeyOptionsAreRefusedWhereNoKeyCanSign(t *testing.T) {
	f, registry := newKeyStepUpFixture(t)
	if _, err := keyStepUpOptions(t, registry, signedInActor(), installationHost); err == nil ||
		!strings.Contains(err.Error(), codeNoRelyingParty) {
		t.Errorf("another host answered %v", err)
	}
	token := signedInActor()
	token.TokenID = refreshRowID
	if _, err := keyStepUpOptions(t, registry, token, workspaceHost); !errors.Is(err, shared.ErrForbidden) ||
		!strings.Contains(err.Error(), "auth.step_up_session_required") {
		t.Errorf("a token answered %v", err)
	}
	f.keys.held = nil
	if _, err := keyStepUpOptions(t, registry, signedInActor(), workspaceHost); err == nil ||
		!strings.Contains(err.Error(), codeNoRelyingParty) {
		t.Errorf("no key answered %v", err)
	}
	if len(f.proofs.issued) != 0 {
		t.Error("a challenge was issued where none can be answered")
	}
}

// The methods, in order: the password, the app, the key, the recovery code - the code wherever a
// factor is held and one is left.
func TestTheStepUpNamesTheKeyAmongTheAccountsMethods(t *testing.T) {
	f, _ := newKeyStepUpFixture(t)
	f.session.writer.Recovery.(*recoveryStore).codes = map[string]bool{"one": true}
	methods, err := StepUpVerifier{Writer: f.session.writer}.Methods(t.Context(), tenant, account)
	if err != nil {
		t.Fatal(err)
	}
	want := []stepup.Method{stepup.MethodPassword, stepup.MethodPasskey, stepup.MethodRecovery}
	if !slices.Equal(methods, want) {
		t.Errorf("methods %v, want %v", methods, want)
	}
	enrolled(t, f.session)
	methods, err = StepUpVerifier{Writer: f.session.writer}.Methods(t.Context(), tenant, account)
	if err != nil {
		t.Fatal(err)
	}
	want = []stepup.Method{stepup.MethodPassword, stepup.MethodTotp, stepup.MethodPasskey, stepup.MethodRecovery}
	if !slices.Equal(methods, want) {
		t.Errorf("methods %v, want %v", methods, want)
	}
}

// An operator raises their session on a key's fresh proof.
func TestAnOperatorRaisesTheirSessionOnAKeysProof(t *testing.T) {
	f, registry := newKeyStepUpFixture(t)
	options, err := keyStepUpOptions(t, registry, signedInActor(), workspaceHost)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := keyStepUp(t, registry, options["public_key"].(usecase.Output).String("challenge"), bertsKey)
	if err != nil {
		t.Fatal(err)
	}
	register := &operatorRegister{accounts: map[shared.ID]bool{account: true}}
	elevation, err := ElevateSession{
		Writer: f.session.writer, Operators: register,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}.Execute(t.Context(), signedInActor(), grant.String("step_up_token"))
	if err != nil || elevation.Until.IsZero() {
		t.Errorf("the elevation on a key's proof: (%+v, %v)", elevation, err)
	}
}
