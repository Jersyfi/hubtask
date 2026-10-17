// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package security

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/catalogue"
	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	identity "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// The purchase platform's credential as a gate (T-26): the narrow scope reaches the verbs
// UC-INS-06 check 6 lists and no other, and the one widening it brings - a deletion without a
// step-up - stays a registered service account's.

const (
	provisioningScope = "admin:provisioning"
	operatorScope     = "admin:tenants"
)

// platformVerbs is UC-INS-06 check 6, as use cases. Written down here on purpose, against the
// descriptors: the scope is derived from them, so the gate is what notices an eleventh use case
// quietly opened to a platform's token.
var platformVerbs = []string{
	"ListTenants", "MoveTenantPlan", "ProvisionTenant", "QueryTenants", "ReadTenant",
	"RequestTenantDeletion", "ResumeTenant", "SuspendTenant", "UpdateTenant", "UpdateTenantQuotas",
}

func TestSGTheProvisioningScopeAdmitsExactlyThePlatformsVerbs(t *testing.T) {
	var admitted []string
	for _, descriptor := range catalogue.Descriptors() {
		if descriptor.TokenScope == provisioningScope {
			t.Errorf("%s demands the narrow scope alone; it is a subset of %s, never its own door",
				descriptor.Name, operatorScope)
		}
		if descriptor.AlsoScope == "" {
			continue
		}
		if descriptor.AlsoScope != provisioningScope || descriptor.TokenScope != operatorScope {
			t.Errorf("%s is admitted by %q beside %q; only the control plane has a narrower scope",
				descriptor.Name, descriptor.AlsoScope, descriptor.TokenScope)
		}
		admitted = append(admitted, descriptor.Name)
	}
	slices.Sort(admitted)
	if !slices.Equal(admitted, platformVerbs) {
		t.Errorf("the narrow scope admits %v, want exactly %v (UC-INS-06 check 6)", admitted, platformVerbs)
	}
	if !slices.Contains(catalogue.Scopes(), provisioningScope) {
		t.Error("the narrow scope is not in the catalogue, so no token can be minted with it")
	}
	if slices.Contains(catalogue.SessionScopes(), provisioningScope) {
		t.Error("a session carries the narrow scope; the control plane is entered by a minted credential")
	}
}

// The step-up exemption, walked across the actors that could claim it. Only the service account the
// register names by its own row gets through without a proof.
func TestSGOnlyARegisteredServiceAccountRequestsADeletionWithoutAStepUp(t *testing.T) {
	const (
		registered = shared.ID("0192f000-0000-7000-8000-0000000000e1")
		ownerOnly  = shared.ID("0192f000-0000-7000-8000-0000000000e2")
	)
	cases := []struct {
		name     string
		kind     appshared.ActorKind
		account  shared.ID
		register registerDouble
		want     string
	}{
		{"a person in the register", appshared.ActorUser, registered, registerDouble{}, stepup.CodeRequired},
		{"an agent in the register", appshared.ActorAIAgent, registered, registerDouble{}, stepup.CodeRequired},
		// The double cannot tell this from "not registered"; the register's own answer for the
		// empty register's owner is test/integration's TestTheRegisterNamesAServiceAccountByItsOwnRow.
		{"a service account only the empty register's owner", appshared.ActorServiceAccount, ownerOnly,
			registerDouble{}, stepup.CodeRequired},
		{"a service account while the register is unreadable", appshared.ActorServiceAccount, registered,
			registerDouble{broken: true}, ""},
		{"a registered service account", appshared.ActorServiceAccount, registered, registerDouble{}, "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.register.rows = []shared.ID{registered}
			deletion := adminservice.RequestTenantDeletion{
				Tenants: &leavingTenant{}, Journal: discardJournal{}, Automations: noAutomations{},
				Holds: noHolds{}, Jobs: discardJobs{}, StepUp: noProof{}, Register: tc.register,
				Audit: discardAudit{}, UnitOfWork: passThrough{}, Clock: clock.Fixed(at),
				IDs: fixedIDs{}, Tenancy: env.TenancyMulti,
			}
			actor := appshared.ActorContext{
				Kind: tc.kind, TenantID: tenantID, AccountID: tc.account, Scopes: []string{provisioningScope},
			}
			_, err := deletion.Execute(t.Context(), actor, adminservice.RequestTenantDeletionCommand{
				TenantID: hubID, Confirmation: "Acme GmbH",
			})
			switch tc.want {
			case "none":
				if err != nil {
					t.Fatalf("the registered platform was refused: %v", err)
				}
			case "":
				if err == nil {
					t.Fatal("a register that cannot be read waived the step-up")
				}
			default:
				if shared.AsError(err).DetailCode != tc.want {
					t.Fatalf("answered %v, want %s", err, tc.want)
				}
			}
		})
	}
}

// registerDouble answers the register's service-account rows; an empty-register owner is not one.
type registerDouble struct {
	rows   []shared.ID
	broken bool
}

func (r registerDouble) RegisteredServiceAccount(_ context.Context, accountID shared.ID) (bool, error) {
	if r.broken {
		return false, shared.ErrUnavailable.WithDetail("postgres.query_failed")
	}
	return slices.Contains(r.rows, accountID), nil
}

type leavingTenant struct{ adminrepo.Tenants }

func (leavingTenant) Find(context.Context) (adminrepo.TenantRecord, error) {
	return adminrepo.TenantRecord{ID: hubID, Slug: "acme", DisplayName: "Acme GmbH",
		Status: identity.TenantActive}, nil
}

func (leavingTenant) RequestDeletion(context.Context, time.Time, time.Time) (bool, error) {
	return true, nil
}

type (
	discardJournal struct{ adminrepo.Journal }
	noAutomations  struct{}
	noHolds        struct{}
	discardJobs    struct{}
	noProof        struct{ stepup.Verifier }
	discardAudit   struct{}
	passThrough    struct{}
	fixedIDs       struct{}
)

func (discardJournal) Record(context.Context, adminrepo.InstanceEvent) error { return nil }
func (noAutomations) DisableAll(context.Context, time.Time) (int, error)     { return 0, nil }
func (noHolds) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return nil, nil
}

func (noHolds) Active(context.Context) (lifecycle.Holds, error)               { return nil, nil }
func (discardJobs) Enqueue(context.Context, queue.Request) (shared.ID, error) { return hubID, nil }
func (noProof) Available() bool                                               { return true }
func (noProof) Satisfied(context.Context, shared.ID, string) (bool, error)    { return false, nil }
func (discardAudit) Append(context.Context, audit.Entry) error                { return nil }
func (fixedIDs) NewID() shared.ID                                             { return hubID }

func (noProof) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}

func (passThrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passThrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}
