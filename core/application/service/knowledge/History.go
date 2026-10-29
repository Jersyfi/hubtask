// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	ListPageVersionsName   = "ListPageVersions"
	ReadPageVersionName    = "ReadPageVersion"
	NamePageVersionName    = "NamePageVersion"
	RestorePageVersionName = "RestorePageVersion"

	maxVersionsListed = 100
)

// VersionOutput is a version as a history lists it (schema PageVersion); the body only when read
// on its own.
func VersionOutput(row repository.VersionRow) usecase.Output {
	return usecase.Output{
		"seq": row.Seq, "title": row.Title, "author_id": idOrNil(row.AuthorID), "name": nilIfEmpty(row.Name),
		"restored_from": restoredOrNil(row.Restored), "size": row.Size, "created_at": row.CreatedAt,
	}
}

func restoredOrNil(seq int) any {
	if seq == 0 {
		return nil
	}
	return seq
}

// ListPageVersions reads a page's history, newest first (UC-DOC-06).
type ListPageVersions struct{ Space Space }

// Execute reads the history.
func (h ListPageVersions) Execute(
	ctx context.Context, actor appshared.ActorContext, pageID shared.ID, before, limit int,
) ([]repository.VersionRow, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxVersionsListed {
		limit = maxVersionsListed
	}
	var rows []repository.VersionRow
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = h.Space.Pages.Versions(ctx, page.ID, before, limit)
		return err
	})
	return rows, err
}

// Descriptor is the catalogue entry.
func (h ListPageVersions) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListPageVersionsName, Permission: service.PermissionRead,
		Summary: "Reads a page's history newest first: one version per editing session, each with its " +
			"author, moment, size and name, and the version it brought back where it was a restore. " +
			"Without the bodies; continue with `before`.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "before", Kind: usecase.KindInt, Description: "Answer the versions before this one."},
			{Name: "limit", Kind: usecase.KindInt, Description: "At most this many, up to 100."},
		},
		Audit: usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			rows, err := h.Execute(ctx, actor, id, in.Int("before"), in.Int("limit"))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(rows))
			for _, row := range rows {
				data = append(data, VersionOutput(row))
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// ReadPageVersion reads one version with its body, so that two can be compared.
type ReadPageVersion struct{ Space Space }

// Execute reads the version.
func (h ReadPageVersion) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, seq int) (domain.Version, error) {
	page, err := h.Space.readable(ctx, actor, pageID, PageReadAction)
	if err != nil {
		return domain.Version{}, err
	}
	var version domain.Version
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		version, err = h.Space.Pages.Version(ctx, page.ID, seq)
		return err
	})
	return version, err
}

// Descriptor is the catalogue entry.
func (h ReadPageVersion) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPageVersionName, Permission: service.PermissionRead,
		Summary:     "Reads one version of a page with its title and body as they stood, to compare it with another or with the page.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "seq", Kind: usecase.KindInt, Required: true, Description: "The version."},
		},
		Audit: usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			version, err := h.Execute(ctx, actor, id, in.Int("seq"))
			if err != nil {
				return nil, err
			}
			out := VersionOutput(repository.VersionRow{
				Seq: version.Seq, Title: version.Title, AuthorID: version.AuthorID, Name: version.Name,
				Restored: version.Restored, Size: len(version.Body), CreatedAt: version.CreatedAt,
			})
			out["body"] = version.Body
			return out, nil
		}),
	})
}

// NamePageVersion names a version ("Sent to legal"), or takes its name away. A named version is
// never folded into by a later write of the same session.
type NamePageVersion struct{ Space Space }

// Execute names the version.
func (h NamePageVersion) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID, seq int, raw string) error {
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageVersionNamedAction, pageTarget); err != nil {
		return err
	}
	name, err := domain.CheckVersionName(raw, h.Space.Text)
	if err != nil {
		return err
	}
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Space.Pages.NameVersion(ctx, place.ID, seq, name); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageVersionNamedAction, pageTarget, place.ID, []audit.Change{
			{Field: "seq", Classification: audit.Open, To: seq},
			{Field: "name", Classification: audit.Sensitive, To: name},
		}, h.Space.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h NamePageVersion) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: NamePageVersionName, Permission: service.PermissionWriteItems,
		Summary:     "Names a version of a page, or takes its name away with an empty name. A named version stays as it is: the next write starts a new one.",
		SideEffects: "Writes the version's name and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "seq", Kind: usecase.KindInt, Required: true, Description: "The version."},
			{Name: "name", Kind: usecase.KindString, Description: "The name, one line; empty takes it away."},
		},
		Audit: usecase.AuditDeclaration{Action: PageVersionNamedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			seq := in.Int("seq")
			if err := h.Execute(ctx, actor, id, seq, in.String("name")); err != nil {
				return nil, err
			}
			return usecase.Output{"seq": seq, "name": nilIfEmpty(in.String("name"))}, nil
		}),
	})
}

// RestorePageVersion brings an older version back as the page's newest: nothing in between is
// lost, the restore is a version of its own (P-03, UC-DOC-06).
type RestorePageVersion struct{ Space Space }

// Execute restores the version.
func (h RestorePageVersion) Execute(
	ctx context.Context, actor appshared.ActorContext, pageID shared.ID, seq, expected int,
) (domain.Page, error) {
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageVersionBackAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, pageID, true)
		if err != nil {
			return err
		}
		if expected != 0 && expected != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		version, err := h.Space.Pages.Version(ctx, current.ID, seq)
		if err != nil {
			return err
		}
		last, found, err := h.Space.Pages.LastVersion(ctx, current.ID)
		if err != nil {
			return err
		}
		now := h.Space.Clock.Now()
		next := current
		next.Title, next.Body, next.UpdatedBy, next.UpdatedAt = version.Title, version.Body, actor.AccountID, now
		if err := h.Space.Pages.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		restored := domain.Version{
			PageID: current.ID, Seq: 1, Title: version.Title, Body: version.Body, AuthorID: actor.AccountID,
			Restored: seq, CreatedAt: now,
		}
		if found {
			restored.Seq = last.Seq + 1
		}
		if err := h.Space.Pages.InsertVersion(ctx, restored); err != nil {
			return err
		}
		if err := h.Space.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageVersionBackAction, pageTarget, current.ID, []audit.Change{
			{Field: "seq", Classification: audit.Open, To: seq},
		}, now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h RestorePageVersion) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RestorePageVersionName, Permission: service.PermissionWriteItems,
		Summary: "Brings an older version of a page back: its title and body become the page's, written " +
			"as a new version that names the one it restored, so nothing in between is lost.",
		SideEffects: "Writes the page, a new version and an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "seq", Kind: usecase.KindInt, Required: true, Description: "The version to bring back."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The page's version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: PageVersionBackAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, id, in.Int("seq"), in.Int("expected_version"))
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}
