// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListStarsName    = "ListStars"
	ReplaceStarsName = "ReplaceStars"

	// StarsReadAction and StarsChangedAction are the audit codes. The entry of a change carries the
	// count and never which places: a star names objects (rule 10, UC-WRK-37/4).
	StarsReadAction    audit.Action = "account.stars_read"
	StarsChangedAction audit.Action = "account.stars_changed"
)

// StarReader is the slice of the catalogue the stars read through: each star is asked of the use
// case that reads its kind, as the caller, so a star answers only what the caller may read now
// (P-05) and the decision stays where every other read makes it.
type StarReader interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// starReads names, per kind, the use case that reads it and the input it takes the id as.
var starReads = map[domain.StarKind][2]string{
	domain.StarHub:        {"GetContainer", "container_id"},
	domain.StarCollection: {"GetContainer", "container_id"},
	domain.StarView:       {"GetSavedView", "view_id"},
	domain.StarPage:       {"ReadPage", "page_id"},
	domain.StarWhiteboard: {"ReadWhiteboard", "whiteboard_id"},
	domain.StarDashboard:  {"ReadDashboard", "dashboard_id"},
	domain.StarPortfolio:  {"ReadPortfolio", "portfolio_id"},
}

// Stars is what both star use cases share.
type Stars struct {
	Stars      repository.Stars
	Accounts   repository.Accounts
	Reader     StarReader
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// readable keeps the stars the caller may read now, in their order. A refusal or a missing object
// drops the star; any other failure is the answer's.
func (s Stars) readable(ctx context.Context, actor appshared.ActorContext, stars []domain.Star) ([]domain.Star, error) {
	kept := make([]domain.Star, 0, len(stars))
	for _, star := range stars {
		read, known := starReads[star.Kind]
		if !known || s.Reader == nil {
			continue
		}
		out, err := s.Reader.Invoke(ctx, read[0], actor, usecase.Input{read[1]: star.ID.String()})
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrForbidden) ||
				errors.Is(err, shared.ErrGone) || errors.Is(err, shared.ErrValidation) {
				continue
			}
			return nil, err
		}
		// A hub and a collection are both containers; a star of the one kind is not the other.
		if star.Kind == domain.StarHub || star.Kind == domain.StarCollection {
			if out.String("type") != string(star.Kind) {
				continue
			}
		}
		kept = append(kept, star)
	}
	return kept, nil
}

func (s Stars) subject(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, action audit.Action, scope string) (shared.ID, error) {
	return preferenceSubject(ctx, actor, accountID, s.Authorizer, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: scope,
		TargetType: accountTarget,
	})
}

func (s Stars) stored(ctx context.Context, actor appshared.ActorContext, target shared.ID) (domain.StarList, error) {
	var list domain.StarList
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := findAccount(ctx, s.Accounts, target); err != nil {
			return err
		}
		var err error
		list, err = s.Stars.Find(ctx, target)
		return err
	})
	return list, err
}

// ListStars answers a person's stars in their order, only those the caller may read now
// (UC-WRK-37 checks 1 and 3).
type ListStars struct{ Stars Stars }

// Execute reads the list.
func (h ListStars) Execute(ctx context.Context, actor appshared.ActorContext, accountID shared.ID) (domain.StarList, error) {
	target, err := h.Stars.subject(ctx, actor, accountID, StarsReadAction, accountsRead)
	if err != nil {
		return domain.StarList{}, err
	}
	list, err := h.Stars.stored(ctx, actor, target)
	if err != nil {
		return domain.StarList{}, err
	}
	list.Stars, err = h.Stars.readable(ctx, actor, list.Stars)
	return list, err
}

// Descriptor is the catalogue entry.
func (h ListStars) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListStarsName,
		Permission: service.PermissionManageMembers,
		Summary: "Reads a person's stars - the hubs, collections, views, pages, whiteboards, dashboards " +
			"and portfolios they keep close - in their order, only those the caller may read now. " +
			"The caller's own need no permission; anybody else's need the member management permission.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "Whose stars. Omitted means the caller's own."},
		},
		Audit: usecase.AuditDeclaration{
			Action: StarsReadAction, TargetType: accountTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			accountID, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			list, err := h.Execute(ctx, actor, accountID)
			if err != nil {
				return nil, err
			}
			return starsOutput(list), nil
		}),
	}
}

// ReplaceStarsCommand is the input, typed.
type ReplaceStarsCommand struct {
	AccountID shared.ID
	Stars     []domain.Star
	// ExpectedVersion is the version last read; zero accepts whatever is there.
	ExpectedVersion int
}

// ReplaceStars writes a person's whole ordered list of stars (UC-WRK-37 checks 2-4). A star of
// something the caller may not read is dropped, which is how a lost one leaves at the next write.
type ReplaceStars struct{ Stars Stars }

// Execute writes the list and answers it as stored.
func (h ReplaceStars) Execute(ctx context.Context, actor appshared.ActorContext, cmd ReplaceStarsCommand) (domain.StarList, error) {
	target, err := h.Stars.subject(ctx, actor, cmd.AccountID, StarsChangedAction, accountsWrite)
	if err != nil {
		return domain.StarList{}, err
	}
	stars, err := domain.CheckStars(cmd.Stars)
	if err != nil {
		return domain.StarList{}, err
	}
	if stars, err = h.Stars.readable(ctx, actor, stars); err != nil {
		return domain.StarList{}, err
	}
	var written domain.StarList
	err = h.Stars.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := findAccount(ctx, h.Stars.Accounts, target); err != nil {
			return err
		}
		current, err := h.Stars.Stars.Find(ctx, target)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("stars.version_conflict")
		}
		now := h.Stars.Clock.Now()
		if err := h.Stars.Stars.Replace(ctx, target, stars, current.Version, now); err != nil {
			return err
		}
		written = domain.StarList{Stars: stars, Version: current.Version + 1}
		return h.Stars.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     StarsChangedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: accountTarget,
			TargetID:   target,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "stars", Classification: audit.Open,
				From: strconv.Itoa(len(current.Stars)), To: strconv.Itoa(len(stars)),
			}),
		})
	})
	if err != nil {
		return domain.StarList{}, err
	}
	return written, nil
}

// Descriptor is the catalogue entry.
func (h ReplaceStars) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReplaceStarsName,
		Permission: service.PermissionManageMembers,
		Summary: "Replaces a person's whole ordered list of stars, at most 100 of the kinds HUB, " +
			"COLLECTION, VIEW, PAGE, WHITEBOARD, DASHBOARD and PORTFOLIO. A star of something the " +
			"caller may not read is dropped; the same place twice is kept once.",
		SideEffects: "Writes the list and an audit entry with its count.",
		TokenScope:  accountsWrite,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Description: "Whose stars. Omitted means the caller's own."},
			{Name: "stars", Kind: usecase.KindList, Required: true, Description: "The stars in their order, each {kind, id}."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{
			Action: StarsChangedAction, TargetType: accountTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			accountID, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			stars, err := starsOf(in["stars"])
			if err != nil {
				return nil, err
			}
			list, err := h.Execute(ctx, actor, ReplaceStarsCommand{
				AccountID: accountID, Stars: stars, ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return starsOutput(list), nil
		}),
	}
}

// starsOf reads the list as a door hands it: objects with a kind and an id.
func starsOf(raw any) ([]domain.Star, error) {
	members, _ := raw.([]any)
	stars := make([]domain.Star, 0, len(members))
	for i, member := range members {
		object, ok := member.(map[string]any)
		kind, _ := object["kind"].(string)
		id, _ := object["id"].(string)
		if !ok || kind == "" || id == "" {
			return nil, shared.ErrValidation.WithDetail("stars.member_invalid").
				WithFields(shared.FieldError{Path: "/stars/" + strconv.Itoa(i), Code: "stars.member_invalid"})
		}
		parsed, err := shared.ParseID(id)
		if err != nil {
			return nil, shared.ErrValidation.WithDetail("stars.member_invalid").
				WithFields(shared.FieldError{Path: "/stars/" + strconv.Itoa(i) + "/id", Code: "stars.member_invalid"})
		}
		stars = append(stars, domain.Star{Kind: domain.StarKind(kind), ID: parsed})
	}
	return stars, nil
}

func starsOutput(list domain.StarList) usecase.Output {
	rows := make([]usecase.Output, 0, len(list.Stars))
	for _, star := range list.Stars {
		rows = append(rows, usecase.Output{"kind": string(star.Kind), "id": star.ID.String()})
	}
	return usecase.Output{"stars": rows, "version": list.Version}
}
