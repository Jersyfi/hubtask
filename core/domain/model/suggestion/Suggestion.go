// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package suggestion is what AI proposed, and did not do (J-05, ADR-0012).
//
// It is its own package rather than a corner of `work` or `integration`, and the reason is the one
// ai-first.md opens with: "what it does not mean: AI in the domain model". A suggestion influences
// no invariant of anything - no entry is valid or invalid because of one, no rule fires on one, and
// deleting every row here changes nothing about the workspace. Keeping it in a package that nothing
// else imports is how that stays true rather than being asserted.
//
// The aggregate is small and deliberately dumb. What it knows is: what was proposed, about what,
// where it came from, what it was made from, and whether somebody has decided about it. What it
// does not know is how to apply itself - that is the application layer's, because applying means
// calling an ordinary use case with an ordinary permission check, and a domain object that could
// apply itself would be one that could grant somebody something.
package suggestion

import (
	"crypto/sha256"
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Suggestion is one proposal about one entry.
type Suggestion struct {
	ID         shared.ID
	TenantID   shared.ID
	TargetType TargetType
	TargetID   shared.ID
	Kind       Kind
	Status     Status
	// Payload is what was proposed, in the shape the kind fixes. It is data, and the one thing
	// this package will never do with it is read it as an instruction (ai-first.md §1.3).
	Payload map[string]any
	Provenance
	// InputDigest is the fingerprint of what the suggestion was made from.
	InputDigest []byte
	CreatedAt   time.Time
	DecidedAt   time.Time
	DecidedBy   shared.ID
	Version     int
}

// Provenance is where a proposal came from, and it is the reason a suggestion is stored at all
// rather than applied and forgotten (ai-first.md §2, ADR-0018's traceability row).
type Provenance struct {
	// Source is where the proposal came from. One value today, and a field rather than an
	// assumption: the day a second source exists, the rows written before it must not become
	// ambiguous.
	Source Source
	// Model is what the provider said answered, which is not always what was configured - a
	// provider resolving an alias answers under the resolved name. For a proposal read off the
	// embedding index rather than from a completion, it is the *embedding* model whose vectors
	// were compared: a similarity means nothing outside one model's space (K-04).
	Model string
	// PromptID and PromptVersion resolve to the words that produced this, because prompt files
	// are versioned in their names and the old ones stay (ADR-0049 decision 3). Both empty for a
	// proposal no prompt produced, and never one without the other.
	PromptID      string
	PromptVersion string
	// ProducedAt is when the provider answered, which is not when the record was written: a
	// queued suggestion can be minutes older than its row.
	ProducedAt time.Time
}

// Source is where a proposal came from.
type Source string

// SourceAI is the only one there is. Written down rather than assumed, for the reason above.
const SourceAI Source = "AI"

// TargetType is what a suggestion is about.
type TargetType string

const (
	TargetWorkItem    TargetType = "WORK_ITEM"
	TargetJumbleEntry TargetType = "JUMBLE_ENTRY"
)

var targetTypes = []TargetType{TargetWorkItem, TargetJumbleEntry}

// TargetTypes is the closed set, in the contract's order.
func TargetTypes() []TargetType { return slices.Clone(targetTypes) }

// Valid reports whether the target type is one of the defined ones.
func (t TargetType) Valid() bool { return slices.Contains(targetTypes, t) }

// Kind is what accepting does, which is the only thing a kind has to say.
//
// Three, and there is a reason there are not six. A summary and a classification are FIELDS
// suggestions whose payload happens to be notes or labels - the act of accepting them is the same
// act - and a kind per *feature* rather than per *effect* would be a closed set that grows with
// every feature and tells the acceptance nothing it does not already know.
type Kind string

const (
	// KindFields proposes values for the target entry.
	KindFields Kind = "FIELDS"
	// KindDecomposition proposes a tree of entries under the target.
	KindDecomposition Kind = "DECOMPOSITION"
	// KindDuplicates says which entries look like the target, and is the one kind nothing accepts
	// (K-04). It follows the rule above rather than breaking it: what accepting *would* do is
	// nothing, because deciding two entries are the same is a person's act through the ordinary
	// use cases - archive one, trash one, move one under the other - and a proposal cannot know
	// which of those they mean. So it is a kind of its own precisely because its effect is its
	// own, and `:dismiss` is what closes it.
	KindDuplicates Kind = "DUPLICATES"
)

var kinds = []Kind{KindFields, KindDecomposition, KindDuplicates}

// Kinds is the closed set, in the contract's order.
func Kinds() []Kind { return slices.Clone(kinds) }

// Valid reports whether the kind is one of the defined ones.
func (k Kind) Valid() bool { return slices.Contains(kinds, k) }

// Status is whether somebody has decided about it.
type Status string

const (
	StatusProposed Status = "PROPOSED"
	// StatusAccepted is what happened, not what changed: the change is the target's own history.
	StatusAccepted Status = "ACCEPTED"
	// StatusDismissed is a state and not a deletion, so "what was proposed and turned down" has an
	// answer; the retention engine ages both decided states out by rule.
	StatusDismissed Status = "DISMISSED"
)

var statuses = []Status{StatusProposed, StatusAccepted, StatusDismissed}

// Statuses is the closed set, in the contract's order.
func Statuses() []Status { return slices.Clone(statuses) }

// Valid reports whether the status is one of the defined ones.
func (s Status) Valid() bool { return slices.Contains(statuses, s) }

// Decided reports whether somebody has already answered this suggestion.
func (s Status) Decided() bool { return s == StatusAccepted || s == StatusDismissed }

// NewInput is what recording a suggestion needs.
type NewInput struct {
	ID          shared.ID
	TenantID    shared.ID
	TargetType  TargetType
	TargetID    shared.ID
	Kind        Kind
	Payload     map[string]any
	Provenance  Provenance
	InputDigest []byte
	Now         time.Time
}

// New validates a proposal and records it as PROPOSED.
//
// Every refusal here is an internal error rather than a validation failure, and that is not an
// oversight: nobody submits a suggestion. They are produced by this system's own jobs from this
// system's own prompts, so a malformed one is a defect in the producer and never somebody's input.
// Answering `validation_failed` would send whoever is reading the log looking for a caller who
// does not exist.
func New(in NewInput) (Suggestion, error) {
	switch {
	case in.ID.IsZero(), in.TenantID.IsZero(), in.TargetID.IsZero(), in.Now.IsZero():
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.incomplete")
	case !in.TargetType.Valid():
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.target_type_unknown")
	case !in.Kind.Valid():
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.kind_unknown")
	case len(in.Payload) == 0:
		// A suggestion that proposes nothing is one somebody would accept to no effect, and a
		// list of them is an inbox of noise. The producer decides there is nothing to propose and
		// records nothing.
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.payload_empty")
	case len(in.InputDigest) == 0:
		// Without it, staleness cannot be judged, and a suggestion nobody can call stale is one
		// that will eventually be applied to something it was not made from.
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.input_digest_required")
	}

	provenance := in.Provenance
	provenance.Source = SourceAI
	if provenance.Model == "" || provenance.ProducedAt.IsZero() {
		// The whole point of the record. A suggestion without provenance is indistinguishable
		// from a person's own draft, which is exactly what ADR-0012 promised it never would be.
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.provenance_incomplete")
	}
	if (provenance.PromptID == "") != (provenance.PromptVersion == "") {
		// A prompt and its version, or neither. Neither is what a proposal no prompt produced
		// carries - the nearest neighbours of an entry's vector are a query, and the honest answer
		// to "which prompt produced this" is that none did (K-04) - and half of the pair is a
		// record that resolves to nothing, which is what the versioning exists to prevent
		// (ADR-0049 decision 3).
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.provenance_incomplete")
	}
	provenance.ProducedAt = provenance.ProducedAt.UTC()

	return Suggestion{
		ID: in.ID, TenantID: in.TenantID,
		TargetType: in.TargetType, TargetID: in.TargetID,
		Kind: in.Kind, Status: StatusProposed, Payload: in.Payload,
		Provenance: provenance, InputDigest: slices.Clone(in.InputDigest),
		CreatedAt: in.Now.UTC(), Version: 1,
	}, nil
}

// ErrAlreadyDecided is a suggestion somebody has already answered. A conflict rather than a
// not-found: the record is there and readable, and what cannot happen again is the deciding.
var ErrAlreadyDecided = shared.ErrConflict.WithDetail("suggestions.already_decided")

// ErrStale is a suggestion made from a state of the entry that no longer holds.
//
// A conflict for the same reason a version conflict is one: nothing is wrong with the request, the
// world moved. Applying it would attach a proposal about one state of an entry to a different one,
// which is the failure mode a fingerprint exists to catch - it looks like a working feature and
// produces a task nobody recognises.
var ErrStale = shared.ErrConflict.WithDetail("suggestions.stale")

// Decide answers a proposal, once.
func (s Suggestion) Decide(status Status, by shared.ID, at time.Time) (Suggestion, error) {
	if s.Status.Decided() {
		return Suggestion{}, ErrAlreadyDecided
	}
	if !status.Decided() {
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.decision_unknown")
	}
	if by.IsZero() || at.IsZero() {
		// A suggestion is decided by a person. No rule and no job writes here, and the column
		// constraint in the schema says the same thing from the other side.
		return Suggestion{}, shared.ErrInternal.WithDetail("suggestions.decider_required")
	}

	decided := s
	decided.Status = status
	decided.DecidedBy, decided.DecidedAt = by, at.UTC()
	decided.Version = s.Version + 1
	return decided, nil
}

// Fresh reports whether this suggestion was made from the state the digest describes.
func (s Suggestion) Fresh(digest []byte) bool {
	return len(digest) > 0 && slices.Equal(s.InputDigest, digest)
}

// Digest is the fingerprint of the state a suggestion was made from.
//
// It lives here rather than in whoever computes it, because "the same state" has to mean the same
// thing to the producer and to the acceptance - two implementations of one hash is a staleness
// check that passes when it should fail. The parts are joined with a separator that cannot occur
// in the values, so that ("ab", "c") and ("a", "bc") do not collide into the same state.
//
// SHA-256 and not a cheaper hash: the input is somebody's title and notes, the output is stored,
// and a collision here would apply a proposal to text it was not made from. It is not a credential
// and this is not a security boundary - what it protects against is an accident, and it costs
// microseconds.
func Digest(parts ...string) []byte {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(part))
		// A byte no UTF-8 text contains, so the separator cannot appear inside a part.
		_, _ = hash.Write([]byte{0xff})
	}
	return hash.Sum(nil)
}
