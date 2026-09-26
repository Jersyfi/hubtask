// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

const (
	someTarget  = shared.ID("0198f0a0-0000-7000-8000-00000000a001")
	someTenant  = shared.ID("0198f0a0-0000-7000-8000-00000000b001")
	someRestore = shared.ID("0198f0a0-0000-7000-8000-00000000c001")
	somePrefix  = "hubtask-backup-0198f0a0-0000-7000-8000-00000000b001-20260101T030000Z-full"
)

// request is a valid MERGE into a living tenant, which every case below varies from.
func request(change func(*domain.RestoreRequest)) domain.RestoreRequest {
	out := domain.RestoreRequest{
		TargetID: someTarget, SourceArchive: somePrefix,
		Mode: domain.RestoreMerge, TenantID: someTenant, DryRun: true,
	}
	change(&out)
	return out
}

func detailOf(t *testing.T, err error) string {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	return domainErr.DetailCode
}

func TestAValidRequestIsAccepted(t *testing.T) {
	if err := request(func(*domain.RestoreRequest) {}).Validate(); err != nil {
		t.Fatalf("a valid request was refused: %v", err)
	}
}

func TestWhatARequestHasToCarry(t *testing.T) {
	cases := map[string]struct {
		change func(*domain.RestoreRequest)
		code   string
	}{
		"no target": {
			func(r *domain.RestoreRequest) { r.TargetID = "" },
			domain.CodeRestoreTargetRequired,
		},
		"no archive": {
			func(r *domain.RestoreRequest) { r.SourceArchive = "  " },
			domain.CodeRestoreArchiveRequired,
		},
		"a mode nobody defined": {
			func(r *domain.RestoreRequest) { r.Mode = "EVERYTHING" },
			domain.CodeRestoreModeInvalid,
		},
		"a conflict rule nobody defined": {
			func(r *domain.RestoreRequest) { r.ConflictRule = "MERGE_HARDER" },
			domain.CodeRestoreConflictRuleInvalid,
		},
		"a selective restore that selects nothing": {
			func(r *domain.RestoreRequest) { r.Mode = domain.RestoreSelective },
			domain.CodeRestoreSelectionRequired,
		},
		"a merge with no tenant to merge into": {
			func(r *domain.RestoreRequest) { r.TenantID = "" },
			domain.CodeRestoreTenantRequired,
		},
		"a new tenant that names a tenant": {
			func(r *domain.RestoreRequest) { r.Mode = domain.RestoreNewTenant },
			domain.CodeRestoreTenantUnexpected,
		},
		"an instance restore that names a tenant": {
			func(r *domain.RestoreRequest) { r.Mode = domain.RestoreInstance },
			domain.CodeRestoreTenantUnexpected,
		},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			err := request(test.change).Validate()
			if err == nil {
				t.Fatalf("the request was accepted")
			}
			if code := detailOf(t, err); code != test.code {
				t.Fatalf("refused with %s, want %s", code, test.code)
			}
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("refused with %v, want a validation error", err)
			}
		})
	}
}

func TestASelectionIsBounded(t *testing.T) {
	var many []shared.ID
	for range 1001 {
		many = append(many, someTenant)
	}

	err := request(func(r *domain.RestoreRequest) {
		r.Mode = domain.RestoreSelective
		r.Selection = domain.Selection{ItemIDs: many}
	}).Validate()

	if code := detailOf(t, err); code != domain.CodeRestoreSelectionTooLarge {
		t.Fatalf("refused with %s, want %s", code, domain.CodeRestoreSelectionTooLarge)
	}
}

func TestOnlyReplaceAndInstanceAreDestructive(t *testing.T) {
	for mode, destructive := range map[domain.RestoreMode]bool{
		domain.RestoreInspect:       false,
		domain.RestoreSelective:     false,
		domain.RestoreMerge:         false,
		domain.RestoreNewTenant:     false,
		domain.RestoreReplaceTenant: true,
		domain.RestoreInstance:      true,
	} {
		if mode.Destructive() != destructive {
			t.Errorf("%s: destructive is %v, want %v", mode, mode.Destructive(), destructive)
		}
	}
}

func TestOnlyInspectWritesNothing(t *testing.T) {
	if domain.RestoreInspect.Writes() {
		t.Error("INSPECT writes")
	}
	for _, mode := range []domain.RestoreMode{
		domain.RestoreSelective, domain.RestoreMerge,
		domain.RestoreReplaceTenant, domain.RestoreNewTenant, domain.RestoreInstance,
	} {
		if !mode.Writes() {
			t.Errorf("%s writes nothing", mode)
		}
	}
	if domain.RestoreMode("EVERYTHING").Writes() {
		t.Error("a mode nobody defined writes")
	}
}

func TestTheDefaultRuleIsToLeaveTheLivingObjectAlone(t *testing.T) {
	if rule := request(func(*domain.RestoreRequest) {}).RuleOrDefault(); rule != domain.ConflictSkip {
		t.Fatalf("the default rule is %s, want SKIP", rule)
	}
	named := request(func(r *domain.RestoreRequest) { r.ConflictRule = domain.ConflictOverwrite })
	if rule := named.RuleOrDefault(); rule != domain.ConflictOverwrite {
		t.Fatalf("a named rule became %s", rule)
	}
}

func TestTheConfirmationIsComparedExactly(t *testing.T) {
	asked := request(func(r *domain.RestoreRequest) { r.Confirmation = "Acme GmbH" })

	if !asked.ConfirmationMatches("Acme GmbH") {
		t.Error("the exact name did not match")
	}
	for _, near := range []string{"acme gmbh", "Acme GmbH ", "Acme", ""} {
		if asked.ConfirmationMatches(near) {
			t.Errorf("%q matched", near)
		}
	}
	// An empty confirmation matches nothing, including an unnamed tenant.
	if request(func(*domain.RestoreRequest) {}).ConfirmationMatches("") {
		t.Error("no confirmation matched no name")
	}
}

func TestADuplicateIdentityIsTheSameOnEveryAttempt(t *testing.T) {
	first := domain.DuplicateID(someRestore, "work_items", "a-title")
	again := domain.DuplicateID(someRestore, "work_items", "a-title")

	if first != again {
		t.Fatalf("%s and %s differ, so a resumed restore would duplicate twice", first, again)
	}
	if _, err := shared.ParseID(first.String()); err != nil {
		t.Fatalf("%s is not a well-formed identifier: %v", first, err)
	}
	if first[14] != '8' {
		t.Errorf("%s does not say it is a derived identifier", first)
	}
	if variant := first[19]; variant != '8' && variant != '9' && variant != 'a' && variant != 'b' {
		t.Errorf("%s has variant nibble %q, which is not RFC 9562's", first, variant)
	}
}

func TestADuplicateIdentityDiffersPerRunAndPerObject(t *testing.T) {
	other := shared.ID("0198f0a0-0000-7000-8000-00000000c002")

	distinct := map[shared.ID]bool{
		domain.DuplicateID(someRestore, "work_items", "one"): true,
		domain.DuplicateID(someRestore, "work_items", "two"): true,
		domain.DuplicateID(someRestore, "containers", "one"): true,
		domain.DuplicateID(other, "work_items", "one"):       true,
	}
	if len(distinct) != 4 {
		t.Fatalf("%d distinct identifiers out of four inputs", len(distinct))
	}
}

// The six characters someRestore hashes to, pinned rather than recomputed: a test that derived
// them the way the code does would agree with the code whatever the code said.
const runDigits = "17f937"

func TestADuplicatedNameIsTheSameOnEveryAttempt(t *testing.T) {
	at := time.Date(2026, 9, 24, 11, 30, 0, 0, time.UTC)

	first := domain.DuplicatedName(someRestore, at, "Errands")
	again := domain.DuplicatedName(someRestore, at, "Errands")

	if first != again {
		t.Fatalf("%q and %q differ, so a resumed restore would name its copy twice", first, again)
	}
	if want := "Errands (restored 2026-09-24 " + runDigits + ")"; first != want {
		t.Errorf("the copy is called %q, want %q", first, want)
	}
}

// The reason this run's characters are in the name at all: the same archive restored twice into
// the same workspace makes two copies, and two copies cannot share one name.
func TestASecondRestoreOfTheSameArchiveNamesItsCopyDifferently(t *testing.T) {
	at := time.Date(2026, 9, 24, 11, 30, 0, 0, time.UTC)
	other := shared.ID("0198f0a0-0000-7000-8000-00000000c002")

	if first, second := domain.DuplicatedName(someRestore, at, "Errands"),
		domain.DuplicatedName(other, at, "Errands"); first == second {
		t.Fatalf("both runs call their copy %q", first)
	}
}

// The clock the name is read off is the run's own, not the reader's: a restore that began
// yesterday evening in Berlin names its copy after the day the run started, in UTC, on every
// attempt that resumes it.
func TestADuplicatedNameIsReadOffTheRunsOwnClock(t *testing.T) {
	berlin := time.FixedZone("CEST", 2*60*60)
	evening := time.Date(2026, 9, 24, 1, 30, 0, 0, berlin) // 2026-09-23 23:30 UTC

	if name := domain.DuplicatedName(someRestore, evening, "Errands"); !strings.Contains(name, "2026-09-23") {
		t.Errorf("%q does not carry the run's own date", name)
	}
}

func TestADuplicatedNameStaysInsideTheColumn(t *testing.T) {
	at := time.Date(2026, 9, 24, 11, 30, 0, 0, time.UTC)

	for _, original := range []string{
		strings.Repeat("a", domain.MaxDuplicatedName),    // exactly the bound
		strings.Repeat("a", domain.MaxDuplicatedName+50), // longer than the column ever held
		strings.Repeat("ä", domain.MaxDuplicatedName),    // the bound in characters, not bytes
		"Errands",
	} {
		name := domain.DuplicatedName(someRestore, at, original)
		if length := len([]rune(name)); length > domain.MaxDuplicatedName {
			t.Errorf("a name of %d characters came back as %d, past the column's bound",
				len([]rune(original)), length)
		}
		if !strings.HasSuffix(name, runDigits+")") {
			t.Errorf("%q lost the suffix that makes it unique", name)
		}
	}
}

func TestTheReportCountsEachDecisionOnce(t *testing.T) {
	var report domain.Report

	report.Count(domain.ConflictSkip, false)     // a new object: nothing to decide
	report.Count(domain.ConflictSkip, true)      // a collision left alone
	report.Count(domain.ConflictOverwrite, true) // a collision replaced
	report.Count(domain.ConflictDuplicate, true) // a collision imported beside
	report.Withhold(domain.WithheldDeleted)
	report.Withhold(domain.WithheldDeleted)
	report.Withhold(domain.WithheldExcluded)
	report.Contributed("work_items")

	if report.Conflicts != 3 {
		t.Errorf("conflicts is %d, want 3", report.Conflicts)
	}
	if report.New != 1 {
		t.Errorf("new is %d, want 1", report.New)
	}
	if report.Skipped != 1 || report.Overwritten != 1 || report.Duplicated != 1 {
		t.Errorf("skipped %d, overwritten %d, duplicated %d",
			report.Skipped, report.Overwritten, report.Duplicated)
	}
	if report.Deleted() != 2 {
		t.Errorf("the deletion journal kept out %d, want 2", report.Deleted())
	}
	if report.Withheld[domain.WithheldExcluded] != 1 {
		t.Errorf("the excluded entities were not counted")
	}
	if report.Entities["work_items"] != 1 {
		t.Errorf("the entity contribution was not counted")
	}
}
