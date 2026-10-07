// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// workspaceStore is the tenant's own row, in memory. `updates` records what the use case asked
// for, because the version it guards on is half of what this task had to get right.
type workspaceStore struct {
	row     domain.Workspace
	missing bool
	updates []workspaceWrite
	// refuse makes the guarded write fail the way a concurrent commit makes it fail.
	refuse bool
}

type workspaceWrite struct {
	changed  domain.Workspace
	expected int
	at       time.Time
}

func (s *workspaceStore) Find(context.Context) (domain.Workspace, error) {
	if s.missing {
		return domain.Workspace{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return s.row, nil
}

func (s *workspaceStore) Update(
	_ context.Context, changed domain.Workspace, expectedVersion int, now time.Time,
) (bool, error) {
	s.updates = append(s.updates, workspaceWrite{changed: changed, expected: expectedVersion, at: now})
	if s.refuse || expectedVersion != s.row.Version {
		return false, nil
	}
	s.row = changed
	s.row.Version = expectedVersion + 1
	s.row.UpdatedAt = now
	return true, nil
}

type workspaceFixture struct {
	writer WorkspaceWriter
	store  *workspaceStore
	auth   *authorizer
	audit  *auditSink
}

func newWorkspaceFixture(at time.Time) *workspaceFixture {
	f := &workspaceFixture{
		store: &workspaceStore{row: domain.Workspace{
			Tenant: domain.Tenant{
				ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
				DefaultLocale: "en", DefaultTimeZone: "UTC",
			},
			Version: 4,
		}},
		auth:  &authorizer{},
		audit: &auditSink{},
	}
	f.writer = WorkspaceWriter{
		Workspaces: f.store, Authorizer: f.auth, Audit: f.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at),
	}
	return f
}

func workspaceActor() appshared.ActorContext {
	actor := adminActor()
	actor.Scopes = append(actor.Scopes, workspaceRead, workspaceManage)
	return actor
}

func at() time.Time { return time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC) }

// The read is open to the auditor's read-only configuration permission as well as to READ - the
// pair G-12 split, and the reason an auditor can see how a workspace is set up without holding
// the right to change it.
func TestReadingTheWorkspaceAcceptsTheAuditorsPermissionToo(t *testing.T) {
	f := newWorkspaceFixture(at())

	if _, err := (ReadWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor()); err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(f.auth.requests) != 1 {
		t.Fatalf("%d authorisation requests, want one", len(f.auth.requests))
	}
	request := f.auth.requests[0]
	if request.Permission != service.PermissionRead {
		t.Errorf("asked for %q, want READ", request.Permission)
	}
	if request.Alternative != service.PermissionReadConfiguration {
		t.Errorf("the alternative is %q, want READ_CONFIGURATION", request.Alternative)
	}
	if request.TokenScope != workspaceRead {
		t.Errorf("token scope %q", request.TokenScope)
	}
}

// The write asks for STRUCTURE and offers no alternative: reading how a workspace is set up and
// changing it are not the same right.
func TestChangingTheWorkspaceNeedsTheStructurePermissionAlone(t *testing.T) {
	f := newWorkspaceFixture(at())
	name := "Acme GmbH"

	if _, err := (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{Change: domain.WorkspaceChange{DisplayName: &name}}); err != nil {
		t.Fatalf("changing: %v", err)
	}
	request := f.auth.requests[0]
	if request.Permission != service.PermissionStructure {
		t.Errorf("asked for %q, want STRUCTURE", request.Permission)
	}
	if request.Alternative != "" {
		t.Errorf("the write offers %q as an alternative", request.Alternative)
	}
}

// The enforcement switch H-02 has read since 0.6.0 is the rule's switch under its old name: setting
// it changes `mfa_required_for`, behind the proof, and the trail says so with the rule's own field -
// which is what "who turned enforcement off" needs (UC-ID-12 check 2).
func TestTheEnforcementSwitchIsSetAndRecorded(t *testing.T) {
	f := newWorkspacePolicyFixture(at())
	wanted := true

	changed, err := (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{RequireAdminTotp: &wanted, StepUpToken: "hbt_stp_x"})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	if rule := changed.Settings.SignIn.MfaRequiredFor; rule == nil || *rule != domain.MfaForAdmins {
		t.Errorf("the answer carries the rule %v, want ADMINS", rule)
	}
	if changed.Version != 5 {
		t.Errorf("version %d, want 5", changed.Version)
	}

	if len(f.audit.entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(f.audit.entries))
	}
	entry := f.audit.entries[0]
	if entry.Action != WorkspaceChangedAction {
		t.Errorf("action %q", entry.Action)
	}
	if entry.TargetID != tenant {
		t.Errorf("the entry names %v rather than the workspace", entry.TargetID)
	}
	change, held := entry.Changes["mfa_required_for"].(map[string]any)
	if !held {
		t.Fatalf("the entry records %v rather than the rule", entry.Changes)
	}
	// Open rather than masked: the switch is configuration, and an entry that hid it could not
	// answer "who turned enforcement off".
	if change["from"] != "NONE" || change["to"] != "ADMINS" {
		t.Errorf("the change is %v", change)
	}
}

// A patch that moves nothing writes nothing. A client that sends the whole form back on every
// save must not fill the trail with entries saying nothing happened.
func TestAPatchThatMovesNothingWritesNothing(t *testing.T) {
	f := newWorkspaceFixture(at())
	same := "Acme"

	answer, err := (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{Change: domain.WorkspaceChange{DisplayName: &same}})
	if err != nil {
		t.Fatalf("changing: %v", err)
	}
	if len(f.store.updates) != 0 {
		t.Errorf("%d writes for a patch that moved nothing", len(f.store.updates))
	}
	if len(f.audit.entries) != 0 {
		t.Errorf("%d audit entries for a patch that moved nothing", len(f.audit.entries))
	}
	if answer.Version != 4 {
		t.Errorf("version %d, want the stored 4", answer.Version)
	}
}

// A version the caller named through If-Match is what the write guards on, and a stale one is a
// conflict rather than a silent overwrite (ADR-0025).
func TestAStaleVersionIsAConflict(t *testing.T) {
	f := newWorkspaceFixture(at())
	name := "Acme GmbH"

	_, err := (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			Change: domain.WorkspaceChange{DisplayName: &name}, ExpectedVersion: 3,
		})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != "workspace.version_conflict" {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	if len(f.store.updates) != 1 || f.store.updates[0].expected != 3 {
		t.Errorf("the write did not guard on the version the caller named: %+v", f.store.updates)
	}
	if f.store.row.DisplayName != "Acme" {
		t.Error("the row moved despite the conflict")
	}
	if len(f.audit.entries) != 0 {
		t.Error("a refused write was recorded as one that happened")
	}
}

// A value the workspace cannot hold is refused before anything is written, and the field error
// the domain produces travels out unchanged.
func TestAnInvalidValueIsRefusedBeforeAnyWrite(t *testing.T) {
	f := newWorkspaceFixture(at())
	zone := "Mars/Olympus"

	_, err := (UpdateWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{Change: domain.WorkspaceChange{DefaultTimeZone: &zone}})

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("error %v is not a domain error", err)
	}
	if domainErr.DetailCode != "admin.time_zone_invalid" {
		t.Errorf("detail code %q", domainErr.DetailCode)
	}
	if len(f.store.updates) != 0 || len(f.audit.entries) != 0 {
		t.Error("something was written for a refused value")
	}
}

// The read shape is what both use cases answer, and it carries no key the contract does not
// declare - the structural half of "the answer after a write is the answer a read gives".
func TestTheAnswerCarriesWhatTheContractDeclares(t *testing.T) {
	f := newWorkspaceFixture(at())

	read, err := (ReadWorkspace{Writer: f.writer}).Execute(t.Context(), workspaceActor())
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	out := workspaceOutput(read, nil, nil)
	for _, wanted := range []string{
		"id", "slug", "display_name", "status", "default_locale", "default_time_zone",
		"require_admin_totp", "created_at", "version",
	} {
		if _, held := out[wanted]; !held {
			t.Errorf("the answer has no %q", wanted)
		}
	}
	if _, held := out["settings"]; held {
		t.Error("the answer carries the settings document, which is the adapter's shape")
	}
	// The anchoring target is the second modelled setting the answer carries (issue 774): absent
	// while anchoring is off, the identifier once it is on.
	if _, held := out["audit_anchor_target_id"]; held {
		t.Error("anchoring is off and the answer names a target")
	}
	read.Settings.AuditAnchorTargetID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	if got := workspaceOutput(read, nil, nil)["audit_anchor_target_id"]; got != "0192f000-0000-7000-8000-0000000000b1" {
		t.Errorf("the answer names %v as the anchoring target", got)
	}
}

// The workspace's own read answers an operator's opening of the password while it stands - until when
// for every member, who asked and why only for a reader of the configuration (ADR-0078 §3) - and none
// once its end has passed, whatever the row still holds until the end is recorded.
func TestTheWorkspaceReadAnswersAnOpeningWhileItStands(t *testing.T) {
	f := newWorkspaceFixture(at())
	configuration := &permitter{allowed: true}
	f.writer.Permits = configuration
	read := func() usecase.Output {
		t.Helper()
		out, err := ReadWorkspace{Writer: f.writer}.invoke(t.Context(), workspaceActor(), usecase.Input{})
		if err != nil {
			t.Fatalf("reading: %v", err)
		}
		return out
	}
	if _, held := read()["password_opening"]; held {
		t.Error("a workspace with no opening answers one")
	}

	f.store.row.PasswordOpening = domain.PasswordOpening{
		Until: at().Add(time.Hour), Requester: "TICKET-4711", Reason: "the directory answers 500",
	}
	opening, _ := read()["password_opening"].(usecase.Output)
	if until, _ := opening["until"].(time.Time); !until.Equal(at().Add(time.Hour)) ||
		opening["requester"] != "TICKET-4711" || opening["reason"] != "the directory answers 500" {
		t.Errorf("a configuration reader is answered %+v", opening)
	}
	asked := configuration.requests[len(configuration.requests)-1]
	if asked.Permission != service.PermissionReadConfiguration {
		t.Errorf("the attribution was decided by %s", asked.Permission)
	}

	// A member without the configuration permission learns that the password is open and until when.
	configuration.allowed = false
	opening, _ = read()["password_opening"].(usecase.Output)
	if until, _ := opening["until"].(time.Time); !until.Equal(at().Add(time.Hour)) {
		t.Errorf("a member is not told until when: %+v", opening)
	}
	if _, held := opening["requester"]; held {
		t.Errorf("a member reads who asked: %+v", opening)
	}
	if _, held := opening["reason"]; held {
		t.Errorf("a member reads why: %+v", opening)
	}
	// Nobody to ask is nobody told.
	f.writer.Permits = nil
	if opening, _ := read()["password_opening"].(usecase.Output); opening["requester"] != nil {
		t.Errorf("a writer without a permitter answered the requester: %+v", opening)
	}

	f.writer.Permits = configuration
	f.store.row.PasswordOpening.Until = at()
	if _, held := read()["password_opening"]; held {
		t.Error("an opening at its end is still answered")
	}
}
