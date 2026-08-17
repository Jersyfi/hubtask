// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package audit is the port for the evidence trail (audit.md, ADR-0017).
//
// The audit trail is not a by-product of the domain events. Events describe business changes,
// while the trail also has to describe events *without* one - a failed login, a download of an
// export, a rejected permission check - and those are exactly what an auditor asks about. So it
// is written deliberately, by the application layer, inside the same transaction as the change it
// records: no change without an entry, no entry without a change (audit.md §7).
//
// What an adapter adds and this port therefore does not carry: the sequence number, the previous
// hash, and the hash. They are the chain, and a chain computed by anything but the one component
// that appends to it is a chain with two authors (audit.md §3).
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Action is the stable code of what happened: `container.created`, `member.role_changed`,
// `auth.login_failed`. Stable because an auditor filters on it and a SIEM rule matches on it -
// renaming one is a breaking change to somebody's alerting.
type Action string

// Outcome is how it ended. DENIED is the one that makes the trail complete: an attempt that was
// refused is the entry an auditor is looking for, and it has no business change behind it.
type Outcome string

const (
	OutcomeSuccess Outcome = "SUCCESS"
	OutcomeDenied  Outcome = "DENIED"
	OutcomeFailed  Outcome = "FAILED"
)

// Severity is how much attention the entry deserves. It is not the same as the outcome: a
// successful change of the audit retention period is more interesting than a failed login.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityNotice   Severity = "NOTICE"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

var (
	outcomes   = [...]Outcome{OutcomeSuccess, OutcomeDenied, OutcomeFailed}
	severities = [...]Severity{SeverityInfo, SeverityNotice, SeverityWarning, SeverityCritical}
)

func (o Outcome) Valid() bool  { return slices.Contains(outcomes[:], o) }
func (s Severity) Valid() bool { return slices.Contains(severities[:], s) }

// Classification decides what a changed value looks like in the trail (audit.md §4).
//
// An audit log that keeps every title in full text undermines the deletion obligation of the very
// item it documents - the entry outlives the item by design, so a title recorded in clear text is
// a copy that no deletion reaches. Hence three levels rather than one.
type Classification string

const (
	// Open is a value that is not personal data and carries no content: a status, a type, a role.
	// It is recorded as it is, because an auditor has to be able to read what changed.
	Open Classification = "OPEN"
	// Sensitive is user content: a name, a title, a note. Recorded as "changed", with a hash that
	// makes two entries comparable without either of them being readable.
	Sensitive Classification = "SENSITIVE"
	// Secret is a credential, a token, or a key. Not recorded at all, in any form.
	Secret Classification = "SECRET"
)

// Change is one field of the recorded object, before masking.
type Change struct {
	Field          string
	Classification Classification
	From, To       any
}

// Changes masks a set of fields for the `changes` column.
//
// The masking happens here rather than in the adapter, because an adapter that masked would be a
// second place where a classification could be got wrong, and the wrong direction of that mistake
// writes a title into the audit trail permanently.
func Changes(changes ...Change) map[string]any {
	masked := make(map[string]any, len(changes))

	for _, change := range changes {
		switch change.Classification {
		case Secret:
			continue
		case Sensitive:
			entry := map[string]any{"changed": true}
			if digest := fingerprint(change.To); digest != "" {
				entry["to_hash"] = digest
			}
			if digest := fingerprint(change.From); digest != "" {
				entry["from_hash"] = digest
			}
			masked[change.Field] = entry
		default:
			entry := map[string]any{"to": change.To}
			if change.From != nil {
				entry["from"] = change.From
			}
			masked[change.Field] = entry
		}
	}
	return masked
}

// fingerprint is what makes two sensitive values comparable without either being readable. Not a
// password hash and not meant to be one: the input is content, not a credential, and the point is
// "did this change back to what it was", not secrecy against an offline attack.
func fingerprint(value any) string {
	if value == nil || value == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(fmt.Sprint(value)))
	return hex.EncodeToString(digest[:])
}

// Context is the request an entry belongs to (audit.md §2). The address is truncated (IPv4 /24,
// IPv6 /48) and the user agent reduced to a class, because an audit trail is evidence about
// actions, not a second analytics dataset.
type Context struct {
	RequestID      string
	TraceID        string
	IPTruncated    string
	UserAgentClass string
	APIClient      string
	RuleID         shared.ID
}

// Entry is one record of the trail. Actor and target are denormalised on purpose: an entry that
// only points at a foreign key becomes unreadable once the account is deleted, and a trail that
// loses its meaning through a deletion does not do its job (audit.md §2, test AT-7).
type Entry struct {
	TenantID   shared.ID
	OccurredAt time.Time
	Action     Action
	Outcome    Outcome
	Severity   Severity

	ActorKind  shared.ActorKind
	ActorID    shared.ID
	ActorLabel string
	// OnBehalfOf is the principal an automation rule or an agent runs as (`run_as`).
	OnBehalfOf shared.ID

	TargetType  string
	TargetID    shared.ID
	TargetLabel string

	Context Context
	// Changes is what Changes() produced: masked per field, never raw content.
	Changes map[string]any
	// LegalBasis names the occasion for a privacy-relevant entry, e.g. `dsr.erasure`.
	LegalBasis string
}

// Validate refuses an entry the trail could not stand behind. It is deliberately strict: an entry
// that cannot be written aborts the transaction it belongs to, and a business change that reaches
// the database without its audit entry is the failure this whole port exists to prevent
// (test AT-5).
func (e Entry) Validate() error {
	switch {
	case e.TenantID.IsZero(), e.Action == "", e.OccurredAt.IsZero():
		return shared.ErrInternal.WithDetail("audit.entry_incomplete")
	case !e.Outcome.Valid(), !e.Severity.Valid():
		return shared.ErrInternal.WithDetail("audit.entry_incomplete")
	case !e.ActorKind.Auditable():
		// Including the anonymous actor: a refused anonymous attempt is recorded as the actor the
		// credential claimed to be, or as SYSTEM, never as a kind the database will reject.
		return shared.ErrInternal.
			WithDetail("audit.actor_kind_invalid").
			WithParams(map[string]string{"actor_type": string(e.ActorKind)})
	}
	return nil
}

// Sink appends to the trail.
//
// It runs inside the caller's transaction: the entry and the business change commit together or
// not at all. An implementation therefore never opens a transaction of its own and never retries
// on its own - a retried append would be a second entry for one action.
type Sink interface {
	Append(ctx context.Context, entry Entry) error
}
