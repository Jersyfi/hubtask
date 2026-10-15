// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// Passkeys and security keys on the caller's own account (identity.md §23, UC-ID-22): listed with
// whether another can be added now, and added in two calls - a step-up and the options, then the
// browser's ceremony. The verifier is an adapter behind core/port/passkey; nothing here reads a byte
// of a ceremony.

const ListOwnPasskeysName = "ListOwnPasskeys"

// The reasons a passkey cannot be added now, as message codes: the list answers them for the profile
// to say, and starting answers the same code as its refusal (P-05).
const (
	codeLimitReached   = "auth.passkey_limit_reached"
	codeNoRelyingParty = "auth.passkey_unavailable"
	codeStepUpNoMethod = "auth.step_up_no_method"
	codePersonOnly     = "auth.passkey_person_only"
)

// PasskeyInstallation is where the installation is reached, from its public address: what a
// workspace's relying party is derived from (identity.md §23.3). Empty Host where no public address
// is configured, and then no passkey can be bound.
type PasskeyInstallation struct {
	Host   string
	Scheme string
	Port   string
	Multi  bool
}

// PasskeyWriter is what the passkey use cases share.
type PasskeyWriter struct {
	// Session carries the transaction, the clock, the randomness, the ids, the trail, the recovery
	// codes and the pending rows.
	Session    SessionWriter
	Passkeys   repository.PasskeyCredentials
	Verifier   passkey.Verifier
	Workspaces repository.Workspaces
	// Hosts is the workspace's hosts, the canonical one first. Optional: a workspace without a row
	// answers at the host its slug makes (TenantHost.go).
	Hosts        repository.TenantHosts
	Installation PasskeyInstallation
	// StepUp is the fresh proof adding asks for (identity.md §23.2). Nil refuses, as everywhere.
	StepUp stepup.Verifier
}

// relyingParty is a workspace's relying party with the name its options carry.
type relyingParty struct {
	passkey.RelyingParty
	Name string
}

// relyingParty answers the workspace's relying party, inside a transaction bound to it; false where
// the installation has no public address.
func (w PasskeyWriter) relyingParty(ctx context.Context) (relyingParty, bool, error) {
	workspace, err := w.Workspaces.Find(ctx)
	if err != nil {
		return relyingParty{}, false, err
	}
	host := domain.CanonicalHostOf(workspace.Slug, w.Installation.Host, w.Installation.Multi)
	if w.Installation.Host == "" {
		host = ""
	}
	if w.Hosts != nil {
		hosts, err := w.Hosts.List(ctx)
		if err != nil {
			return relyingParty{}, false, err
		}
		for _, candidate := range hosts {
			if candidate.Canonical && candidate.State == domain.HostActive {
				host = candidate.Host
				break
			}
		}
	}
	if host != "" && w.Session.Domains != nil {
		ascii, err := w.Session.Domains.ToASCII(host)
		if err != nil {
			return relyingParty{}, false, nil
		}
		host = ascii
	}
	rpID, origin, ok := domain.PasskeyRelyingParty(host, w.Installation.Scheme, w.Installation.Port)
	if !ok {
		return relyingParty{}, false, nil
	}
	return relyingParty{
		RelyingParty: passkey.RelyingParty{ID: rpID, Origin: origin}, Name: workspace.DisplayName,
	}, true, nil
}

// passkeyPerson admits a person acting for themselves, and nobody else: a machine holds tokens
// (identity.md §15.2), and an automation or an agent has no device to confirm on.
func passkeyPerson(actor appshared.ActorContext) error {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if actor.Kind != appshared.ActorUser {
		return shared.ErrForbidden.WithDetail(codePersonOnly)
	}
	return nil
}

// addability is whether another passkey can be added now, and where not, why: the one condition the
// list answers and starting enforces (P-05).
type addability struct {
	party  relyingParty
	held   []domain.PasskeyCredential
	reason string
}

func (a addability) possible() bool { return a.reason == "" }

// addable reads the account's credentials and the relying party, inside a transaction bound to the
// workspace, and decides everything but the step-up's methods, which the caller asks outside it.
func (w PasskeyWriter) addable(ctx context.Context, actor appshared.ActorContext) (addability, error) {
	account, err := w.Session.People.Find(ctx, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	if account.Kind != domain.AccountUser {
		return addability{}, shared.ErrForbidden.WithDetail(codePersonOnly)
	}
	held, err := w.Passkeys.ListOf(ctx, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	party, ok, err := w.relyingParty(ctx)
	if err != nil {
		return addability{}, err
	}
	result := addability{party: party, held: held}
	switch {
	case !ok:
		result.reason = codeNoRelyingParty
	case len(held) >= domain.MaxPasskeys:
		result.reason = codeLimitReached
	}
	return result, nil
}

// withMethods completes the decision with the step-up's methods: an account that can give no fresh
// proof cannot add one, and the button that would ask is not drawn.
func (w PasskeyWriter) withMethods(
	ctx context.Context, actor appshared.ActorContext, decided addability,
) (addability, error) {
	if !decided.possible() || w.StepUp == nil || !w.StepUp.Available() {
		if decided.possible() {
			decided.reason = codeStepUpNoMethod
		}
		return decided, nil
	}
	methods, err := w.StepUp.Methods(ctx, actor.TenantID, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	if len(methods) == 0 {
		decided.reason = codeStepUpNoMethod
	}
	return decided, nil
}

// OwnPasskeys is the list as the profile shows it.
type OwnPasskeys struct {
	Passkeys []OwnPasskey
	CanAdd   bool
	// CannotAddReason is the message code saying why not, where CanAdd is false.
	CannotAddReason string
	// RPID is the host the workspace's passkeys belong to, empty where there is none.
	RPID string
}

// OwnPasskey is one credential as its holder sees it: never its identifier, key or handle.
type OwnPasskey struct {
	ID         shared.ID
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

func ownPasskeyOf(credential domain.PasskeyCredential) OwnPasskey {
	return OwnPasskey{
		ID: credential.ID, Name: credential.Name,
		CreatedAt: credential.CreatedAt, LastUsedAt: credential.LastUsedAt,
	}
}

// ListOwnPasskeys answers the caller's passkeys and whether another can be added.
type ListOwnPasskeys struct{ Writer PasskeyWriter }

// Execute reads them.
func (h ListOwnPasskeys) Execute(ctx context.Context, actor appshared.ActorContext) (OwnPasskeys, error) {
	if err := passkeyPerson(actor); err != nil {
		return OwnPasskeys{}, err
	}
	w := h.Writer
	var decided addability
	err := w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		decided, err = w.addable(ctx, actor)
		return err
	})
	if err != nil {
		return OwnPasskeys{}, err
	}
	decided, err = w.withMethods(ctx, actor, decided)
	if err != nil {
		return OwnPasskeys{}, err
	}
	listed := OwnPasskeys{
		Passkeys: make([]OwnPasskey, 0, len(decided.held)),
		CanAdd:   decided.possible(), CannotAddReason: decided.reason, RPID: decided.party.ID,
	}
	for _, credential := range decided.held {
		listed.Passkeys = append(listed.Passkeys, ownPasskeyOf(credential))
	}
	return listed, nil
}

// OwnPasskeysReadAction names the read for the trail's refusals.
const OwnPasskeysReadAction audit.Action = "auth.own_passkeys_read" //nolint:gosec // G101: an audit code

// Descriptor is the catalogue entry.
func (h ListOwnPasskeys) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOwnPasskeysName,
		Summary: "Lists the passkeys and security keys of the caller's own account - each one's " +
			"name, when it was added and when it was last used, never its identifier or key - and " +
			"whether another can be added now, with the reason where not.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			// The caller's own records: nothing to refuse beyond the credential, nothing to record.
			Action: OwnPasskeysReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(listed.Passkeys))
			for _, each := range listed.Passkeys {
				data = append(data, ownPasskeyOutput(each))
			}
			out := usecase.Output{
				"data": data, "limit": domain.MaxPasskeys, "can_add": listed.CanAdd, "rp_id": listed.RPID,
			}
			if listed.CannotAddReason != "" {
				out["cannot_add_reason"] = listed.CannotAddReason
			}
			return out, nil
		}),
	}
}

func ownPasskeyOutput(each OwnPasskey) usecase.Output {
	out := usecase.Output{
		"id": each.ID.String(), "name": each.Name, "created_at": each.CreatedAt.UTC(),
	}
	if !each.LastUsedAt.IsZero() {
		out["last_used_at"] = each.LastUsedAt.UTC()
	}
	return out
}
