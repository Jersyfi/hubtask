// SPDX-License-Identifier: BUSL-1.1
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
// in a bounded transaction of its own, the way every control-plane act touches a workspace.
type ReadEncryptionStatus struct {
	Tenants    adminrepo.Tenants
	Census     sealingrepo.Census
	Encryptor  cryptoport.Encryptor
	UnitOfWork persistence.UnitOfWork
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

	censuses := make([]map[string]int64, 0, len(tenants))
	for _, tenant := range tenants {
		err := h.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant.ID},
			func(ctx context.Context) error {
				counted, err := h.Census.CountByKey(ctx)
				censuses = append(censuses, counted)
				return err
			})
		if err != nil {
			return Status{}, err
		}
	}
	counts := sealingrepo.Sum(censuses...)

	status := Status{ActiveKeyID: h.Encryptor.ActiveKeyID()}
	inRing := map[string]bool{}
	for _, keyID := range h.Encryptor.KeyIDs() {
		inRing[keyID] = true
		status.Keys = append(status.Keys, KeyUsage{
			KeyID: keyID, Active: keyID == status.ActiveKeyID, InRing: true,
			SealedValues: counts[keyID],
		})
	}
	orphaned := make([]string, 0)
	for keyID := range counts {
		if !inRing[keyID] {
			orphaned = append(orphaned, keyID)
		}
	}
	sort.Strings(orphaned)
	for _, keyID := range orphaned {
		status.Keys = append(status.Keys, KeyUsage{KeyID: keyID, SealedValues: counts[keyID]})
	}
	return status, nil
}

// Descriptor is the catalogue entry.
func (h ReadEncryptionStatus) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadEncryptionStatusName,
		Summary: "Reads the installation's master keyring and, for every key, how many stored " +
			"values still name it across all workspaces. Zero on a key that is not the active " +
			"one is what says the key may be removed from the ring; a key with values that the " +
			"ring no longer holds is the state to repair by putting the key back.",
		TokenScope: adminTenantsScope,
		ReadOnly:   true,
		Audit: usecase.AuditDeclaration{
			Action: "encryption.status_read", TargetType: tenantTarget,
			Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "a read of the control plane touches no item; the history is an item's " +
				"(domain-model.md §3.5).",
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
		keys = append(keys, usecase.Output{
			"key_id": key.KeyID, "active": key.Active, "in_ring": key.InRing,
			"sealed_values": key.SealedValues,
		})
	}
	return usecase.Output{"active_key_id": status.ActiveKeyID, "keys": keys}, nil
}
