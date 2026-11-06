// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package stepup is the seam a destructive act asks for a second, stronger proof of identity
// through (identity.md §16, backup-restore.md §8.3, security.md §5).
//
// The proof is a fresh re-authentication on the current session, consumed by the one privileged
// action it is presented to, and the refusal every demanding operation answers without one lives
// here, so that two operations cannot describe the same demand differently.
//
// Where no proof can be given, the act is refused rather than let through with the confirmation
// skipped: a confirmation that is structurally impossible to give is a stronger position than one
// that is skipped, because the refusal is visible and names its own reason.
package stepup

import (
	"context"
	"slices"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CodeRequired is the demand every privileged operation surfaces without a satisfied step-up: 403,
// naming the accepted methods, so a client knows to ask the person rather than to retry
// (identity.md §16.2).
const CodeRequired = "auth.step_up_required"

// Method is one way a person proves themselves afresh, as `POST /auth/step-up` takes it.
type Method string

const (
	// MethodPassword is the proof every account with a password can give.
	MethodPassword Method = "PASSWORD"
	// MethodTotp is the authenticator's code, offered only where a factor is armed.
	MethodTotp Method = "TOTP"
	// MethodRecovery is a recovery code, offered where a factor is armed and a code is left
	// (ADR-0075 §1). The step-up consumes it.
	MethodRecovery Method = "RECOVERY"
	// MethodPasskey is a passkey or security key, offered where the account holds one that belongs to
	// the workspace's relying party.
	MethodPasskey Method = "PASSKEY"
	// MethodProvider is a fresh sign-in at the provider the account is connected to, offered where
	// that provider is switched on for the account's workspace (ADR-0075 §2).
	MethodProvider Method = "PROVIDER"
)

// Required is the demand itself, minted in exactly one place, naming the methods this account can
// answer with - space-separated, in the order given, the way the contract describes the parameter.
// A demand that names TOTP to an account without a factor sends the person to a prompt for a code
// they cannot produce, which is why the list is the account's and not the endpoint's.
func Required(methods ...Method) error {
	if len(methods) == 0 {
		methods = []Method{MethodPassword}
	}
	return requiredOf(methods)
}

// requiredOf is the demand naming exactly the methods given - none at all for an account that
// holds neither a password nor a factor (one that signs in only through a provider, UC-ID-05 check
// 5). Naming the password there would send the person to a field they cannot fill.
func requiredOf(methods []Method) *shared.Error {
	names := make([]string, 0, len(methods))
	for _, method := range methods {
		names = append(names, string(method))
	}
	return shared.ErrForbidden.
		WithDetail(CodeRequired).
		WithParams(map[string]string{"methods": strings.Join(names, " ")})
}

// Demand is the check every privileged operation runs: a wired verifier, a presented token, a
// satisfied proof - or the one refusal. A nil or unavailable verifier refuses rather than permits,
// because a destructive mode permitted by omission is the failure this seam exists against; it
// names both methods, because nothing can look the account's up.
func Demand(
	ctx context.Context, verifier Verifier, tenantID, accountID shared.ID, token string,
) error {
	if verifier == nil || !verifier.Available() {
		return Required(MethodPassword, MethodTotp)
	}
	if token == "" {
		return refuse(ctx, verifier, tenantID, accountID)
	}
	satisfied, err := verifier.Satisfied(ctx, accountID, token)
	if err != nil {
		return err
	}
	if !satisfied {
		return refuse(ctx, verifier, tenantID, accountID)
	}
	return nil
}

// refuse is the demand with the account's own methods in it - and, where one of them is PROVIDER,
// the provider's name, which is what lets a prompt say whom the person confirms with.
func refuse(ctx context.Context, verifier Verifier, tenantID, accountID shared.ID) error {
	methods, err := verifier.Methods(ctx, tenantID, accountID)
	if err != nil {
		return err
	}
	demand := requiredOf(methods)
	namer, names := verifier.(ProviderNamer)
	if !names || !slices.Contains(methods, MethodProvider) {
		return demand
	}
	name, err := namer.ProviderName(ctx, tenantID, accountID)
	if err != nil {
		return err
	}
	return demand.WithParams(map[string]string{
		"methods": demand.Params["methods"], "provider": name,
	})
}

// ProviderNamer is what a verifier that offers PROVIDER answers beside the methods: the name of the
// provider the proof goes to, as its sign-in button shows it. Asked only when PROVIDER is named.
type ProviderNamer interface {
	ProviderName(ctx context.Context, tenantID, accountID shared.ID) (string, error)
}

// Verifier judges the proof.
type Verifier interface {
	// Available reports whether this installation can ask anybody for a step-up at all.
	//
	// Separate from Satisfied, because the two failures are different problems with different
	// fixes: "you did not prove it" is something the caller can act on, and "nothing here can
	// prove it" is something only the operator can. A single boolean would collapse them into one
	// message that is wrong for one of the two.
	Available() bool

	// Satisfied reports whether the token proves a fresh, stronger authentication of this
	// account. It is asked only when Available says yes.
	//
	// The token is opaque here on purpose: what proves a step-up is an authentication decision,
	// and the application layer's business is whether one was made rather than how.
	Satisfied(ctx context.Context, accountID shared.ID, token string) (bool, error)

	// Methods answers which proofs this account can give, for the refusal to name: the password
	// where it holds one, the code where a factor is armed - possibly neither. Asked only when a
	// demand is about to refuse.
	Methods(ctx context.Context, tenantID, accountID shared.ID) ([]Method, error)
}

// CodeMethodRefused is the refusal of a proof given by a method the act does not accept - a passkey
// for a change of address, whose relying party is the address that ends (multi-tenancy.md §3.1). The
// proof is spent; the refusal names the methods that are accepted, as the demand does.
const CodeMethodRefused = "auth.step_up_method_refused"

// MethodVerifier is a verifier that also answers which method gave the proof it consumed.
type MethodVerifier interface {
	Verifier
	SatisfiedBy(ctx context.Context, accountID shared.ID, token string) (Method, bool, error)
}

// DemandOneOf is Demand for an act that accepts only some methods. The demand names the account's
// methods among them, and `purpose` beside, so that a prompt can say why the others are not offered;
// a proof by another method is refused with CodeMethodRefused. A verifier that cannot say which
// method proved it refuses, Demand's reasoning: the restriction must not lapse by omission.
func DemandOneOf(
	ctx context.Context, verifier Verifier, tenantID, accountID shared.ID, token, purpose string,
	accepted ...Method,
) error {
	byMethod, ok := verifier.(MethodVerifier)
	if verifier == nil || !ok || !verifier.Available() {
		return withPurpose(requiredOf(accepted), purpose)
	}
	if token != "" {
		method, satisfied, err := byMethod.SatisfiedBy(ctx, accountID, token)
		if err != nil {
			return err
		}
		if satisfied && slices.Contains(accepted, method) {
			return nil
		}
		if satisfied {
			refusal, err := refuseAmong(ctx, verifier, tenantID, accountID, purpose, accepted)
			if err != nil {
				return err
			}
			return refusal.WithDetail(CodeMethodRefused)
		}
	}
	return refuseAmongErr(ctx, verifier, tenantID, accountID, purpose, accepted)
}

func refuseAmongErr(
	ctx context.Context, verifier Verifier, tenantID, accountID shared.ID, purpose string,
	accepted []Method,
) error {
	refusal, err := refuseAmong(ctx, verifier, tenantID, accountID, purpose, accepted)
	if err != nil {
		return err
	}
	return refusal
}

// refuseAmong is refuse with the account's methods narrowed to the accepted ones.
func refuseAmong(
	ctx context.Context, verifier Verifier, tenantID, accountID shared.ID, purpose string,
	accepted []Method,
) (*shared.Error, error) {
	held, err := verifier.Methods(ctx, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	methods := make([]Method, 0, len(held))
	for _, method := range held {
		if slices.Contains(accepted, method) {
			methods = append(methods, method)
		}
	}
	demand := withPurpose(requiredOf(methods), purpose)
	namer, names := verifier.(ProviderNamer)
	if !names || !slices.Contains(methods, MethodProvider) {
		return demand, nil
	}
	name, err := namer.ProviderName(ctx, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	params := map[string]string{"provider": name}
	for key, value := range demand.Params {
		params[key] = value
	}
	return demand.WithParams(params), nil
}

func withPurpose(demand *shared.Error, purpose string) *shared.Error {
	if purpose == "" {
		return demand
	}
	params := map[string]string{"purpose": purpose}
	for key, value := range demand.Params {
		params[key] = value
	}
	return demand.WithParams(params)
}
