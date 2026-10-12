// SPDX-License-Identifier: Apache-2.0
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
// that appends to it is a chain with two authors (audit.md §3). The sink also keys the fingerprints
// of sensitive values, through KeyFingerprints and the Fingerprinter it is handed - before the
// chain, so that the chain covers what is stored.
package audit

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
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
	// Sensitive is user content: a name, a title, a note. Recorded as "changed", with a keyed
	// hash that makes two entries comparable without either of them being readable.
	Sensitive Classification = "SENSITIVE"
	// Secret is a credential, a token, or a key. Not recorded at all, in any form.
	Secret Classification = "SECRET"
)

// MaskingFor derives what the trail may carry from what the data *is*.
//
// Three masking levels from six classes, and the derivation is the whole point of writing it down:
// `data-protection.md` §3 says what a field is, `audit.md` §4 says how a value of it appears in the
// trail, and two vocabularies with no stated relationship would leave every use case deciding the
// masking for itself and nothing able to check the decision.
//
// The one exception is stated rather than derived: the actor's *label* is `PERSONAL_BASIC` and is
// carried in clear, because an entry that only pointed at a foreign key becomes unreadable once
// the account is deleted (§2), and an erasure answers it with a pseudonym at the boundary instead
// (§6).
func MaskingFor(class shared.DataClass) Classification {
	switch class {
	case shared.ClassSecret:
		return Secret
	case shared.ClassNonPersonal, shared.ClassPersonalTechnical:
		// Technical data is already reduced where it is written - an address truncated to a
		// prefix, a user agent to a class - so what reaches the trail is not the datum itself.
		return Open
	default:
		// Everything that is a person's: their identity, their words, and the free text that may
		// hold a special category without anybody intending it.
		return Sensitive
	}
}

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
		case "":
			// Fail closed. An unclassified field falling through to OPEN would be the one direction
			// that cannot be taken back: a title written into the trail in clear text is a copy no
			// deletion reaches (§4). Gate PG-1 refuses one at build time; this refuses one at run
			// time, in case something builds a change from a variable.
			masked[change.Field] = map[string]any{"changed": true, "unclassified": true}
		case Secret:
			continue
		case Sensitive:
			entry := map[string]any{"changed": true}
			if value, ok := deferValue(change.To); ok {
				entry["to_hash"] = value
			}
			if value, ok := deferValue(change.From); ok {
				entry["from_hash"] = value
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

// Redacted is what a sensitive value awaiting its fingerprint shows wherever it is printed.
const Redacted = "[sensitive]"

// unkeyed is a sensitive value on its way to the sink, which replaces it with its keyed
// fingerprint (KeyFingerprints).
//
// The fingerprint is not computed here because its key is the installation's, and Changes is
// called from a hundred places that have no business holding it. So the value travels to the one
// place that does, and cannot be read on the way: every `fmt` verb and every log attribute shows
// Redacted (rule 10), and it refuses to become JSON, so a path that skipped the sink fails closed
// instead of storing something.
type unkeyed struct{ value string }

// deferValue canonicalises the value as the fingerprint will see it. Absent and empty have no
// fingerprint: recording one would read as "it was empty" rather than "it did not exist".
func deferValue(value any) (unkeyed, bool) {
	if value == nil || value == "" {
		return unkeyed{}, false
	}
	return unkeyed{value: fmt.Sprint(value)}, true
}

func (unkeyed) String() string   { return Redacted }
func (unkeyed) GoString() string { return Redacted }

// Format takes precedence over String and GoString for every verb, %x and %q included.
func (unkeyed) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, Redacted) }

func (unkeyed) LogValue() slog.Value { return slog.StringValue(Redacted) }

func (unkeyed) MarshalJSON() ([]byte, error) {
	return nil, shared.ErrInternal.WithDetail("audit.fingerprint_unkeyed")
}

// KeyFingerprints replaces every sensitive value awaiting its fingerprint with the keyed one, in a
// copy: the caller's maps are left as they were. Without a fingerprinter the value is dropped and
// the field records "changed" alone - the safe direction, never an unkeyed hash.
func KeyFingerprints(entry Entry, fingerprints Fingerprinter) Entry {
	if len(entry.Changes) == 0 {
		return entry
	}
	keyed := make(map[string]any, len(entry.Changes))
	for field, change := range entry.Changes {
		masked, ok := change.(map[string]any)
		if !ok {
			keyed[field] = change
			continue
		}
		masked = maps.Clone(masked)
		for key, value := range masked {
			pending, ok := value.(unkeyed)
			if !ok {
				continue
			}
			if fingerprints == nil {
				delete(masked, key)
				continue
			}
			masked[key] = fingerprints.Fingerprint(entry.TenantID, pending.value)
		}
		keyed[field] = masked
	}
	entry.Changes = keyed
	return entry
}

// Fingerprinter keys the fingerprint of a sensitive value (audit.md §4).
//
// Keyed, because an unkeyed hash of a name or a title is readable to anybody who can guess it:
// whoever reads the trail hashes candidates until one matches, and the trail outlives the item
// whose deletion it was meant not to undermine. The key is the installation's, never in the
// database, and the workspace is bound in, so two workspaces' fingerprints of one value differ.
// Which construction it is belongs to the adapter; the answer names its scheme as a prefix, so a
// reader can tell it from a fingerprint written under another one.
type Fingerprinter interface {
	Fingerprint(tenantID shared.ID, value string) string
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

// Chain recomputes what the sink computed, so that a verification can check a stored entry against
// its own hash (audit.md §3, `POST /audit:verify`).
//
// A port rather than a package the application calls, because the chain belongs to the adapter that
// appends to it - the sequence number, the previous hash and the hash are what an adapter adds, and
// this interface is the one seam through which the application may ask for the same arithmetic
// again. There is deliberately no second implementation: a verifier with its own would prove that
// two implementations agree rather than that the chain is intact.
type Chain interface {
	// Link is the hash an entry has, given the hash of the one before it. The same inputs the sink
	// used, in the same order, or the recomputation means nothing.
	Link(previousHash []byte, id shared.ID, seq int64, entry Entry) ([]byte, error)
}

// Sink appends to the trail.
//
// It runs inside the caller's transaction: the entry and the business change commit together or
// not at all. An implementation therefore never opens a transaction of its own and never retries
// on its own - a retried append would be a second entry for one action.
type Sink interface {
	Append(ctx context.Context, entry Entry) error
}
