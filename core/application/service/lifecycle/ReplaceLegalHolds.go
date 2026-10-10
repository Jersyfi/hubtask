// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ReplaceLegalHoldsName = "ReplaceLegalHolds"

	// HoldReplacedAction is the entry a hold placed again after a point-in-time recovery leaves in
	// its workspace's trail. Its own action rather than `lifecycle.hold_placed`: the hold was placed
	// by its owner, at its own moment, and that entry was rewound away with it - this one records
	// that the operator put it back, and from which recovery.
	HoldReplacedAction audit.Action = "lifecycle.hold_replaced"

	// operatorScope is the control plane's scope: re-placing holds crosses workspaces, and it is
	// the operator's act in the recovery procedure (backup-restore.md §8.5 step 5).
	operatorScope = "admin:tenants"

	// maxReplacedHolds bounds one call. A workspace with more holds than this is unheard of; one
	// that has them is placed again in several calls, each idempotent.
	maxReplacedHolds = 1000

	codeRecoveryPointRequired = "lifecycle.recovery_point_required"
	codeHoldsRequired         = "lifecycle.holds_required"
	codeTooManyHolds          = "lifecycle.too_many_holds"
)

// Workspace is the one question the re-placement asks of the workspace itself: whether it is
// there. The admin context's tenant store answers it.
type Workspace interface {
	Find(ctx context.Context) (adminrepo.TenantRecord, error)
}

// ReplaceLegalHolds places the legal holds of a rewound period again, after a point-in-time
// recovery and before traffic is admitted (backup-restore.md §8.5 step 5, data-protection.md §5).
//
// The holds arrive from outside the recovered database - which source is the procedure's question,
// not this use case's - and each is placed as it was: the same identifier, scope, reason, placer
// and moment. One released in the rewound period is placed in force all the same, and reported so
// that its owner releases it again. One the workspace already has is left alone, so a second run
// changes nothing. One whose target the recovery removed is placed anyway and reported: it protects
// nothing until the target comes back, and refusing it would drop a hold a later restore may need.
type ReplaceLegalHolds struct {
	Holds      Holds
	Workspaces Workspace
}

// HoldRecord is one hold as it stood before the rewind.
type HoldRecord struct {
	ID       shared.ID
	Scope    domain.HoldScope
	ScopeID  shared.ID
	Reason   string
	PlacedBy shared.ID
	PlacedAt time.Time
	// ReleasedAt is when it was released in the rewound period; zero if it was not.
	ReleasedAt time.Time
}

// ReplaceLegalHoldsCommand is the input, typed.
type ReplaceLegalHoldsCommand struct {
	TenantID      shared.ID
	RecoveryPoint time.Time
	Holds         []HoldRecord
}

// ReplacementOutcome says what this run did with one hold.
type ReplacementOutcome string

const (
	// HoldPlacedAgain is a hold this run placed.
	HoldPlacedAgain ReplacementOutcome = "PLACED"
	// HoldAlreadyPresent is a hold the workspace already had; it was left as it is.
	HoldAlreadyPresent ReplacementOutcome = "PRESENT"
)

// Replacement is what happened to one hold.
type Replacement struct {
	ID               shared.ID
	Outcome          ReplacementOutcome
	TargetPresent    bool
	ReleasedInPeriod bool
}

// Execute places the holds again and answers what happened to each, in the order given.
func (h ReplaceLegalHolds) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ReplaceLegalHoldsCommand,
) ([]Replacement, error) {
	if err := actor.RequireScope(operatorScope); err != nil {
		return nil, err
	}
	if cmd.TenantID.IsZero() {
		return nil, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	holds, err := h.validated(cmd)
	if err != nil {
		return nil, err
	}

	var replaced []Replacement
	scope := persistenceScope(cmd.TenantID, actor)
	err = h.Holds.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if _, err := h.Workspaces.Find(ctx); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("admin.tenant_not_found")
			}
			return err
		}
		// The exclusive hold lock, as placing one takes it: a deletion that read the holds waits
		// for this transaction, so nothing these holds keep goes in between.
		if err := h.Holds.Holds.Lock(ctx); err != nil {
			return err
		}
		replaced = make([]Replacement, 0, len(holds))
		for i, hold := range holds {
			outcome, err := h.replace(ctx, actor, cmd, hold, !cmd.Holds[i].ReleasedAt.IsZero())
			if err != nil {
				return err
			}
			replaced = append(replaced, outcome)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return replaced, nil
}

// validated turns the records into holds, refusing the whole call over one bad record: a partial
// re-placement would leave the operator working out which half went in.
func (h ReplaceLegalHolds) validated(cmd ReplaceLegalHoldsCommand) ([]domain.LegalHold, error) {
	switch {
	case cmd.RecoveryPoint.IsZero():
		return nil, shared.ErrValidation.WithDetail(codeRecoveryPointRequired).
			WithFields(shared.FieldError{Path: "/recovery_point", Code: codeRecoveryPointRequired})
	case len(cmd.Holds) == 0:
		return nil, shared.ErrValidation.WithDetail(codeHoldsRequired).
			WithFields(shared.FieldError{Path: "/holds", Code: codeHoldsRequired})
	case len(cmd.Holds) > maxReplacedHolds:
		return nil, shared.ErrValidation.WithDetail(codeTooManyHolds).
			WithParams(map[string]string{"maximum": strconv.Itoa(maxReplacedHolds)}).
			WithFields(shared.FieldError{Path: "/holds", Code: codeTooManyHolds})
	}
	holds := make([]domain.LegalHold, 0, len(cmd.Holds))
	for i, record := range cmd.Holds {
		hold, err := domain.ReplacedLegalHold(domain.ReplacedHoldInput{
			ID: record.ID, Scope: record.Scope, ScopeID: record.ScopeID, Reason: record.Reason,
			PlacedBy: record.PlacedBy, PlacedAt: record.PlacedAt, Text: h.Holds.Text,
		})
		if err != nil {
			code := shared.AsError(err).DetailCode
			return nil, shared.ErrValidation.WithDetail(code).
				WithFields(shared.FieldError{Path: fmt.Sprintf("/holds/%d", i), Code: code}).
				WithCause(err)
		}
		holds = append(holds, hold)
	}
	return holds, nil
}

// replace places one hold unless the workspace has it, and records that it did.
func (h ReplaceLegalHolds) replace(
	ctx context.Context, actor appshared.ActorContext, cmd ReplaceLegalHoldsCommand,
	hold domain.LegalHold, released bool,
) (Replacement, error) {
	present := true
	if hold.Scope != domain.HoldTenant {
		var err error
		if present, err = h.Holds.Holds.TargetExists(ctx, hold.Scope, hold.ScopeID); err != nil {
			return Replacement{}, err
		}
	}
	result := Replacement{ID: hold.ID, TargetPresent: present, ReleasedInPeriod: released}

	if _, err := h.Holds.Holds.Find(ctx, hold.ID); err == nil {
		result.Outcome = HoldAlreadyPresent
		return result, nil
	} else if !errors.Is(err, shared.ErrNotFound) {
		return Replacement{}, err
	}

	if err := h.Holds.Holds.Place(ctx, hold); err != nil {
		return Replacement{}, err
	}
	result.Outcome = HoldPlacedAgain
	return result, h.Holds.Audit.Append(ctx, audit.Entry{
		TenantID: cmd.TenantID, OccurredAt: h.Holds.Clock.Now(),
		Action: HoldReplacedAction, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: holdTarget, TargetID: hold.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "scope_kind", Classification: audit.Open, To: string(hold.Scope)},
			audit.Change{Field: "scope_id", Classification: audit.Open, To: hold.ScopeID.String()},
			audit.Change{Field: "reason", Classification: audit.Open, To: hold.Reason},
			audit.Change{Field: "placed_by", Classification: audit.Open, To: hold.PlacedBy.String()},
			audit.Change{Field: "placed_at", Classification: audit.Open,
				To: hold.PlacedAt.UTC().Format(time.RFC3339Nano)},
			audit.Change{Field: "recovery_point", Classification: audit.Open,
				To: cmd.RecoveryPoint.UTC().Format(time.RFC3339Nano)},
			audit.Change{Field: "released_in_period", Classification: audit.Open,
				To: strconv.FormatBool(released)},
		),
	})
}

// persistenceScope is the target workspace's transaction, opened by the operator.
func persistenceScope(tenantID shared.ID, actor appshared.ActorContext) persistence.Scope {
	return persistence.Scope{TenantID: tenantID, ActorID: actor.AccountID}
}

// Descriptor is the catalogue entry.
func (h ReplaceLegalHolds) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReplaceLegalHoldsName,
		Summary: "After a point-in-time recovery of the installation, places again the legal " +
			"holds the rewind removed from one workspace, each with its own identifier, scope, " +
			"reason, placer and moment. A hold released in the rewound period is placed in force " +
			"and reported, so that its owner releases it again; one the workspace already has is " +
			"left alone. The operator's, before traffic is admitted.",
		SideEffects: "Writes each missing hold and a lifecycle.hold_replaced entry into the " +
			"workspace's trail. Nothing is deleted.",
		TokenScope: operatorScope,
		Input: []usecase.Field{
			{Name: "tenant_id", Kind: usecase.KindID, Required: true},
			{
				Name: "recovery_point", Kind: usecase.KindString, Format: usecase.FormatDateTime,
				Required:    true,
				Description: "The moment the installation was recovered to, RFC 3339.",
			},
			{
				Name: "holds", Kind: usecase.KindList, Required: true,
				Description: "The holds as they stood before the rewind, at most 1000: each with " +
					"id, scope (kind and id), reason, placed_by, placed_at and, if it was released " +
					"in the rewound period, released_at.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: HoldReplacedAction, TargetType: holdTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the control plane acts on workspaces, not on items.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReplaceLegalHolds) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	tenantID, err := in.ID("tenant_id")
	if err != nil {
		return nil, err
	}
	cmd := ReplaceLegalHoldsCommand{TenantID: tenantID}
	if raw := in.String("recovery_point"); raw != "" {
		if cmd.RecoveryPoint, err = time.Parse(time.RFC3339Nano, raw); err != nil {
			return nil, shared.ErrValidation.WithDetail(codeRecoveryPointRequired).
				WithFields(shared.FieldError{Path: "/recovery_point", Code: codeRecoveryPointRequired})
		}
	}
	list, _ := in["holds"].([]any)
	for i, item := range list {
		record, err := holdRecordOf(item)
		if err != nil {
			return nil, shared.ErrValidation.WithDetail(domain.CodeHoldIncomplete).
				WithFields(shared.FieldError{Path: fmt.Sprintf("/holds/%d", i), Code: domain.CodeHoldIncomplete}).
				WithCause(err)
		}
		cmd.Holds = append(cmd.Holds, record)
	}

	replaced, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	rows := make([]any, 0, len(replaced))
	for _, r := range replaced {
		rows = append(rows, usecase.Output{
			"id": r.ID.String(), "outcome": string(r.Outcome),
			"target_present": r.TargetPresent, "released_in_period": r.ReleasedInPeriod,
		})
	}
	return usecase.Output{"holds": rows}, nil
}

// holdRecordOf reads one hold record as the channels deliver it: a JSON object of strings.
func holdRecordOf(item any) (HoldRecord, error) {
	fields, ok := item.(map[string]any)
	if !ok {
		return HoldRecord{}, errors.New("a hold record is not an object")
	}
	text := func(object map[string]any, key string) string {
		value, _ := object[key].(string)
		return value
	}
	moment := func(key string) (time.Time, error) {
		raw := text(fields, key)
		if raw == "" {
			return time.Time{}, nil
		}
		return time.Parse(time.RFC3339Nano, raw)
	}
	id := func(object map[string]any, key string) (shared.ID, error) {
		raw := text(object, key)
		if raw == "" {
			return "", nil
		}
		return shared.ParseID(raw)
	}

	record := HoldRecord{Reason: text(fields, "reason")}
	scope, _ := fields["scope"].(map[string]any)
	record.Scope = domain.HoldScope(text(scope, "kind"))
	var err error
	if record.ID, err = id(fields, "id"); err != nil {
		return HoldRecord{}, err
	}
	if record.ScopeID, err = id(scope, "id"); err != nil {
		return HoldRecord{}, err
	}
	if record.PlacedBy, err = id(fields, "placed_by"); err != nil {
		return HoldRecord{}, err
	}
	if record.PlacedAt, err = moment("placed_at"); err != nil {
		return HoldRecord{}, err
	}
	if record.ReleasedAt, err = moment("released_at"); err != nil {
		return HoldRecord{}, err
	}
	return record, nil
}
