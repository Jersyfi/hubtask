// SPDX-License-Identifier: BUSL-1.1
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
