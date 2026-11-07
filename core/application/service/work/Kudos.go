// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ReadKudosName     = "ReadKudos"
	GiveKudosName     = "GiveKudos"
	TakeBackKudosName = "TakeBackKudos"

	// The audit codes. All three are written to the trail only when refused, by the authorisation
	// service: a trail of every give would let an auditor count who acknowledged whom (UC-WRK-29
	// check 6), so giving and taking back are a named exception to gate SG-13 (security.md §13).
	KudosReadAction      audit.Action = "kudos.read"
	KudosGivenAction     audit.Action = "kudos.given"
	KudosTakenBackAction audit.Action = "kudos.taken_back"
)

// kudosExempt is why kudos leave no step in an entry's history: the history is read by everyone
// who reads the entry, and who gave kudos is the business of the person they were given to alone.
const kudosExempt = "kudos are seen only by the person who completed the entry, and the history " +
	"is read by everybody who reads it; a step there would tell every reader who acknowledged whom."

// KudosDesk is what the three kudos use cases share (UC-WRK-29 checks 5 and 6).
//
// The right to give kudos is the right to read the entry: kudos change nothing of it - not its
// version, not its history - so a viewer and a guest on a shared entry may acknowledge a colleague
// as a member may. Nothing is announced: no event (a webhook or a rule would carry who acknowledged
// whom out of the product) and no change record.
//
// The trail gets a refusal and nothing else (audit.md §7): a refused give still leaves a trace of
// misuse, a successful one none an auditor could count.
type KudosDesk struct {
	Kudos      repository.Kudos
	Items      repository.Items
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ReadKudos answers what the reader may know of an entry's kudos.
type ReadKudos struct{ Desk KudosDesk }

// GiveKudos acknowledges an entry's completion to whoever completed it.
type GiveKudos struct{ Desk KudosDesk }

// TakeBackKudos takes back the reader's kudos on an entry's current completion.
type TakeBackKudos struct{ Desk KudosDesk }

// kudosAct is which of the three a call is.
type kudosAct int

const (
	kudosReading kudosAct = iota
	kudosGiving
	kudosTakingBack
)

func (a kudosAct) action() audit.Action {
	switch a {
	case kudosGiving:
		return KudosGivenAction
	case kudosTakingBack:
		return KudosTakenBackAction
	default:
		return KudosReadAction
	}
}

// Execute answers the reader's view of the entry's kudos.
func (h ReadKudos) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (domain.KudosView, error) {
	return h.Desk.act(ctx, actor, itemID, kudosReading)
}

// Execute gives the reader's kudos and answers their view.
func (h GiveKudos) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (domain.KudosView, error) {
	return h.Desk.act(ctx, actor, itemID, kudosGiving)
}

// Execute takes the reader's kudos back and answers their view.
func (h TakeBackKudos) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (domain.KudosView, error) {
	return h.Desk.act(ctx, actor, itemID, kudosTakingBack)
}

// act is the whole of the three use cases: the permission question before any transaction (a
// refusal writes an audit entry, audit.md §7), then the act and the reader's view in one.
func (d KudosDesk) act(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, act kudosAct,
) (domain.KudosView, error) {
	if itemID.IsZero() {
		return domain.KudosView{}, itemIDRequired()
	}
	subject, collection, err := readItemScope(ctx, d.UnitOfWork, d.Items, d.Containers, actor, itemID)
	if err != nil {
		return domain.KudosView{}, err
	}
	scope := itemsWrite
	if act == kudosReading {
		scope = itemsRead
	}
	if err := d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(collection),
		Action:     act.action(),
		TokenScope: scope,
		TargetType: itemTarget,
		TargetID:   itemID,
		On:         reading(subject),
	}); err != nil {
		return domain.KudosView{}, err
	}

	var view domain.KudosView
	within := d.UnitOfWork.Within
	if act == kudosReading {
		within = d.UnitOfWork.WithinReadOnly
	}
	err = within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Read again inside the transaction: whoever completed the entry is what the kudos go to,
		// and it may have moved since the permission question.
		item, err := findItem(ctx, d.Items, itemID)
		if err != nil {
			return err
		}
		switch act {
		case kudosGiving:
			to, err := domain.KudosTo(item, actor.AccountID)
			if err != nil {
				return err
			}
			if _, err = d.Kudos.Give(ctx, domain.Kudos{
				ItemID: item.ID, GivenBy: actor.AccountID, GivenTo: to, GivenAt: d.Clock.Now(),
			}); err != nil {
				return err
			}
		case kudosTakingBack:
			if to, err := domain.KudosTo(item, actor.AccountID); err == nil {
				if _, err = d.Kudos.TakeBack(ctx, item.ID, actor.AccountID, to); err != nil {
					return err
				}
			}
		}
		view, err = d.view(ctx, actor, item)
		return err
	})
	if err != nil {
		return domain.KudosView{}, err
	}
	return view, nil
}

// view is the reader's own side and nothing more: whether they gave kudos to the current
// completion, and what they received on the entry. Another person's kudos are never read here.
func (d KudosDesk) view(
	ctx context.Context, actor appshared.ActorContext, item domain.WorkItem,
) (domain.KudosView, error) {
	view := domain.KudosView{ItemID: item.ID}
	if to, err := domain.KudosTo(item, actor.AccountID); err == nil {
		given, err := d.Kudos.Given(ctx, item.ID, actor.AccountID, to)
		if err != nil {
			return domain.KudosView{}, err
		}
		view.Given = given
	}
	received, err := d.Kudos.Received(ctx, item.ID, actor.AccountID)
	if err != nil {
		return domain.KudosView{}, err
	}
	view.From = received
	return view, nil
}

func kudosOutput(view domain.KudosView) usecase.Output {
	from := make([]any, 0, len(view.From))
	for _, kudos := range view.From {
		from = append(from, map[string]any{"account_id": kudos.GivenBy.String(), "given_at": kudos.GivenAt})
	}
	return usecase.Output{"item_id": view.ItemID.String(), "given": view.Given, "from": from}
}

func kudosInput() []usecase.Field {
	return []usecase.Field{{
		Name: "item_id", Kind: usecase.KindID, Required: true,
		Description: "The entry whose completion is acknowledged.",
	}}
}

// Descriptor is the catalogue entry.
func (h ReadKudos) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionRead,
		Name:       ReadKudosName,
		Summary: "Reads what the caller may know of an entry's kudos: whether they gave kudos to its " +
			"completion, and - only to the person they were given to - who gave theirs. Nobody " +
			"learns who acknowledged somebody else, and nothing counts kudos per person.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input:       kudosInput(),
		Audit: usecase.AuditDeclaration{
			Action: KudosReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return invokeKudos(ctx, actor, in, h.Execute)
		}),
	}
}

// Descriptor is the catalogue entry.
func (h GiveKudos) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionRead,
		Name:       GiveKudosName,
		Summary: "Acknowledges an entry's completion to the person who completed it. Idempotent. " +
			"Refused on an open entry and on the caller's own completion. Needs only the right to " +
			"read the entry.",
		SideEffects: "Records the kudos, seen only by the person who completed the entry. The audit " +
			"trail records only a refusal. The entry, its version and its history are untouched; " +
			"nothing is announced.",
		TokenScope: itemsWrite,
		Input:      kudosInput(),
		Audit: usecase.AuditDeclaration{
			Action: KudosGivenAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: kudosExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return invokeKudos(ctx, actor, in, h.Execute)
		}),
	}
}

// Descriptor is the catalogue entry.
func (h TakeBackKudos) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionRead,
		Name:       TakeBackKudosName,
		Summary:    "Takes back the caller's kudos on an entry's current completion. Idempotent.",
		SideEffects: "Removes the caller's kudos. The audit trail records only a refusal. The entry, " +
			"its version and its history are untouched; nothing is announced.",
		TokenScope: itemsWrite,
		Input:      kudosInput(),
		Audit: usecase.AuditDeclaration{
			Action: KudosTakenBackAction, TargetType: itemTarget, Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{Exempt: kudosExempt},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			return invokeKudos(ctx, actor, in, h.Execute)
		}),
	}
}

func invokeKudos(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
	execute func(context.Context, appshared.ActorContext, shared.ID) (domain.KudosView, error),
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	view, err := execute(ctx, actor, itemID)
	if err != nil {
		return nil, err
	}
	return kudosOutput(view), nil
}
