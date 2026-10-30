// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package filelink is the use cases of an entry's files in a store (UC-INT-14): a Nextcloud,
// SharePoint, OneDrive or WebDAV file or folder linked by its address under WRITE_ITEMS on the
// entry, read by whoever reads the entry, and taken off again.
//
// Nothing here calls the store: the address is kept, never fetched, so the file keeps its own
// permissions and a reader without access to it in its store opens nothing there.
package filelink

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/filelink"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/filelink"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The use case names.
const (
	ListEntryFileLinksName = "ListEntryFileLinks"
	LinkEntryFileName      = "LinkEntryFile"
	UnlinkEntryFileName    = "UnlinkEntryFile"
)

// The stable audit codes (audit.md §2). An address and a file's name never reach the trail
// (rule 10): the store does.
const (
	LinksReadAction audit.Action = "filelink.links_read"
	LinkedAction    audit.Action = "filelink.linked"
	UnlinkedAction  audit.Action = "filelink.unlinked"
	itemTarget                   = "work_item"
	linkTarget                   = "file_link"
	readScope                    = "items:read"
	writeScope                   = "items:write"
	exemptReason                 = "a file link points at a file kept in its own store; the audit trail records who added or took it off."
)

// Entity is the change log's entity of a file link, filed under its entry's collection so that a
// device holding the entry holds its files' addresses (offline-sync.md §13).
const Entity = "file_link"

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Entries reads the summaries of live entries: their hub and collection.
type Entries interface {
	Entries(ctx context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error)
}

// Files is what every file link use case shares.
type Files struct {
	Store      repository.Store
	Entries    Entries
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// Changes and HLC tell offline devices of a link added or taken off. A nil Changes records
	// nothing, a test's convenience.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

func entryPath(s workrepo.EntrySummary) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !s.HubID.IsZero() {
		path = append(path, identity.HubScope(s.HubID))
	}
	return append(path, identity.CollectionScope(s.CollectionID), identity.ItemScope(s.ID))
}

// entry answers a live entry's summary and asks whether the actor may read or write it.
func (f Files) entry(ctx context.Context, actor appshared.ActorContext, id shared.ID, write bool, action audit.Action) (workrepo.EntrySummary, error) {
	var summaries []workrepo.EntrySummary
	if err := f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		summaries, err = f.Entries.Entries(ctx, []shared.ID{id})
		return err
	}); err != nil {
		return workrepo.EntrySummary{}, err
	}
	if len(summaries) != 1 {
		return workrepo.EntrySummary{}, shared.ErrNotFound.WithDetail("work_item.not_found")
	}
	permission, scope := service.PermissionRead, readScope
	if write {
		permission, scope = service.PermissionWriteItems, writeScope
	}
	return summaries[0], f.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: entryPath(summaries[0]), Action: action, TokenScope: scope,
		TargetType: itemTarget, TargetID: id,
	})
}

func (f Files) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, id shared.ID, changes ...audit.Change) error {
	return f.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: f.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: linkTarget, TargetID: id, Changes: audit.Changes(changes...),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// Output is a file link as the entry's readers see it, and as the change log carries it.
func Output(l domain.Link, collectionID shared.ID) usecase.Output {
	return usecase.Output{
		"id": l.ID.String(), "item_id": l.ItemID.String(), "collection_id": collectionID.String(),
		"provider": string(l.Provider), "address": l.Address, "name": l.Name, "added_by": l.AddedBy.String(),
		"added_at": l.AddedAt.UTC().Format(time.RFC3339),
	}
}

func itemInput() usecase.Field {
	return usecase.Field{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."}
}

func providerNames() []string {
	out := make([]string, 0, len(domain.Providers()))
	for _, p := range domain.Providers() {
		out = append(out, string(p))
	}
	return out
}

// ListEntryFileLinks answers an entry's files in a store.
type ListEntryFileLinks struct{ Files Files }

// Descriptor is the catalogue entry.
func (h ListEntryFileLinks) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListEntryFileLinksName, Permission: service.PermissionRead,
		Summary: "Lists the files and folders an entry links in a file store - Nextcloud, SharePoint, OneDrive or " +
			"WebDAV - in the order they were added, each with its store, address and name. The files stay in " +
			"their store with its permissions.",
		SideEffects: "None. Reads only.", TokenScope: readScope, ReadOnly: true,
		Input:    []usecase.Field{itemInput()},
		Audit:    usecase.AuditDeclaration{Action: LinksReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			item, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			f := h.Files
			summary, err := f.entry(ctx, actor, item, false, LinksReadAction)
			if err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				links, err := f.Store.List(ctx, item)
				for _, l := range links {
					rows = append(rows, Output(l, summary.CollectionID))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// LinkEntryFile links a file or folder in a store on an entry.
type LinkEntryFile struct{ Files Files }

// Descriptor is the catalogue entry.
func (h LinkEntryFile) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: LinkEntryFileName, Permission: service.PermissionWriteItems,
		Summary: "Links a file or folder in a file store on an entry by its https address: a Nextcloud share or file, " +
			"a SharePoint or OneDrive document, any WebDAV address. The store is told by the address unless it is " +
			"given, the name taken from it unless one is given. The file is never copied.",
		SideEffects: "Adds the link to the entry; the file in its store is untouched.",
		TokenScope:  writeScope,
		Input: []usecase.Field{
			itemInput(),
			{Name: "address", Kind: usecase.KindString, Required: true, Description: "The file's or folder's https address, at most 2048 bytes; no credentials in it."},
			{Name: "provider", Kind: usecase.KindString, Enum: providerNames(), Description: "The store; told by the address when not given."},
			{Name: "name", Kind: usecase.KindString, Description: "What the link is called, at most 200 characters; the address's last segment when not given."},
		},
		Audit:    usecase.AuditDeclaration{Action: LinkedAction, TargetType: linkTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			item, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			f := h.Files
			address, provider, name, err := domain.Check(in.String("address"), domain.Provider(in.String("provider")), in.String("name"))
			if err != nil {
				return nil, err
			}
			summary, err := f.entry(ctx, actor, item, true, LinkedAction)
			if err != nil {
				return nil, err
			}
			link := domain.Link{
				ID: f.IDs.NewID(), TenantID: actor.TenantID, ItemID: item, Provider: provider, Address: address,
				Name: name, AddedBy: actor.AccountID, AddedAt: f.Clock.Now(),
			}
			err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				n, err := f.Store.Count(ctx, item)
				if err != nil {
					return err
				}
				if n >= domain.MaxLinksPerEntry {
					return shared.ErrConflict.WithDetail("filelink.too_many")
				}
				added, err := f.Store.Add(ctx, link)
				if err != nil {
					return err
				}
				if !added {
					return shared.ErrConflict.WithDetail("filelink.already_linked")
				}
				if f.Changes != nil {
					if err := f.Changes.Record(ctx, changelog.Change{
						TenantID: actor.TenantID, Entity: Entity, EntityID: link.ID, Op: changelog.Upsert,
						ContainerID: summary.CollectionID, ActorID: actor.AccountID, HLC: f.HLC.Next(),
						Payload: Output(link, summary.CollectionID),
					}); err != nil {
						return err
					}
				}
				return f.record(ctx, actor, LinkedAction, link.ID,
					audit.Change{Field: "item_id", Classification: audit.Open, To: item.String()},
					audit.Change{Field: "provider", Classification: audit.Open, To: string(provider)})
			})
			if err != nil {
				return nil, err
			}
			return Output(link, summary.CollectionID), nil
		}),
	}
}

// UnlinkEntryFile takes a file link off its entry.
type UnlinkEntryFile struct{ Files Files }

// Descriptor is the catalogue entry.
func (h UnlinkEntryFile) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UnlinkEntryFileName, Permission: service.PermissionWriteItems,
		Summary:     "Takes a file link off its entry. The file itself stays in its store, untouched.",
		SideEffects: "Removes the link from the entry.",
		TokenScope:  writeScope,
		Input: []usecase.Field{
			{Name: "link_id", Kind: usecase.KindID, Required: true, Description: "The file link."},
		},
		Audit:    usecase.AuditDeclaration{Action: UnlinkedAction, TargetType: linkTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("link_id")
			if err != nil {
				return nil, err
			}
			f := h.Files
			var link domain.Link
			if err := f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				link, err = f.Store.Find(ctx, id)
				return err
			}); err != nil {
				return nil, err
			}
			summary, err := f.entry(ctx, actor, link.ItemID, true, UnlinkedAction)
			if err != nil {
				return nil, err
			}
			err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := f.Store.Remove(ctx, id); err != nil {
					return err
				}
				if f.Changes != nil {
					if err := f.Changes.Record(ctx, changelog.Change{
						TenantID: actor.TenantID, Entity: Entity, EntityID: id, Op: changelog.Delete,
						ContainerID: summary.CollectionID, ActorID: actor.AccountID, HLC: f.HLC.Next(),
					}); err != nil {
						return err
					}
				}
				return f.record(ctx, actor, UnlinkedAction, id,
					audit.Change{Field: "item_id", Classification: audit.Open, From: link.ItemID.String()},
					audit.Change{Field: "provider", Classification: audit.Open, From: string(link.Provider)})
			})
			return nil, err
		}),
	}
}
