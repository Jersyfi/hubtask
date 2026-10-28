// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// A passkey or security key is a second factor (identity.md §23.2): the profile, turning the
// authenticator app off, the recovery codes and removing a key all read what an account holds
// through secondFactorOf, so no two of them can disagree.
//
// The sign-in's step, the step-up's methods, replacing the app and the provider's kept factor still
// read the app alone, and must until they can ask for a key: a factor counted where it cannot be
// asked for would let a person whom the rule covers in with the password alone (P-02).

// FactorKeys is what the second-factor rules read of an account's passkeys.
type FactorKeys interface {
	// CurrentKeys counts the account's passkeys of the workspace's current address - the ones that
	// can sign here. One made for another host counts as no factor.
	CurrentKeys(ctx context.Context, accountID shared.ID) (int, error)
	// LockAccount holds the account's row until the transaction ends, so two removals of the last
	// two factors are decided one after the other.
	LockAccount(ctx context.Context, accountID shared.ID) error
}

// PasskeyParty derives a workspace's relying party (identity.md §23.3).
type PasskeyParty struct {
	Workspaces repository.Workspaces
	// Hosts is the workspace's hosts, the canonical one first. Optional: a workspace without a row
	// answers at the host its slug makes (TenantHost.go).
	Hosts        repository.TenantHosts
	Installation PasskeyInstallation
	Domains      text.DomainEncoder
}

// of answers the workspace's relying party, inside a transaction bound to it; false where the
// installation has no public address.
func (p PasskeyParty) of(ctx context.Context) (relyingParty, bool, error) {
	workspace, err := p.Workspaces.Find(ctx)
	if err != nil {
		return relyingParty{}, false, err
	}
	host := domain.CanonicalHostOf(workspace.Slug, p.Installation.Host, p.Installation.Multi)
	if p.Installation.Host == "" {
		host = ""
	}
	if p.Hosts != nil {
		hosts, err := p.Hosts.List(ctx)
		if err != nil {
			return relyingParty{}, false, err
		}
		if main := domain.MainHostOf(hosts); main != "" {
			host = main
		}
	}
	if host != "" && p.Domains != nil {
		ascii, err := p.Domains.ToASCII(host)
		if err != nil {
			return relyingParty{}, false, nil
		}
		host = ascii
	}
	rpID, origin, ok := domain.PasskeyRelyingParty(host, p.Installation.Scheme, p.Installation.Port)
	if !ok {
		return relyingParty{}, false, nil
	}
	return relyingParty{
		RelyingParty: passkey.RelyingParty{ID: rpID, Origin: origin}, Name: workspace.DisplayName,
	}, true, nil
}

// PasskeyKeys is FactorKeys over the credential table and the relying party.
type PasskeyKeys struct {
	Passkeys repository.PasskeyCredentials
	Party    PasskeyParty
}

var _ FactorKeys = PasskeyKeys{}

// CurrentKeys counts the credentials that belong to the workspace's relying party. Without one -
// no public address - no key can sign, and none counts.
func (k PasskeyKeys) CurrentKeys(ctx context.Context, accountID shared.ID) (int, error) {
	party, ok, err := k.Party.of(ctx)
	if err != nil || !ok {
		return 0, err
	}
	held, err := k.Passkeys.ListOf(ctx, accountID)
	if err != nil {
		return 0, err
	}
	return currentOf(held, party.ID), nil
}

// LockAccount locks the account's row.
func (k PasskeyKeys) LockAccount(ctx context.Context, accountID shared.ID) error {
	return k.Passkeys.LockAccount(ctx, accountID)
}

func currentOf(held []domain.PasskeyCredential, rpID string) int {
	count := 0
	for _, credential := range held {
		if credential.BelongsTo(rpID) {
			count++
		}
	}
	return count
}

// secondFactorOf reads what the account holds, inside the caller's transaction. Keys nil holds none,
// the shape of an installation wired before passkeys. armed is the app; begun is an enrolment
// waiting for its first code, which is no factor.
func secondFactorOf(
	ctx context.Context, enrollments repository.MfaEnrollments, keys FactorKeys, accountID shared.ID,
) (factor domain.SecondFactor, begun bool, err error) {
	if enrollments != nil {
		enrollment, err := enrollments.Find(ctx, accountID)
		switch {
		case errors.Is(err, shared.ErrNotFound):
		case err != nil:
			return domain.SecondFactor{}, false, err
		case enrollment.ConfirmedAt.IsZero():
			begun = true
		default:
			factor.App = true
		}
	}
	if keys != nil {
		count, err := keys.CurrentKeys(ctx, accountID)
		if err != nil {
			return domain.SecondFactor{}, false, err
		}
		factor.Keys = count
	}
	return factor, begun, nil
}

// servesAt reports whether a request that arrived at host - the Host header, a port perhaps beside
// it - is at the relying party's very host. A browser runs a ceremony only for the host it is on.
func (p relyingParty) servesAt(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if at := strings.LastIndex(host, ":"); at >= 0 && !strings.Contains(host[at:], "]") {
		host = host[:at]
	}
	return p.ID != "" && strings.TrimSuffix(host, ".") == p.ID
}
