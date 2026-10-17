// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"encoding/base64"
	"strings"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// UC-ID-22 check 8: the setup during sign-in offers a passkey or security key beside the app.

type enrollmentFixture struct {
	*keyFactorFixture
	told     *toldOf
	enrolled *usecase.Registry
}

// newEnrollmentFixture is Bert, whose workspace demands a factor of everyone and who holds none.
func newEnrollmentFixture(t *testing.T) *enrollmentFixture {
	t.Helper()
	f := &enrollmentFixture{keyFactorFixture: newKeyFactorFixture(t, domain.MfaForEveryone), told: &toldOf{}}
	f.keys.held = nil
	writer := PasskeyWriter{
		Session: f.session.writer, Passkeys: f.keys, Verifier: f.verifier,
		Workspaces: f.session.writer.KeyProofs.Party.Workspaces, Installation: f.session.writer.KeyProofs.Party.Installation,
		WayIn: f.told,
	}
	registry, err := usecase.NewRegistry(nil,
		StartPasskeyEnrollment{Writer: writer}.Descriptor(), CompletePasskeyEnrollment{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.enrolled = registry
	return f
}

func (f *enrollmentFixture) enrollStep(t *testing.T) string {
	t.Helper()
	out, err := f.password(t, workspaceHost)
	if err != nil || out["passkey_setup"] != true {
		t.Fatalf("no setup with a key offered: %v, %v", out, err)
	}
	return out.String("pending_token")
}

func (f *enrollmentFixture) options(t *testing.T, pending, host string) (usecase.Output, error) {
	t.Helper()
	return f.enrolled.Invoke(t.Context(), StartPasskeyEnrollmentName, appshared.ActorContext{},
		usecase.Input{"pending_token": pending, "host": host})
}

func (f *enrollmentFixture) complete(t *testing.T, pending, challenge string) (usecase.Output, error) {
	t.Helper()
	return f.enrolled.Invoke(t.Context(), CompletePasskeyEnrollmentName, appshared.ActorContext{}, usecase.Input{
		"pending_token": pending, "name": "Desk key",
		"raw_id": base64.RawURLEncoding.EncodeToString(bertsKey), "client_data_json": challenge,
		"attestation_object": "AQ", "transports": []any{"usb"},
	})
}

func TestASetupDuringSignInKeepsTheKeyAndOpensTheSession(t *testing.T) {
	f := newEnrollmentFixture(t)
	pending := f.enrollStep(t)
	options, err := f.options(t, pending, workspaceHost)
	if err != nil {
		t.Fatalf("the options were refused: %v", err)
	}
	publicKey := options["public_key"].(usecase.Output)
	if publicKey["rp"].(usecase.Output).String("id") != workspaceHost {
		t.Errorf("options %v", publicKey)
	}
	issued := f.proofs.issued
	if len(issued) != 1 || issued[0].challenge.Purpose != domain.PendingPasskeyEnroll ||
		len(issued[0].challenge.UserHandle) != domain.PasskeyUserHandleBytes {
		t.Fatalf("issued %+v, want a setup's challenge with its handle", issued)
	}

	out, err := f.complete(t, pending, publicKey.String("challenge"))
	if err != nil {
		t.Fatalf("the setup was refused: %v", err)
	}
	if len(out["recovery_codes"].([]any)) != domain.RecoveryCodeCount {
		t.Errorf("codes %v, want ten for the first factor", out["recovery_codes"])
	}
	if out["tokens"] == nil {
		t.Fatalf("no session: %v", out)
	}
	if held := f.keys.held; len(held) != 1 || held[0].RPID != workspaceHost {
		t.Errorf("keys %+v", held)
	}
	if opened := f.session.sessions.inserted; len(opened) != 1 || opened[0].SignedInWith != domain.SignedInWithPasswordPasskey {
		t.Errorf("sessions %+v", opened)
	}
	f.told.only(t, domain.WayInPasskeyAdded)
	if !containsAction(auditActions(f.session.audit.entries), PasskeyAddedAction) {
		t.Error("the new key was not audited")
	}
	// The ENROLL credential is spent: it sets up nothing more.
	if _, err := f.options(t, pending, workspaceHost); err == nil {
		t.Error("a spent ENROLL credential was taken again")
	}
}

// Only where a browser can make the key, and only with the setup's own credential.
func TestTheSetupsKeyIsRefusedElsewhere(t *testing.T) {
	f := newEnrollmentFixture(t)
	pending := f.enrollStep(t)
	if _, err := f.options(t, pending, installationHost); err == nil || !strings.Contains(err.Error(), codeNoRelyingParty) {
		t.Errorf("another host answered %v", err)
	}

	withKey := newKeyFactorFixture(t, domain.MfaForEveryone)
	step := withKey.step(t)
	f2 := newEnrollmentFixture(t)
	if _, err := f2.options(t, step.String("pending_token"), workspaceHost); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_challenge_failed") {
		t.Errorf("a factor step's credential answered %v", err)
	}
	if len(f.proofs.issued) != 0 {
		t.Error("a challenge was issued where no key can be made")
	}
}

// A factor that appeared meanwhile ends the setup: the ENROLL credential was minted for an account
// that held none.
func TestASetupIsRefusedWhereAFactorAppearedMeanwhile(t *testing.T) {
	f := newEnrollmentFixture(t)
	pending := f.enrollStep(t)
	options, err := f.options(t, pending, workspaceHost)
	if err != nil {
		t.Fatal(err)
	}
	f.keys.held = append(f.keys.held, f.bert)
	if _, err := f.complete(t, pending, options["public_key"].(usecase.Output).String("challenge")); err == nil ||
		!strings.Contains(err.Error(), "auth.mfa_already_armed") {
		t.Errorf("answered %v, want the factor already held", err)
	}
	if len(f.session.sessions.inserted) != 0 {
		t.Error("a session was opened")
	}
}
