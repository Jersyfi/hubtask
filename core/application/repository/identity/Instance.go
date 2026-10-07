// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The level above the workspaces (ADR-0070 §2).
//
// Typed rather than a bag of keys and JSON. The set of switches is closed - the domain names every
// one of them - so the layer can cross this seam as the value it is, and the dotted keys stay where
// they belong: in the adapter, which is the only thing that has to know how a row spells them.

// InstanceLevel is what the installation decided, in the shape the resolver takes.
type InstanceLevel struct {
	// Policy is the instance's sign-in layer: what it set, and what it locked.
	Policy identity.PolicyLayer
	// Legal is the instance's four links and their locks.
	Legal identity.LegalLayer
	// Localisation is the default language, time zone and week start a workspace inherits when it
	// has set none of its own.
	//
	// **A default, never a lock** (ADR-0070 decision 6): a company that may not work in German
	// because the operator set it so is a product defect. So there is no lock map beside it, which
	// is the structural way of saying it rather than a check somebody can forget.
	Localisation identity.LocalisationDefaults
	// Quotas are the ceilings the installation sets for every workspace that sets none of its own:
	// the middle level of `Effective(product, instance, plan, workspace)` (ADR-0070 decision 7). A
	// lock here is meaningful — an operator may forbid a workspace raising its own — so the locks
	// travel.
	Quotas identity.QuotaDefaults
	// BlocklistFile is the path to the operator's own list of refused passwords, read offline.
	// Instance-only and deliberately not a workspace switch: the file is on the operator's disk,
	// so there is nothing for a workspace to point at (ADR-0068 §1, and the reason the thirteenth
	// switch of the table is not on the settings screen).
	BlocklistFile string
	// Source says where the values came from, for the health report: "database" or the path of
	// the file that is enforcing them (ADR-0070 §5).
	Source string
	// IsEnforcedFromFile is the `enforce` mode: the writing routes refuse, because the file is
	// the source and a write through the API would be overwritten at the next start.
	IsEnforcedFromFile bool
}

// InstanceSettings reads and writes the installation's own level.
//
// No method takes a tenant, and not for row level security's reason this time: `instance_setting`
// deliberately carries no policy, and the transaction it runs in has no tenant at all. What bounds
// the write is the use case - the control plane's, behind `admin:tenants` and the operator register.
type InstanceSettings interface {
	// Read answers the whole level. Everything unset is absent from the layer rather than zero,
	// which is what lets the resolver tell "the operator chose nothing" from "the operator chose
	// zero".
	Read(ctx context.Context) (InstanceLevel, error)

	// Write replaces the level whole: every switch the caller decided is stored, every switch it
	// did not is removed. A PUT, because a merge over eighteen switches that can each be absent
	// has no way to say "unset this one".
	Write(ctx context.Context, level InstanceLevel, by shared.ID, at time.Time) error
}

// Operator is one row of the register (ADR-0070 §1): an account of some workspace that operates
// this installation, and who put it there.
type Operator struct {
	TenantID  shared.ID
	AccountID shared.ID
	AddedAt   time.Time
	AddedBy   shared.ID
}

// Operators is the register. Every method goes through a narrow function rather than at the table:
// `operator` carries no row-level policy *and* no grant to the application role, because its rows
// name accounts across workspaces - a policy-free table the application role could read would let
// every workspace enumerate the installation's operators.
type Operators interface {
	// Holds reports whether this account operates the installation. **An empty register answers
	// true**, which is the private installation: nothing was configured, and the owner is the
	// operator exactly as they were before the register existed.
	//
	// The account alone, here and in Add: the workspace is on the row and the function reads it
	// from the account, so no caller holds a pair that could disagree - and no method here takes a
	// tenant, which rule 3 does not permit and this register does not need.
	Holds(ctx context.Context, accountID shared.ID) (bool, error)

	// List answers the whole register, for the control plane's own screen.
	List(ctx context.Context) ([]Operator, error)

	// Resolve answers the account an address names inside one workspace, and zero where the pair
	// matches nothing.
	//
	// It exists because an account id is not something an operator can look up: `account` is behind
	// row level security, so the control plane cannot list accounts across workspaces and the
	// screen would be asking somebody to type a UUID out of the database by hand (migration 0109).
	// An address and a workspace are what a person actually knows.
	//
	// **One identifier and nothing else.** Not a list, not a search, not a name — and no more than
	// a caller already learns by trying `Add`, so nothing here widens what the control plane sees.
	Resolve(ctx context.Context, slug, email string) (shared.ID, error)

	// Add puts an account in. False means it was already there **or** that no such account exists:
	// the first is not an error - a caller asking for somebody to be an operator got what they
	// asked for - and the second is, which the caller tells apart by reading the register.
	Add(ctx context.Context, accountID, by shared.ID) (bool, error)

	// Remove takes one out, by account alone: an identifier is unique across the installation, and
	// a caller that had to name the workspace too would have to read the register to find out which
	// one it is. False means it was not there, **or** that it is the last one - the register may not
	// be emptied, and which of the two applies is answered by reading the list.
	Remove(ctx context.Context, accountID shared.ID) (bool, error)
}
