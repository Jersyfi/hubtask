// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"net/url"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The device's list of workspaces (ADR-0095 §7, UC-ID-25, multi-tenancy.md §3.1): after a sign-in
// a workspace hands the browser an entry - its main address and display name - signed by the
// installation, and the installation's own address keeps it once it confirmed its own signature
// and that the host is still a main address. Nothing else is signed into it, so the list holds no
// person and no token, and the check answers nothing for an entry the installation did not sign.

const (
	IssueDeviceListEntryName = "IssueDeviceListEntry"
	CheckDeviceListName      = "CheckDeviceList"

	DeviceListEntryReadAction audit.Action = "workspace.device_list_entry_read"

	// DeviceListCheckMax bounds one check: a list longer than this is no person's list.
	DeviceListCheckMax = 50
)

// DeviceListSigner signs an entry with the installation's key and reads one back. An interface
// here and a keyed HMAC in infrastructure/security, because the installation secret has no
// business above the adapters (security.md §8). Read refuses every entry it did not sign alike.
type DeviceListSigner interface {
	Sign(entry domain.DeviceListEntry) string
	Read(signed string) (domain.DeviceListEntry, error)
}

// IssueDeviceListEntry answers the caller's workspace's entry, in multi mode only.
type IssueDeviceListEntry struct {
	Signer DeviceListSigner
	// Address is the workspace's main address, every link's (WorkspaceAddress).
	Address    WorkspaceAddress
	Workspaces WorkspaceSlug
	UnitOfWork persistence.UnitOfWork
}

// SignedDeviceListEntry is an entry and what it says.
type SignedDeviceListEntry struct {
	Signed string
	domain.DeviceListEntry
}

// Execute signs the entry. A person only: a service account has no device to remember it on.
func (h IssueDeviceListEntry) Execute(ctx context.Context, actor appshared.ActorContext) (SignedDeviceListEntry, error) {
	if !actor.IsAuthenticated() {
		return SignedDeviceListEntry{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(accountsRead); err != nil {
		return SignedDeviceListEntry{}, err
	}
	if actor.Kind != appshared.ActorUser || actor.AccountID.IsZero() {
		return SignedDeviceListEntry{}, shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	if !h.Address.Multi {
		return SignedDeviceListEntry{}, shared.ErrNotFound.WithDetail("access.device_list_not_offered")
	}
	var entry domain.DeviceListEntry
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		base, err := h.Address.Of(ctx)
		if err != nil {
			return err
		}
		workspace, err := h.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		parsed, err := url.Parse(base)
		if err != nil {
			return shared.Internalf("the workspace's address %q: %w", base, err)
		}
		made, ok := domain.NewDeviceListEntry(parsed.Hostname(), workspace.DisplayName)
		if !ok {
			return shared.ErrNotFound.WithDetail("access.device_list_not_offered")
		}
		entry = made
		return nil
	})
	if err != nil {
		return SignedDeviceListEntry{}, err
	}
	return SignedDeviceListEntry{Signed: h.Signer.Sign(entry), DeviceListEntry: entry}, nil
}

// CheckDeviceList answers, for the entries the installation signed, whether each host is still a
// workspace's main address. Anonymous: it is asked by the installation's own address, where nobody
// is signed in.
type CheckDeviceList struct {
	Signer     DeviceListSigner
	Hosts      repository.MainAddresses
	UnitOfWork persistence.UnitOfWork
	// Multi is whether the installation runs several workspaces; single mode keeps no list.
	Multi bool
}

// CheckedDeviceListEntry is one entry the installation signed, and whether it still answers.
type CheckedDeviceListEntry struct {
	SignedDeviceListEntry
	Answers bool
}

// Execute checks them, in the order asked. An entry it did not sign is left out, not refused: the
// answer then says nothing about a host nobody was handed.
func (h CheckDeviceList) Execute(ctx context.Context, entries []string) ([]CheckedDeviceListEntry, error) {
	if len(entries) > DeviceListCheckMax {
		return nil, shared.Validation("/entries", "auth.device_list_too_many", map[string]string{"maximum": "50"})
	}
	checked := []CheckedDeviceListEntry{}
	if !h.Multi {
		return checked, nil
	}
	for _, signed := range entries {
		entry, err := h.Signer.Read(signed)
		if err != nil {
			continue
		}
		checked = append(checked, CheckedDeviceListEntry{
			SignedDeviceListEntry: SignedDeviceListEntry{Signed: signed, DeviceListEntry: entry},
		})
	}
	if len(checked) == 0 {
		return checked, nil
	}
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		for index := range checked {
			answers, err := h.Hosts.IsMainHost(ctx, checked[index].Host)
			if err != nil {
				return err
			}
			checked[index].Answers = answers
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return checked, nil
}

// Descriptor is the catalogue entry.
func (h IssueDeviceListEntry) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: IssueDeviceListEntryName,
		Summary: "Answers the caller's workspace's entry for the device's list of workspaces: its " +
			"main address and display name, signed by the installation. Multi mode and people only.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: DeviceListEntryReadAction, TargetType: workspaceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			entry, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			return deviceListEntryOutput(entry), nil
		}),
	}
}

// Descriptor registers the check. Anonymous, as the installation's own address is.
func (h CheckDeviceList) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CheckDeviceListName,
		Summary: "Answers, for the entries of a device's list of workspaces this installation " +
			"signed, whether each host is still a workspace's main address. An entry it did not " +
			"sign is left out of the answer.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "entries", Kind: usecase.KindList, Required: true,
				Description: "The signed entries, as the device holds them; at most 50."},
		},
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, _ appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			entries, err := in.StringList("entries")
			if err != nil {
				return nil, err
			}
			checked, err := h.Execute(ctx, entries)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(checked))
			for _, each := range checked {
				out := deviceListEntryOutput(each.SignedDeviceListEntry)
				out["answers"] = each.Answers
				data = append(data, out)
			}
			return usecase.Output{"entries": data}, nil
		}),
	}
}

func deviceListEntryOutput(entry SignedDeviceListEntry) usecase.Output {
	return usecase.Output{"entry": entry.Signed, "host": entry.Host, "display_name": entry.Name}
}
