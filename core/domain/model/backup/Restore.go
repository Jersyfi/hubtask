// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// RestoreMode is what a restore does with what it reads (backup-restore.md §8.2).
//
// Six values and not one of them a variation on another. That is worth saying because the table in
// §8.2 reads like a spectrum from "look" to "replace everything", and treating it as one would
// produce a single code path with five flags - the shape in which "INSPECT changed something" is a
// missed branch rather than an impossibility.
type RestoreMode string

const (
	// RestoreInspect reads the archive and reports the difference against the current state. It
	// changes nothing, and that is enforced rather than intended: the mode never opens a writing
	// transaction at all.
	RestoreInspect RestoreMode = "INSPECT"
	// RestoreSelective pulls named containers or items back into the living tenant - the mode for
	// an accidentally deleted collection.
	RestoreSelective RestoreMode = "SELECTIVE"
	// RestoreMerge imports the archive into the living tenant, deciding each collision by the
	// conflict rule.
	RestoreMerge RestoreMode = "MERGE"
	// RestoreReplaceTenant resets a tenant to the archive. Destructive.
	RestoreReplaceTenant RestoreMode = "REPLACE_TENANT"
	// RestoreNewTenant imports the archive beside the living data as a tenant of its own. §8.2
	// names it the way to check before a destructive mode, which is why the API makes it the
	// cheap path rather than a documented discipline.
	RestoreNewTenant RestoreMode = "NEW_TENANT"
	// RestoreInstance restores a system backup under maintenance. Destructive, and the operator's
	// rather than a tenant's.
	RestoreInstance RestoreMode = "INSTANCE"
)

func (m RestoreMode) Valid() bool {
	switch m {
	case RestoreInspect, RestoreSelective, RestoreMerge,
		RestoreReplaceTenant, RestoreNewTenant, RestoreInstance:
		return true
	}
	return false
}

// Writes reports whether the mode may change anything at all. Only INSPECT may not.
func (m RestoreMode) Writes() bool { return m.Valid() && m != RestoreInspect }

// Destructive reports the modes §8.3 puts the typed tenant name and the step-up in front of, and
// the ones that get a safety copy first.
//
// REPLACE_TENANT and INSTANCE, and deliberately not MERGE with `overwrite`. An overwrite replaces
// the objects the archive names and leaves everything else; a replace removes what the archive
// does not name, which is the difference between losing an edit and losing a month.
func (m RestoreMode) Destructive() bool {
	return m == RestoreReplaceTenant || m == RestoreInstance
}

// ConflictRule is how a MERGE decides an object that is in the archive and in the tenant.
type ConflictRule string

const (
	// ConflictSkip leaves the living object alone. The default, because the living object is the
	// one somebody has been working in.
	ConflictSkip ConflictRule = "SKIP"
	// ConflictOverwrite replaces the living object with the archive's version.
	ConflictOverwrite ConflictRule = "OVERWRITE"
	// ConflictDuplicate imports the archive's version beside the living one, under a new
	// identity. See DuplicateID for why that identity is derived rather than drawn.
	ConflictDuplicate ConflictRule = "DUPLICATE"
)

func (r ConflictRule) Valid() bool {
	return r == ConflictSkip || r == ConflictOverwrite || r == ConflictDuplicate
}

// RestoreStatus is where a restore run stands. The values are the column's, which are the backup
// run's plus VALIDATING - a restore has a pre-check long enough to be worth watching (§8.3 step 1).
type RestoreStatus string

const (
	RestorePending    RestoreStatus = "PENDING"
	RestoreValidating RestoreStatus = "VALIDATING"
	RestoreRunning    RestoreStatus = "RUNNING"
	RestoreSucceeded  RestoreStatus = "SUCCEEDED"
	RestoreFailed     RestoreStatus = "FAILED"
	RestoreCancelled  RestoreStatus = "CANCELLED"
)

func (s RestoreStatus) Valid() bool {
	switch s {
	case RestorePending, RestoreValidating, RestoreRunning,
		RestoreSucceeded, RestoreFailed, RestoreCancelled:
		return true
	}
	return false
}

// Selection is what a SELECTIVE restore names: containers, items, or both.
//
// Identifiers rather than a query. A restore is judged against what somebody looked at in an
// INSPECT report, and a query re-evaluated at execution time could select something the report
// never showed.
type Selection struct {
	ContainerIDs []shared.ID
	ItemIDs      []shared.ID
}

// Empty reports a selection that names nothing.
func (s Selection) Empty() bool { return len(s.ContainerIDs)+len(s.ItemIDs) == 0 }

// maxSelection bounds what one SELECTIVE restore may name. Beyond this the caller wants MERGE,
// and an unbounded list is a request body that becomes a query with as many parameters as the
// sender feels like sending.
const maxSelection = 1000

// RestoreRequest is one restore as somebody asked for it.
//
// It is a value rather than a set of arguments because §8.3 is a checklist whose steps are about
// the request as a whole - a mode, and what that mode obliges the caller to have supplied - and a
// check spread over six parameters is a check that gets one of them wrong.
type RestoreRequest struct {
	TargetID shared.ID
	// SourceArchive is the archive's directory at the target, not a run identifier. §8.1 requires
	// a restore to work when the database is gone, and a run identifier only means something to a
	// database.
	SourceArchive string
	Mode          RestoreMode
	// TenantID is the tenant being restored into. Empty for NEW_TENANT, which mints one, and for
	// INSTANCE, which belongs to no tenant.
	TenantID     shared.ID
	ConflictRule ConflictRule
	Selection    Selection
	DryRun       bool
	// CreateSafetyBackup asks for the copy §8.3 step 4 takes before a destructive mode. It can be
	// declined, and declining it is recorded on the run rather than argued with: an operator
	// restoring onto a target that is full has to be able to say so.
	CreateSafetyBackup bool
	// Confirmation is the tenant name, typed. Compared against the name of the tenant being
	// replaced, exactly.
	Confirmation string
	// StepUpToken is the proof of a fresh, stronger authentication. Nothing can issue one yet;
	// see CodeStepUpUnavailable.
	StepUpToken string
}

// Validate reports what makes a request unanswerable, before anything is read at the target.
//
// It is deliberately about the request rather than about the world: whether the archive is there,
// whether the caller may have it, and whether this installation can satisfy a step-up are all
// questions for the application layer, which has the ports to ask them. What is here is what can
// be decided from the request alone, and deciding it here is what keeps the six modes from
// growing six copies of the same three checks.
func (r RestoreRequest) Validate() error {
	invalid := func(code, field string) error {
		return shared.ErrValidation.WithDetail(code).
			WithFields(shared.FieldError{Path: field, Code: code}).
			WithCause(errors.New(code))
	}
	switch {
	case r.TargetID.IsZero():
		return invalid(CodeRestoreTargetRequired, "/target_id")
	case strings.TrimSpace(r.SourceArchive) == "":
		return invalid(CodeRestoreArchiveRequired, "/archive_id")
	case !r.Mode.Valid():
		return invalid(CodeRestoreModeInvalid, "/mode")
	case r.ConflictRule != "" && !r.ConflictRule.Valid():
		return invalid(CodeRestoreConflictRuleInvalid, "/conflict_rule")
	case r.Mode == RestoreSelective && r.Selection.Empty():
		return invalid(CodeRestoreSelectionRequired, "/selection")
	case len(r.Selection.ContainerIDs)+len(r.Selection.ItemIDs) > maxSelection:
		return invalid(CodeRestoreSelectionTooLarge, "/selection")
	// A mode that writes into a living tenant has to name it. NEW_TENANT mints one and INSTANCE
	// belongs to none, so for those a named tenant is the mistake instead.
	case r.needsTenant() && r.TenantID.IsZero():
		return invalid(CodeRestoreTenantRequired, "/target_tenant_id")
	case !r.needsTenant() && !r.TenantID.IsZero():
		return invalid(CodeRestoreTenantUnexpected, "/target_tenant_id")
	}
	return nil
}

// needsTenant reports the modes that write into a tenant that already exists.
func (r RestoreRequest) needsTenant() bool {
	switch r.Mode {
	case RestoreInspect, RestoreSelective, RestoreMerge, RestoreReplaceTenant:
		return true
	}
	return false
}

// RuleOrDefault is the conflict rule to apply, SKIP unless another was named.
//
// SKIP rather than OVERWRITE, and the contract's default says the same: the living object is the
// one somebody has been working in, and a default that replaced it would make the safe reading of
// a partly specified request the destructive one.
func (r RestoreRequest) RuleOrDefault() ConflictRule {
	if r.ConflictRule == "" {
		return ConflictSkip
	}
	return r.ConflictRule
}

// ConfirmationMatches reports whether the typed name is the name of what is about to be replaced.
//
// Exact, including case and spacing. §8.3 asks for the tenant name to be typed, and the point of
// typing it is that it cannot be produced by acknowledging a dialogue - a comparison that forgave
// case would forgive a copy of the wrong tenant's name just as readily.
func (r RestoreRequest) ConfirmationMatches(name string) bool {
	return r.Confirmation != "" && r.Confirmation == name
}

// Report is what a restore did, or - on a dry run - what it would do (§8.3 step 2).
//
// One shape for both, so that the report a caller approved and the report they get back are
// comparable. A dry run that reported in a different vocabulary from the execution would make the
// approval a formality.
type Report struct {
	New         int
	Overwritten int
	Skipped     int
	Duplicated  int
	// Conflicts is how many collisions the rule had to decide. It is the sum of the other three
	// ways one can be decided, and it is counted separately because "how much of this is a
	// decision I made in advance" is the question a dry run exists to answer.
	Conflicts int
	// Withheld is what the restore deliberately did not bring back, by reason. The reasons are
	// the constants below, so that six code paths cannot spell one of them six ways.
	Withheld map[string]int
	Media    int
	// Entities is how many records each entity contributed, by the archive's entity name.
	Entities map[string]int
}

// The reasons an object is in the archive and not in the result.
const (
	// WithheldDeleted is the deletion journal doing its work: the object was deleted after the
	// archive was taken, and a restore that brought it back would undo an erasure (§7).
	WithheldDeleted = "deletion_journal"
	// WithheldExcluded is an entity an archive carries that a restore deliberately does not
	// import - the tokens and the outbox of §8.4.
	WithheldExcluded = "excluded_entity"
	// WithheldNotSelected is an object outside a SELECTIVE restore's selection.
	WithheldNotSelected = "not_selected"
	// WithheldMediaMissing is a row whose attachment is not in the archive. The row is restored
	// and the bytes are not: an object whose content is gone is the honest outcome, and refusing
	// a whole restore over one missing file would be the wrong trade on the day it is being used.
	WithheldMediaMissing = "media_missing"
)

// Count records one decision about one object: the rule that decided it, and whether there was
// anything to decide.
//
// An object the tenant does not hold is new, whatever the rule says, and it is counted as new
// rather than as an overwrite of nothing. That distinction is the report's whole value on a
// MERGE: "four hundred new, two overwritten" and "four hundred and two overwritten" describe the
// same writes and would be approved by different people.
func (r *Report) Count(outcome ConflictRule, collided bool) {
	if !collided {
		r.New++
		return
	}
	r.Conflicts++
	switch outcome {
	case ConflictSkip:
		r.Skipped++
	case ConflictOverwrite:
		r.Overwritten++
	case ConflictDuplicate:
		r.Duplicated++
	}
}

// Withhold records one object the restore did not bring back, and why.
func (r *Report) Withhold(reason string) {
	if r.Withheld == nil {
		r.Withheld = map[string]int{}
	}
	r.Withheld[reason]++
}

// Contributed records one record written for an entity.
func (r *Report) Contributed(entity string) {
	if r.Entities == nil {
		r.Entities = map[string]int{}
	}
	r.Entities[entity]++
}

// Deleted is how many objects the deletion journal kept out. A reading of Withheld rather than a
// counter of its own: two numbers that have to agree are two numbers that eventually do not.
func (r Report) Deleted() int { return r.Withheld[WithheldDeleted] }

// Restore is one restore run, as the table records it.
type Restore struct {
	ID            shared.ID
	TargetID      shared.ID
	TenantID      shared.ID
	SourceArchive string
	Mode          RestoreMode
	ConflictRule  ConflictRule
	Selection     Selection
	DryRun        bool
	// CreateSafetyBackup is whether §8.3 step 4's copy is wanted. It travels with the run rather
	// than with the request because the copy is taken minutes later, by the job.
	CreateSafetyBackup bool
	SafetyRunID        shared.ID
	Status             RestoreStatus
	Report             Report
	// Progress is how many of each entity's records have already been decided, by the archive's
	// entity name. It is what a resumed attempt skips: the archive is immutable and the read order
	// is fixed, so "the first N records of this entity" names the same N on every attempt (BK-7).
	Progress    map[string]int
	RequestedBy shared.ID
	ApprovedBy  shared.ID
	StartedAt   time.Time
	FinishedAt  time.Time
	ErrorCode   string
}

// Finished reports a run nothing more will happen to.
func (r Restore) Finished() bool {
	return r.Status == RestoreSucceeded || r.Status == RestoreFailed || r.Status == RestoreCancelled
}

// RestoreOutcome is how a restore ended, as the one statement that closes it takes it.
type RestoreOutcome struct {
	ID          shared.ID
	Status      RestoreStatus
	Report      Report
	SafetyRunID shared.ID
	FinishedAt  time.Time
	// ErrorCode is the message code of the failure, never a message and never anything the run
	// was working on (rules 8 and 10).
	ErrorCode string
}

// DuplicateID is the identity an object gets when the conflict rule is DUPLICATE.
//
// Derived from the run and the object rather than drawn from the identifier generator, and that is
// what makes BK-7's restore half true for this rule. A restore is resumable because everything it
// writes is keyed by identity: re-applying a record that is already there changes nothing. Drawing
// a fresh identifier for a duplicate would break exactly that - a worker that died after writing
// half the duplicates would, on resume, write the other half *and a second copy of the first half*,
// under identifiers nobody could tell from the first ones.
//
// So the identity is a function of (run, entity, original). The same resume produces the same
// identifiers; a second restore of the same archive produces different ones, because it is a
// different run and asking for the same import twice is asking for two copies.
//
// SHA-256 rather than UUIDv5's SHA-1: the length is the same after truncation and there is no
// reason to reach for a hash that is broken for its original purpose. The version nibble is 8 -
// RFC 9562's custom format, which is exactly what this is - and the variant bits are set, so that
// the value is a well-formed UUID everywhere it is stored, compared or logged.
func DuplicateID(runID shared.ID, entity, original string) shared.ID {
	sum := sha256.Sum256([]byte("hubtask/restore-duplicate\x00" +
		runID.String() + "\x00" + entity + "\x00" + original))

	sum[6] = sum[6]&0x0f | 0x80 // version 8
	sum[8] = sum[8]&0x3f | 0x80 // variant RFC 9562

	hexed := hex.EncodeToString(sum[:16])
	return shared.ID(hexed[0:8] + "-" + hexed[8:12] + "-" + hexed[12:16] + "-" +
		hexed[16:20] + "-" + hexed[20:32])
}

// The refusals of a restore, as codes rather than as prose.
const (
	CodeRestoreTargetRequired      = "backup.restore_target_required"
	CodeRestoreArchiveRequired     = "backup.restore_archive_required"
	CodeRestoreModeInvalid         = "backup.restore_mode_invalid"
	CodeRestoreConflictRuleInvalid = "backup.restore_conflict_rule_invalid"
	CodeRestoreSelectionRequired   = "backup.restore_selection_required"
	CodeRestoreSelectionTooLarge   = "backup.restore_selection_too_large"
	CodeRestoreTenantRequired      = "backup.restore_tenant_required"
	CodeRestoreTenantUnexpected    = "backup.restore_tenant_unexpected"
	CodeRestoreNotFound            = "backup.restore_not_found"
	CodeRestoreNotRunning          = "backup.restore_not_running"
	// CodeRestoreConfirmationRequired is a destructive mode without the tenant's name typed into
	// it, or with somebody else's name typed into it.
	CodeRestoreConfirmationRequired = "backup.restore_confirmation_required"
	// CodeStepUpRequired is a destructive mode without the stronger authentication §8.3 asks for.
	CodeStepUpRequired = "backup.restore_step_up_required"
	// CodeStepUpUnavailable is the honest answer of an installation that cannot satisfy a step-up
	// at all, because sessions and MFA arrive in 0.6.0. The destructive mode is refused rather
	// than permitted, and the code says which of the two refusals it is - "you did not prove it"
	// and "nothing here can prove it" are different problems with different fixes.
	CodeStepUpUnavailable = "backup.restore_step_up_unavailable"
	// CodeRestoreArchiveScopeMismatch is an archive of one tenant being restored into another,
	// which is BK-10's refusal. It is the one every mode is checked for, at the listing, at the
	// dry run and at the execution.
	CodeRestoreArchiveScopeMismatch = "backup.restore_archive_scope_mismatch"
	// CodeRestoreSchemaAhead is an archive from a newer schema than this build has migrated to.
	// A restore reads JSON Lines and migrates upwards; downwards it cannot go, and guessing which
	// columns a future migration added is how a restore writes a row that is silently wrong.
	CodeRestoreSchemaAhead = "backup.restore_schema_ahead"
	// CodeRestoreTargetBusy is a second restore into a tenant that is already having one.
	CodeRestoreTargetBusy = "backup.restore_in_progress"
	// CodeRestoreSafetyCopyUnavailable is a destructive mode asking for the copy of §8.3 step 4
	// on an installation that cannot take one. Refused rather than proceeding without it: a
	// destructive restore with no way back is the situation the step exists to prevent, and "there
	// was nowhere to write it" is a reason to stop rather than a reason to carry on.
	CodeRestoreSafetyCopyUnavailable = "backup.restore_safety_copy_unavailable"
	// CodeRestoreFailed is anything unclassified, so that a dashboard never shows a driver's
	// message.
	CodeRestoreFailed = "backup.restore_failed"
)
