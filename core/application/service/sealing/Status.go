// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing

import (
	"context"
	"sort"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	sealingrepo "github.com/Jersyfi/hubtask/core/application/repository/sealing"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/port/audit"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ReadEncryptionStatusName = "ReadEncryptionStatus"
	ResealSecretsName        = "ResealSecrets"
)

// KeyUsage is one key as the operator sees it: whether the ring holds it, whether new values
// seal under it, and how many stored values still name it.
type KeyUsage struct {
	KeyID        string
	Active       bool
	InRing       bool
	SealedValues int64
	// Stores is the same count by the kind of value, so a key the ring lacks says what would stay
	// unreadable. Never by workspace.
	Stores []sealingrepo.StoreCount
}

// Status is the keyring and the census over it.
type Status struct {
	ActiveKeyID string
	// Keys is the ring's order first - the active key, then its predecessors - and after them
	// any key a stored value names that the ring no longer holds.
	Keys []KeyUsage
}

// ReadEncryptionStatus answers what a rotation needs before it can end (ADR-0045).
//
// The census is per tenant, like every read of a tenant's rows, and this is the control plane
// summing it: the tenants are listed through the one legitimate enumerator and each is counted
// in a bounded transaction of its own, the way every control-plane act touches a workspace; the
// installation's own secrets are counted once more, at their own level.
type ReadEncryptionStatus struct {
	Tenants    adminrepo.Tenants
	Census     sealingrepo.Census
	Encryptor  cryptoport.Encryptor
	UnitOfWork persistence.UnitOfWork
	// Observer is told every census this read takes - the health report's, so it is never older
	// than what the operator just saw. Nil tells nobody.
	Observer CensusObserver
}

// CensusObserver takes a census counted elsewhere.
type CensusObserver interface {
	Observe(counts sealingrepo.Counts)
}

// Execute reads the ring and counts across every workspace.
func (h ReadEncryptionStatus) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (Status, error) {
	if err := actor.RequireScope(adminTenantsScope); err != nil {
		return Status{}, err
	}

	var tenants []adminrepo.TenantRecord
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			listed, err := h.Tenants.List(ctx)
			tenants = listed
			return err
		})
	if err != nil {
		return Status{}, err
	}

	counts, err := Count(ctx, h.UnitOfWork, h.Census, tenants)
	if err != nil {
		return Status{}, err
	}
	if h.Observer != nil {
		h.Observer.Observe(counts)
	}

	status := Status{ActiveKeyID: h.Encryptor.ActiveKeyID()}
	totals := counts.ByKey()
	inRing := map[string]bool{}
	for _, keyID := range h.Encryptor.KeyIDs() {
		inRing[keyID] = true
		status.Keys = append(status.Keys, KeyUsage{
			KeyID: keyID, Active: keyID == status.ActiveKeyID, InRing: true,
			SealedValues: totals[keyID], Stores: counts.Stores(keyID),
		})
	}
	orphaned := make([]string, 0)
	for keyID := range totals {
		if !inRing[keyID] {
			orphaned = append(orphaned, keyID)
		}
	}
	sort.Strings(orphaned)
	for _, keyID := range orphaned {
		status.Keys = append(status.Keys, KeyUsage{
			KeyID: keyID, SealedValues: totals[keyID], Stores: counts.Stores(keyID),
		})
	}
	return status, nil
}

// Count is the installation's census: every workspace in a bounded read of its own, then the
// installation's own rows once, in its own scope - never inside a workspace's transaction, which
// may not switch to it.
func Count(
	ctx context.Context, uow persistence.UnitOfWork, census sealingrepo.Census,
	tenants []adminrepo.TenantRecord,
) (sealingrepo.Counts, error) {
	levels := make([]sealingrepo.Counts, 0, len(tenants)+1)
	for _, tenant := range tenants {
		err := uow.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant.ID},
			func(ctx context.Context) error {
				counted, err := census.Count(ctx)
				levels = append(levels, counted)
				return err
			})
		if err != nil {
			return nil, err
		}
	}
	err := uow.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		counted, err := census.CountInstallation(ctx)
		levels = append(levels, counted)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sealingrepo.Sum(levels...), nil
}

// Descriptor is the catalogue entry.
func (h ReadEncryptionStatus) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadEncryptionStatusName,
		Summary: "Reads the installation's master keyring and, for every key, how many stored " +
			"values still name it across all workspaces and the installation's own secrets, and in " +
			"which kind of value. Zero on a key that is not the active " +
			"one is what says the key may be removed from the ring; a key with values that the " +
			"ring no longer holds is the state to repair by putting the key back.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Audit: usecase.AuditDeclaration{
			Action: "encryption.status_read", TargetType: tenantTarget,
			Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read of the control plane touches no item; the history is an item's.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadEncryptionStatus) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	status, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	keys := make([]usecase.Output, 0, len(status.Keys))
	for _, key := range status.Keys {
		stores := make([]usecase.Output, 0, len(key.Stores))
		for _, store := range key.Stores {
			stores = append(stores, usecase.Output{"store": store.Store, "sealed_values": store.Count})
		}
		keys = append(keys, usecase.Output{
			"key_id": key.KeyID, "active": key.Active, "in_ring": key.InRing,
			"sealed_values": key.SealedValues, "stores": stores,
		})
	}
	return usecase.Output{"active_key_id": status.ActiveKeyID, "keys": keys}, nil
}
