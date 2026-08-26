// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup_test

import (
	"errors"
	"testing"

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
