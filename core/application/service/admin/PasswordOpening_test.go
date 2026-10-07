// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// ADR-0078 §3 (SC-34): an operator opens the password for one named workspace - behind the scope, the
// operator register and a step-up - for 24 hours unless said otherwise and never more than a week,
// with who asked and why recorded in the workspace's trail and the installation's journal; and can
// close it early.

type openingFixture struct {
	writer  PasswordOpeningWriter
	tenants *tenantsStore
	journal *journalStore
	audit   *auditSink
	stepUp  *stepUpFake
	work    *unitOfWork
	jobs    *jobsFake
	notices *noticesFake
}

// noticesFake records what the administrators were to be told.
type noticesFake struct{ told []string }

func (n *noticesFake) PasswordOpened(_ context.Context, tenantID shared.ID, until time.Time) error {
	n.told = append(n.told, "opened "+tenantID.String()+" "+until.Format(time.RFC3339))
	return nil
}

func (n *noticesFake) PasswordClosed(_ context.Context, tenantID shared.ID, ended string) error {
	n.told = append(n.told, "closed "+tenantID.String()+" "+ended)
	return nil
}

const openingProof = "hbt_sup_opening"

func newOpeningFixture(register *registerStore) *openingFixture {
	instance, _, _ := newInstanceWriter(register)
	f := &openingFixture{
		tenants: &tenantsStore{record: adminrepo.TenantRecord{
			ID: lifecycleTenant, Slug: "acme", DisplayName: "Acme GmbH", Status: domain.TenantActive,
		}},
		journal: &journalStore{}, audit: &auditSink{},
		stepUp: &stepUpFake{expect: openingProof}, work: &unitOfWork{},
		jobs: &jobsFake{}, notices: &noticesFake{},
	}
	f.writer = PasswordOpeningWriter{
		Instance: instance, Tenants: f.tenants, Journal: f.journal, Audit: f.audit,
		Jobs: f.jobs, Notices: f.notices,
		StepUp: f.stepUp, UnitOfWork: f.work, Clock: clock.Fixed(now), IDs: &ids{},
		Text: text.Composing{},
	}
	return f
}

func (f *openingFixture) open(
	t *testing.T, actor appshared.ActorContext, hours int, proof string,
) (adminrepo.TenantRecord, error) {
	t.Helper()
	return OpenTenantPassword{Writer: f.writer}.Execute(t.Context(), actor, OpenTenantPasswordCommand{
		TenantID: lifecycleTenant, Hours: hours, Requester: "TICKET-4711",
		Reason: "the directory answers 500", StepUpToken: proof,
	})
}

func TestAnOperatorOpensThePasswordForOneWorkspace(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))

	opened, err := f.open(t, operator(), domain.PasswordOpeningDefaultHours, openingProof)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	until := now.Add(24 * time.Hour)
	if !opened.PasswordOpening.Until.Equal(until) || len(f.tenants.openings) != 1 {
		t.Fatalf("the opening ends %v (%d written), want %v", opened.PasswordOpening.Until, len(f.tenants.openings), until)
	}
	if f.stepUp.consumed != 1 {
		t.Errorf("the step-up was consumed %d times, want once", f.stepUp.consumed)
	}
	// In the workspace's own transaction, bound to the operator as its actor.
	if scope := f.work.scopes[len(f.work.scopes)-1]; scope.TenantID != lifecycleTenant || scope.ActorID != operatorID {
		t.Errorf("written under %+v", scope)
	}

	// The workspace's trail: who opened it, until when, for whom and why.
	if len(f.audit.entries) != 1 {
		t.Fatalf("%d trail entries, want one", len(f.audit.entries))
	}
	entry := f.audit.entries[0]
	if entry.Action != TenantPasswordOpenedAction || entry.TenantID != lifecycleTenant ||
		entry.ActorID != operatorID || entry.Severity != audit.SeverityWarning {
		t.Errorf("trail entry %+v", entry)
	}
	for field, want := range map[string]string{
		"password_opened_until": until.Format(time.RFC3339), "requester": "TICKET-4711",
		"reason": "the directory answers 500",
	} {
		if got := changedTo(entry, field); got != want {
			t.Errorf("the trail says %s %v, want %q", field, got, want)
		}
	}

	// The installation's journal: the same act and its end, and that who asked and why were given -
	// never the texts, which go with the workspace's trail.
	if len(f.journal.entries) != 1 {
		t.Fatalf("%d journal entries, want one", len(f.journal.entries))
	}
	recorded := f.journal.entries[0]
	if recorded.Action != journalPasswordOpened || recorded.TenantID != lifecycleTenant ||
		recorded.TenantSlug != "acme" || recorded.ActorLabel != "Root Operator" ||
		recorded.Details["until"] != until.Format(time.RFC3339) ||
		recorded.Details["requester_present"] != true || recorded.Details["reason_present"] != true {
		t.Errorf("journal entry %+v", recorded)
	}
	for _, value := range recorded.Details {
		if value == "TICKET-4711" || value == "the directory answers 500" {
			t.Errorf("the permanent journal holds the text %q", value)
		}
	}
}

func TestAnOpeningLastsAtMostAWeek(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))

	opened, err := f.open(t, operator(), domain.PasswordOpeningMaximumHours, openingProof)
	if err != nil {
		t.Fatalf("a week was refused: %v", err)
	}
	if want := now.Add(7 * 24 * time.Hour); !opened.PasswordOpening.Until.Equal(want) {
		t.Errorf("a week ends %v, want %v", opened.PasswordOpening.Until, want)
	}

	f = newOpeningFixture(newRegister(operatorID))
	_, err = f.open(t, operator(), domain.PasswordOpeningMaximumHours+1, openingProof)
	if shared.AsError(err).DetailCode != "admin.password_opening_hours" {
		t.Fatalf("a week and an hour answered %v", err)
	}
	// Refused before the proof: a typo must not burn a step-up.
	if f.stepUp.consumed != 0 || len(f.tenants.openings) != 0 || len(f.audit.entries) != 0 {
		t.Error("a refused duration consumed the proof or wrote something")
	}
}

func TestOpeningThePasswordDemandsTheScopeTheRegisterAndAStepUp(t *testing.T) {
	cases := []struct {
		name     string
		register *registerStore
		actor    appshared.ActorContext
		proof    string
		code     string
	}{
		{"without the scope", newRegister(operatorID), func() appshared.ActorContext {
			actor := operator()
			actor.Scopes = nil
			return actor
		}(), openingProof, ""},
		{"outside the register", newRegister(secondOperator), operator(), openingProof, "admin.operator_required"},
		{"without a step-up", newRegister(operatorID), operator(), "", "auth.step_up_required"},
		{"with a stale step-up", newRegister(operatorID), operator(), "hbt_sup_other", "auth.step_up_required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOpeningFixture(c.register)
			_, err := f.open(t, c.actor, 24, c.proof)
			if !errors.Is(err, shared.ErrForbidden) {
				t.Fatalf("answer %v, want forbidden", err)
			}
			if c.code != "" && shared.AsError(err).DetailCode != c.code {
				t.Errorf("answer %v, want %s", err, c.code)
			}
			if len(f.tenants.openings) != 0 || len(f.audit.entries) != 0 || len(f.journal.entries) != 0 {
				t.Error("a refused opening wrote something")
			}
		})
	}
}

func TestAWorkspaceThatIsLeavingIsNotOpened(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.Status = domain.TenantPendingDeletion

	if _, err := f.open(t, operator(), 24, openingProof); shared.AsError(err).DetailCode != "admin.tenant_leaving" {
		t.Fatalf("a leaving workspace answered %v", err)
	}
	if len(f.audit.entries) != 0 || len(f.journal.entries) != 0 {
		t.Error("a refused opening was recorded")
	}

	f = newOpeningFixture(newRegister(operatorID))
	f.tenants.findErr = shared.ErrNotFound
	if _, err := f.open(t, operator(), 24, openingProof); shared.AsError(err).DetailCode != "admin.tenant_not_found" {
		t.Errorf("a workspace that is not there answered %v", err)
	}
}

func TestAnOperatorClosesAnOpeningEarly(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	if _, err := f.open(t, operator(), 24, openingProof); err != nil {
		t.Fatalf("opening: %v", err)
	}

	closed, err := CloseTenantPassword{Writer: f.writer}.Execute(t.Context(), operator(), lifecycleTenant)
	if err != nil {
		t.Fatalf("closing: %v", err)
	}
	if !closed.PasswordOpening.Until.IsZero() || !f.tenants.record.PasswordOpening.Until.IsZero() {
		t.Errorf("the opening outlived its close: %+v", closed.PasswordOpening)
	}
	// Any opening, not only a due one.
	if len(f.tenants.closes) != 1 || !f.tenants.closes[0].IsZero() {
		t.Errorf("closed with due %v", f.tenants.closes)
	}
	entry := f.audit.entries[len(f.audit.entries)-1]
	if entry.Action != TenantPasswordClosedAction || entry.ActorID != operatorID ||
		changedTo(entry, "ended") != OpeningEndedByOperator {
		t.Errorf("trail entry %+v", entry)
	}
	recorded := f.journal.entries[len(f.journal.entries)-1]
	if recorded.Action != journalPasswordClosed || recorded.Details["ended"] != OpeningEndedByOperator {
		t.Errorf("journal entry %+v", recorded)
	}

	// Closing again: the state the operator wanted, and nothing recorded twice.
	entries := len(f.audit.entries)
	if _, err := (CloseTenantPassword{Writer: f.writer}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("closing again: %v", err)
	}
	if len(f.audit.entries) != entries {
		t.Error("closing what was closed recorded it again")
	}
}

func TestClosingDemandsTheScopeAndTheRegisterButNoStepUp(t *testing.T) {
	f := newOpeningFixture(newRegister(secondOperator))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{Until: now.Add(time.Hour), Requester: "r", Reason: "r"}
	if _, err := (CloseTenantPassword{Writer: f.writer}).Execute(t.Context(), operator(), lifecycleTenant); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("an operator outside the register closed it: %v", err)
	}

	f = newOpeningFixture(newRegister(operatorID))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{Until: now.Add(time.Hour), Requester: "r", Reason: "r"}
	if _, err := (CloseTenantPassword{Writer: f.writer}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Errorf("closing asked for more than the scope and the register: %v", err)
	}
	if f.stepUp.consumed != 0 {
		t.Error("closing consumed a step-up")
	}
}

// The registry round trip: the input a client sends meets the descriptors' own validation, and the
// default day applies where the client named none.
func TestTheOpeningIsReachableThroughTheRegistry(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	registry, err := usecase.NewRegistry(nil,
		OpenTenantPassword{Writer: f.writer}.Descriptor(), CloseTenantPassword{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registry: %v", err)
	}

	out, err := registry.Invoke(t.Context(), OpenTenantPasswordName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(), "requester": "TICKET-4711",
		"reason": "the directory answers 500", "step_up_token": openingProof,
	})
	if err != nil {
		t.Fatalf("opening through the registry: %v", err)
	}
	opening, _ := out["password_opening"].(usecase.Output)
	if until, _ := opening["until"].(time.Time); !until.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("the default opening answered %+v", out)
	}

	if _, err := registry.Invoke(t.Context(), OpenTenantPasswordName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(), "hours": 0, "requester": "TICKET-4711",
		"reason": "down", "step_up_token": openingProof,
	}); shared.AsError(err).DetailCode != "admin.password_opening_hours" {
		t.Errorf("hours sent as zero answered %v, want the refusal rather than the default", err)
	}

	out, err = registry.Invoke(t.Context(), CloseTenantPasswordName, operator(), usecase.Input{
		"tenant_id": lifecycleTenant.String(),
	})
	if err != nil {
		t.Fatalf("closing through the registry: %v", err)
	}
	if _, held := out["password_opening"]; held {
		t.Errorf("a closed opening is still answered: %+v", out)
	}
}

// changedTo reads the value a trail entry's change carries.
func changedTo(entry audit.Entry, field string) any {
	change, _ := entry.Changes[field].(map[string]any)
	return change["to"]
}

// The administrators are told when it opens, with its end, and when it closes, with how; and the
// opening's own write seeds the job that records its end - per workspace, at that end.
func TestAnOpeningSeedsItsEndAndTellsTheAdministrators(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	opened, err := f.open(t, operator(), 24, openingProof)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	until := opened.PasswordOpening.Until
	if len(f.jobs.requests) != 1 {
		t.Fatalf("%d jobs, want the one that records the end", len(f.jobs.requests))
	}
	job := f.jobs.requests[0]
	if job.Kind != queue.KindPasswordOpeningEnd || job.TenantID != lifecycleTenant ||
		job.DedupeKey != lifecycleTenant.String() || !job.RunAt.Equal(until) {
		t.Errorf("the end job is %+v", job)
	}
	if want := "opened " + lifecycleTenant.String() + " " + until.Format(time.RFC3339); len(f.notices.told) != 1 || f.notices.told[0] != want {
		t.Errorf("the administrators were told %v, want %q", f.notices.told, want)
	}

	if _, err := (CloseTenantPassword{Writer: f.writer}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if want := "closed " + lifecycleTenant.String() + " " + OpeningEndedByOperator; len(f.notices.told) != 2 || f.notices.told[1] != want {
		t.Errorf("the administrators were told %v, want %q last", f.notices.told, want)
	}
}

func TestTheEndOfAnOpeningIsRecordedWhenItsTimeHasPassed(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{
		Until: now.Add(-time.Minute), Requester: "TICKET-4711", Reason: "down",
	}

	again, err := EndPasswordOpening{Writer: f.writer}.Execute(t.Context(), lifecycleTenant)
	if err != nil || again != 0 {
		t.Fatalf("ending answered (%v, %v)", again, err)
	}
	if !f.tenants.record.PasswordOpening.Until.IsZero() {
		t.Error("the row still holds the opening")
	}
	// Due by now, never "any": an opening that runs on must survive the close.
	if len(f.tenants.closes) != 1 || !f.tenants.closes[0].Equal(now) {
		t.Errorf("closed with due %v, want the moment it ran", f.tenants.closes)
	}
	entry := f.audit.entries[0]
	if entry.Action != TenantPasswordClosedAction || entry.ActorKind != appshared.ActorSystem ||
		changedTo(entry, "ended") != OpeningEndedExpired {
		t.Errorf("trail entry %+v", entry)
	}
	if recorded := f.journal.entries[0]; recorded.Action != journalPasswordClosed ||
		recorded.Details["ended"] != OpeningEndedExpired {
		t.Errorf("journal entry %+v", recorded)
	}
	if want := "closed " + lifecycleTenant.String() + " " + OpeningEndedExpired; len(f.notices.told) != 1 || f.notices.told[0] != want {
		t.Errorf("the administrators were told %v", f.notices.told)
	}
}

func TestTheEndJobComesBackForAnOpeningStillRunningAndLeavesAClosedOne(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{
		Until: now.Add(3 * time.Hour), Requester: "TICKET-4712", Reason: "still down",
	}
	again, err := EndPasswordOpening{Writer: f.writer}.Execute(t.Context(), lifecycleTenant)
	if err != nil || again != 3*time.Hour {
		t.Fatalf("a running opening answered (%v, %v), want to come back in three hours", again, err)
	}
	if len(f.audit.entries) != 0 || len(f.notices.told) != 0 || f.tenants.record.PasswordOpening.Until.IsZero() {
		t.Error("a running opening was ended")
	}

	// Closed early, the close was recorded then: nothing is left for the job.
	f.tenants.record.PasswordOpening = domain.PasswordOpening{}
	if again, err := (EndPasswordOpening{Writer: f.writer}).Execute(t.Context(), lifecycleTenant); err != nil || again != 0 {
		t.Errorf("a closed opening answered (%v, %v)", again, err)
	}
	// The workspace went while the job waited.
	f.tenants.findErr = shared.ErrNotFound
	if again, err := (EndPasswordOpening{Writer: f.writer}).Execute(t.Context(), lifecycleTenant); err != nil || again != 0 {
		t.Errorf("a workspace that went answered (%v, %v)", again, err)
	}
	if len(f.audit.entries) != 0 || len(f.notices.told) != 0 {
		t.Error("nothing to end recorded something")
	}
}

// An opening written between the end job's read and its close: the close finds nothing due, and the
// job comes back at the new opening's end instead of finishing - that opening's own job collapsed into
// this running one, and without it nobody would record its end or tell the administrators.
func TestTheEndJobComesBackForAnOpeningMadeWhileItRan(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{
		Until: now.Add(-time.Minute), Requester: "TICKET-4711", Reason: "down",
	}
	f.tenants.racing = &domain.PasswordOpening{
		Until: now.Add(5 * time.Hour), Requester: "TICKET-4714", Reason: "down again",
	}

	again, err := EndPasswordOpening{Writer: f.writer}.Execute(t.Context(), lifecycleTenant)
	if err != nil {
		t.Fatalf("ending: %v", err)
	}
	if again != 5*time.Hour {
		t.Errorf("the job answered %v, want to come back at the new opening's end", again)
	}
	if len(f.audit.entries) != 0 || len(f.journal.entries) != 0 || len(f.notices.told) != 0 {
		t.Error("an opening that still runs was recorded as ended")
	}
}

// Closing an opening whose time has passed but whose end the job has not recorded yet: it ended at
// its time, and that is what the trail, the journal and the mail say - not an early close.
func TestClosingAnOpeningPastItsEndRecordsItAsExpired(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.PasswordOpening = domain.PasswordOpening{
		Until: now.Add(-time.Hour), Requester: "TICKET-4711", Reason: "down",
	}

	if _, err := (CloseTenantPassword{Writer: f.writer}).Execute(t.Context(), operator(), lifecycleTenant); err != nil {
		t.Fatalf("closing: %v", err)
	}
	if entry := f.audit.entries[0]; changedTo(entry, "ended") != OpeningEndedExpired {
		t.Errorf("trail entry %+v", entry)
	}
	if recorded := f.journal.entries[0]; recorded.Details["ended"] != OpeningEndedExpired {
		t.Errorf("journal entry %+v", recorded)
	}
	if want := "closed " + lifecycleTenant.String() + " " + OpeningEndedExpired; len(f.notices.told) != 1 || f.notices.told[0] != want {
		t.Errorf("the administrators were told %v", f.notices.told)
	}
}

// A suspended workspace refuses its people before any password is asked for: an opening there would
// let nobody in, while its administrators were mailed that everybody can sign in. Refused, with the
// way forward in the message, and nothing recorded, mailed or seeded.
func TestASuspendedWorkspaceIsNotOpened(t *testing.T) {
	f := newOpeningFixture(newRegister(operatorID))
	f.tenants.record.Status = domain.TenantSuspended

	_, err := f.open(t, operator(), 24, openingProof)
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "admin.password_opening_suspended" {
		t.Fatalf("a suspended workspace answered %v", err)
	}
	if len(f.tenants.openings) != 0 || len(f.audit.entries) != 0 || len(f.journal.entries) != 0 ||
		len(f.jobs.requests) != 0 || len(f.notices.told) != 0 {
		t.Error("a refused opening wrote, recorded, seeded or told something")
	}
}
