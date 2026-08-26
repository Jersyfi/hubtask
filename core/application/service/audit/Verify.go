// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	VerifyAuditChainName = "VerifyAuditChain"

	// ChainBrokenAction is recorded when a verification finds a break, and only then.
	//
	// A clean check writes nothing, for the reason a read does: the chain would grow by being
	// checked. A break is the opposite case - it is the most serious thing this system can
	// discover about itself, and an auditor reading the trail months later has to be able to see
	// that somebody noticed and when. The outcome is FAILED because the check did not come out
	// right; the operation itself worked, and saying so would put the finding in the one field
	// nobody filters on.
	ChainBrokenAction port.Action = "audit.chain_broken"

	// maxReportedGaps bounds the missing sequence numbers in one answer.
	//
	// A chain with a hole of a million entries would otherwise answer with a million integers, and
	// the client asking whether the trail is intact has its answer after the first one. The count
	// is reported whole; the list is what is cut.
	maxReportedGaps = 100
)

// VerifyAuditChain walks a period of the trail and checks that it has not been rewritten.
//
// The recomputation goes through the port that appended the entries (`audit.Chain`), which is the
// whole point: a verifier with an implementation of its own would prove that two implementations
// agree rather than that the chain is intact. `Canonical` was exported for exactly this.
type VerifyAuditChain struct {
	Trail      repository.Trail
	Chain      port.Chain
	Authorizer Authorizer
	Audit      port.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Verification is what one check found.
type Verification struct {
	// Valid is true when every entry hashes to what is stored beside it, every entry links to its
	// predecessor, and no sequence number is missing.
	Valid bool
	// Checked is how many entries were examined.
	Checked int
	// FirstBrokenSeq is the first entry whose hash or whose link does not hold, and zero when
	// there is none. The first rather than all of them: it is where an investigation starts.
	FirstBrokenSeq int64
	// Gaps are the missing sequence numbers, cut at maxReportedGaps.
	Gaps []int64
	// GapCount is how many are missing in total, which may be more than Gaps holds.
	GapCount int
	// SealedUntil is when this tenant's chain was last anchored outside the database, and the zero
	// time when it never was - which is every installation today (audit.md §3).
	SealedUntil time.Time
}

// Execute checks the chain over a period.
func (h VerifyAuditChain) Execute(
	ctx context.Context, actor appshared.ActorContext, period repository.Period,
) (Verification, error) {
	// The whole trail, always. There is no narrowed verification: a chain checked over one
	// actor's entries would be a chain with every other entry missing, and the answer would be
	// "broken" for a trail that is intact.
	if err := h.Authorizer.Authorize(ctx, actor, verifyRequest(actor)); err != nil {
		return Verification{}, err
	}
	if period.From.IsZero() || period.To.IsZero() {
		return Verification{}, shared.ErrValidation.
			WithDetail("audit.period_required").
			WithFields(shared.FieldError{Path: "/from", Code: "audit.period_required"})
	}
	if !period.To.After(period.From) {
		return Verification{}, shared.ErrValidation.
			WithDetail("audit.period_invalid").
			WithFields(shared.FieldError{Path: "/to", Code: "audit.period_invalid"})
	}

	var found Verification
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		anchor, err := h.Trail.LatestAnchor(ctx)
		if err != nil {
			return err
		}
		found.SealedUntil = anchor.AnchoredAt

		return h.walk(ctx, period, &found)
	})
	if err != nil {
		return Verification{}, err
	}

	found.Valid = found.FirstBrokenSeq == 0 && found.GapCount == 0
	if !found.Valid {
		if err := h.recordTheBreak(ctx, actor, found); err != nil {
			return Verification{}, err
		}
	}
	return found, nil
}

// walk is the check itself: one pass, three questions per entry.
func (h VerifyAuditChain) walk(
	ctx context.Context, period repository.Period, found *Verification,
) error {
	var previous repository.Record
	seen := false

	return h.Trail.Walk(ctx, period, func(record repository.Record) error {
		found.Checked++

		// The hash the sink computed, computed again from the row as it stands. A row somebody
		// edited in the database hashes to something else, whatever else they remembered to change.
		expected, err := h.Chain.Link(record.PrevHash, record.ID, record.Seq, record.Entry)
		if err != nil {
			return err
		}
		if !bytes.Equal(expected, record.Hash) {
			found.note(record.Seq)
		}

		if seen {
			// The link, which is the half a per-row check cannot see: an entry whose own hash is
			// fine but whose predecessor's digest is not the one before it is an entry that was
			// moved, or a predecessor that was removed.
			if !bytes.Equal(record.PrevHash, previous.Hash) {
				found.note(record.Seq)
			}
			for missing := previous.Seq + 1; missing < record.Seq; missing++ {
				found.GapCount++
				if len(found.Gaps) < maxReportedGaps {
					found.Gaps = append(found.Gaps, missing)
				}
			}
		}

		previous, seen = record, true
		return nil
	})
}

// note keeps the first break. The first rather than every one, because that is where an
// investigation starts - and because a chain broken at entry 4,000 reports every entry after it
// as suspicious, which is a list nobody can act on.
func (v *Verification) note(seq int64) {
	if v.FirstBrokenSeq == 0 || seq < v.FirstBrokenSeq {
		v.FirstBrokenSeq = seq
	}
}

// recordTheBreak writes the one entry a verification produces.
func (h VerifyAuditChain) recordTheBreak(
	ctx context.Context, actor appshared.ActorContext, found Verification,
) error {
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Audit.Append(ctx, port.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: ChainBrokenAction, Outcome: port.OutcomeFailed,
			Severity:  port.SeverityCritical,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: trailTarget, TargetID: actor.TenantID,
			Context: port.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: port.Changes(
				port.Change{
					Field: "first_broken_seq", Classification: port.Open,
					To: found.FirstBrokenSeq,
				},
				port.Change{Field: "gaps", Classification: port.Open, To: found.GapCount},
				port.Change{Field: "checked", Classification: port.Open, To: found.Checked},
			),
		})
	})
}

// verifyRequest is what a verification asks for: the whole trail, under the reading scope. Checking
// the chain reads nothing a read of the trail would not, and a second scope for it would be a scope
// nobody could explain.
func verifyRequest(actor appshared.ActorContext) access.Request {
	request := wholeTrail(actor)
	request.Action = ChainBrokenAction
	return request
}

// Descriptor registers the check in all three channels.
func (h VerifyAuditChain) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: VerifyAuditChainName,
		Summary: "Checks that the audit trail of a period has not been rewritten: every entry " +
			"against its own hash, every entry against the one before it, and the sequence " +
			"numbers against their own arithmetic. Answers where the first break is and which " +
			"numbers are missing. What it proves is that nothing was changed inside the " +
			"database; against somebody who can rewrite all of it, only an anchor kept outside " +
			"would say anything, and `sealed_until` reports whether there is one.",
		SideEffects: "None while the chain holds. A break is recorded as a critical entry of its " +
			"own, because an auditor reading the trail later has to see that somebody noticed.",
		TokenScope: auditRead,
		ReadOnly:   true,
		Input: []usecase.Field{
			{
				Name: "from", Kind: usecase.KindString, Required: true,
				Description: "The start of the period, inclusive. RFC 3339.",
			},
			{
				Name: "to", Kind: usecase.KindString, Required: true,
				Description: "The end of the period, exclusive. RFC 3339.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ChainBrokenAction, TargetType: trailTarget,
			Severity: port.SeverityCritical, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h VerifyAuditChain) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	from, err := parseInstant(in.String("from"), "from")
	if err != nil {
		return nil, err
	}
	to, err := parseInstant(in.String("to"), "to")
	if err != nil {
		return nil, err
	}

	found, err := h.Execute(ctx, actor, repository.Period{From: from, To: to})
	if err != nil {
		return nil, err
	}
	return VerificationOutput(found), nil
}

// VerificationOutput is the answer as every channel renders it.
//
// `first_broken_seq` and `sealed_until` are explicit nulls rather than absent fields: a client
// reading "no break" out of a missing key would read the same thing out of a key it forgot.
func VerificationOutput(found Verification) usecase.Output {
	out := usecase.Output{
		"valid":            found.Valid,
		"checked":          found.Checked,
		"first_broken_seq": nil,
		"gaps":             found.Gaps,
		"gap_count":        found.GapCount,
		"sealed_until":     nil,
	}
	if found.FirstBrokenSeq != 0 {
		out["first_broken_seq"] = found.FirstBrokenSeq
	}
	if !found.SealedUntil.IsZero() {
		out["sealed_until"] = found.SealedUntil.UTC()
	}
	if found.Gaps == nil {
		out["gaps"] = []int64{}
	}
	return out
}
