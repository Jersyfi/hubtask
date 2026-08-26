// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The case, as the three use cases move it (E-10, data-protection.md §4). What is under test is
// which permission each step asks for, what the trail records, and when the work is queued.

type harness struct {
	requests   *requestStore
	jobs       *queueDouble
	authorizer *authorizerDouble
	audit      *auditSink
	uow        *unitOfWork
}

func newHarness() *harness {
	return &harness{
		requests: newRequestStore(), jobs: &queueDouble{},
		authorizer: &authorizerDouble{}, audit: &auditSink{}, uow: &unitOfWork{},
	}
}

func (h *harness) cases() Cases {
	return Cases{
		Requests: h.requests, Jobs: h.jobs, Authorizer: h.authorizer, Audit: h.audit,
		UnitOfWork: h.uow, Clock: clock.Fixed(now), IDs: &idSource{},
	}
}

func createCommand(change func(*CreateCommand)) CreateCommand {
	cmd := CreateCommand{Kind: domain.KindAccess, SubjectAccountID: subjectID, TargetID: targetID}
	change(&cmd)
	return cmd
}

func TestRecordingACaseAsksForTheAdministratorsLine(t *testing.T) {
	h := newHarness()

	request, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}

	if len(h.requests.stored) != 1 {
		t.Fatalf("%d cases were written", len(h.requests.stored))
	}
	if request.Status != domain.StatusReceived || !request.DueAt.Equal(now.Add(domain.DefaultDeadline)) {
		t.Errorf("the case came back as %+v", request)
	}

	// A data subject request is about a person rather than about the shape of the workspace.
	asked := h.authorizer.requests[0]
	if asked.Permission != domainservice.PermissionManageMembers {
		t.Errorf("recording a case asked for %s", asked.Permission)
	}
	if asked.TokenScope != privacyManage {
		t.Errorf("recording a case asked for the scope %q", asked.TokenScope)
	}

	// The entry names the occasion, which is the `legal_basis` field audit.md §2 has carried since
	// phase 0 and nothing had ever written.
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d entries were written", len(h.audit.entries))
	}
	entry := h.audit.entries[0]
	if entry.Action != RequestRecordedAction || entry.LegalBasis != "dsr.access" {
		t.Errorf("the case was recorded as %s / %q", entry.Action, entry.LegalBasis)
	}
	if entry.TargetID != request.ID {
		t.Errorf("the entry names the target %s", entry.TargetID)
	}
}

// Nothing of the person's is touched by recording a case: no job, no deletion, no export.
func TestRecordingACaseTouchesNothingOfTheSubjects(t *testing.T) {
	h := newHarness()

	if _, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(c *CreateCommand) {
			c.Kind = domain.KindErasure
		})); err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	// The deadline watch and nothing else: recording a case starts no work on the person's data.
	if kinds := queuedKinds(h); len(kinds) != 1 || kinds[0] != queue.KindPrivacyDeadlines {
		t.Errorf("recording an erasure queued %v", kinds)
	}
}

// Crossing the tenant boundary needs the credential that says so. A role says what somebody may do
// in *this* workspace, and no role in any workspace can answer for another one.
func TestAnInstallationWideCaseNeedsTheInstanceScope(t *testing.T) {
	h := newHarness()
	wide := createCommand(func(c *CreateCommand) { c.Scope = domain.ScopeInstallation })

	_, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), wide)
	if err == nil {
		t.Fatal("an installation-wide case was recorded without the instance scope")
	}
	var problem *shared.Error
	if !errors.As(err, &problem) || problem.DetailCode != domain.CodeInstallationScopeDenied {
		t.Errorf("the refusal came back as %v", err)
	}
	if len(h.requests.stored) != 0 {
		t.Error("a refused case was written")
	}

	if _, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), operator(), wide); err != nil {
		t.Fatalf("an operator could not record an installation-wide case: %v", err)
	}
}

func TestStartingAnErasureAsksForTheOwnersLine(t *testing.T) {
	h := newHarness()
	recorded, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(c *CreateCommand) {
			c.Kind = domain.KindErasure
		}))
	if err != nil {
		t.Fatalf("recording the case: %v", err)
	}
	h.authorizer.requests = nil

	moved, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusInProgress,
			ErasureMode: domain.ModeAnonymize,
		})
	if err != nil {
		t.Fatalf("starting the erasure: %v", err)
	}

	// The matrix's line for the one thing an administrator cannot do, because an erasure destroys
	// work that belongs to the workspace as much as to the person.
	if h.authorizer.requests[0].Permission != domainservice.PermissionDeleteContainer {
		t.Errorf("starting an erasure asked for %s", h.authorizer.requests[0].Permission)
	}
	if moved.Status != domain.StatusInProgress || moved.ErasureMode != domain.ModeAnonymize {
		t.Errorf("the case moved to %+v", moved)
	}

	// The work is queued, once, against the case.
	work := queuedOf(h, queue.KindPrivacyRequest)
	if len(work) != 1 {
		t.Fatalf("the work was queued as %+v", h.jobs.requests)
	}
	if work[0].Payload["request_id"] != recorded.ID.String() {
		t.Errorf("the job names %v", work[0].Payload)
	}

	// And the entry is a warning rather than a notice: an erasure that has started is a deletion
	// nobody can undo.
	entry := h.audit.entries[len(h.audit.entries)-1]
	if entry.Action != RequestStartedAction || entry.Severity != audit.SeverityWarning {
		t.Errorf("starting the erasure was recorded as %s / %s", entry.Action, entry.Severity)
	}
	if entry.LegalBasis != "dsr.erasure" {
		t.Errorf("the entry names the occasion %q", entry.LegalBasis)
	}
}

// Starting an export writes to a target somebody has already approved, which is running the
// workspace rather than destroying anything.
func TestStartingAnExportAsksForTheAdministratorsLine(t *testing.T) {
	h := newHarness()
	recorded, _ := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))
	h.authorizer.requests = nil

	if _, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusInProgress,
		}); err != nil {
		t.Fatalf("starting the export: %v", err)
	}
	if h.authorizer.requests[0].Permission != domainservice.PermissionManageMembers {
		t.Errorf("starting an export asked for %s", h.authorizer.requests[0].Permission)
	}
	if len(queuedOf(h, queue.KindPrivacyRequest)) != 1 {
		t.Errorf("the export queued %d jobs", len(h.jobs.requests))
	}
}

// A kind that needs no special path queues nothing: a job for it would be a job with nothing to do.
func TestStartingARectificationQueuesNothing(t *testing.T) {
	h := newHarness()
	recorded, _ := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(c *CreateCommand) {
			c.Kind = domain.KindRectification
		}))

	if _, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusInProgress,
		}); err != nil {
		t.Fatalf("starting the rectification: %v", err)
	}
	if len(queuedOf(h, queue.KindPrivacyRequest)) != 0 {
		t.Errorf("a rectification queued work: %v", h.jobs.requests)
	}
}

func TestARefusalCarriesItsReasonIntoTheTrail(t *testing.T) {
	h := newHarness()
	recorded, _ := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))

	if _, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusRejected,
		}); err == nil {
		t.Fatal("a case was refused with no reason")
	}

	rejected, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusRejected,
			RejectionReason: "Identity could not be established",
		})
	if err != nil {
		t.Fatalf("refusing: %v", err)
	}
	if rejected.Status != domain.StatusRejected {
		t.Errorf("the case is %s", rejected.Status)
	}

	entry := h.audit.entries[len(h.audit.entries)-1]
	if entry.Action != RequestRejectedAction {
		t.Errorf("the refusal was recorded as %s", entry.Action)
	}
	reason, _ := entry.Changes["rejection_reason"].(map[string]any)
	if reason["to"] != "Identity could not be established" {
		t.Errorf("the entry carries %v", entry.Changes)
	}
}

// An illegitimate step is refused by name rather than ignored.
func TestAnIllegitimateStepIsRefusedByName(t *testing.T) {
	h := newHarness()
	recorded, _ := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))

	_, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusCompleted,
		})
	if err == nil {
		t.Fatal("a case went from RECEIVED straight to COMPLETED")
	}
	var problem *shared.Error
	if !errors.As(err, &problem) || problem.DetailCode != domain.CodeTransitionRefused {
		t.Errorf("the refusal came back as %v", err)
	}
	if problem.Params["from"] != string(domain.StatusReceived) {
		t.Errorf("the refusal does not say where the case was: %v", problem.Params)
	}
}

func TestTheListAnswersWhatIsStillOwed(t *testing.T) {
	h := newHarness()
	create := CreateDataSubjectRequest{Cases: h.cases()}
	open, _ := create.Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))
	closed, _ := create.Execute(context.Background(), actor(), createCommand(func(c *CreateCommand) {
		c.Kind = domain.KindRectification
	}))
	if _, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: closed.ID, Status: domain.StatusRejected, RejectionReason: "Not this workspace",
		}); err != nil {
		t.Fatalf("refusing: %v", err)
	}

	page, err := (ListDataSubjectRequests{Cases: h.cases()}).
		Execute(context.Background(), actor(), ListQuery{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Requests) != 1 || page.Requests[0].ID != open.ID {
		t.Errorf("the list answered %d cases", len(page.Requests))
	}
	if h.authorizer.requests[len(h.authorizer.requests)-1].TokenScope != privacyRead {
		t.Error("the list asked for the managing scope rather than the reading one")
	}

	// And the closed ones when asked for, which is a different question: what did we do.
	page, err = (ListDataSubjectRequests{Cases: h.cases()}).
		Execute(context.Background(), actor(), ListQuery{IncludeClosed: true})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Requests) != 2 {
		t.Errorf("the whole list answered %d cases", len(page.Requests))
	}
}

// "What falls due in the next seven days" includes what is already late, because that is the most
// urgent of it.
func TestTheDeadlineFilterReachesTheRepository(t *testing.T) {
	h := newHarness()

	if _, err := (ListDataSubjectRequests{Cases: h.cases()}).
		Execute(context.Background(), actor(), ListQuery{DueWithinDays: 7}); err != nil {
		t.Fatalf("listing: %v", err)
	}
	asked := h.requests.asked[0]
	if !asked.DueBefore.Equal(now.AddDate(0, 0, 7)) {
		t.Errorf("the filter asked for %s", asked.DueBefore)
	}
	if asked.Size != DefaultPageSize {
		t.Errorf("the page size defaulted to %d", asked.Size)
	}
}

func TestTheDescriptorsTakeWhatTheControllerSends(t *testing.T) {
	for name, in := range map[string]usecase.Input{
		CreateDataSubjectRequestName: {
			"kind": string(domain.KindAccess), "scope": string(domain.ScopeTenant),
			"subject_account_id": subjectID.String(), "subject_email": "anna@example.org",
			"due_at": now.Format(time.RFC3339Nano), "target_id": targetID.String(),
			"notes": "By email on the 26th",
		},
		ListDataSubjectRequestsName: {
			"status": string(domain.StatusReceived), "kind": string(domain.KindErasure),
			"due_within_days": 7, "include_closed": true, "cursor": "opaque", "size": 25,
		},
		UpdateDataSubjectRequestName: {
			"request_id": subjectID.String(), "status": string(domain.StatusInProgress),
			"erasure_mode": string(domain.ModeFullDelete), "handled_by": accountID.String(),
			"rejection_reason": "", "target_id": targetID.String(), "notes": "Started",
		},
	} {
		descriptor := descriptorNamed(t, name)
		if err := descriptor.ValidateInput(in); err != nil {
			t.Errorf("%s refuses the input the controller builds: %v", name, err)
		}
	}
}

func descriptorNamed(t *testing.T, name string) usecase.Descriptor {
	t.Helper()
	switch name {
	case CreateDataSubjectRequestName:
		return CreateDataSubjectRequest{}.Descriptor()
	case ListDataSubjectRequestsName:
		return ListDataSubjectRequests{}.Descriptor()
	default:
		return UpdateDataSubjectRequest{}.Descriptor()
	}
}

// The handlers are what the three channels actually call, and where a request becomes a command.
// Every field is asserted on the way through: a field parsed into the wrong place is a case about
// somebody else.
func TestTheHandlersTurnTheRequestIntoTheCase(t *testing.T) {
	h := newHarness()

	out, err := (CreateDataSubjectRequest{Cases: h.cases()}).Descriptor().Handler.
		Invoke(context.Background(), operator(), usecase.Input{
			"kind":               string(domain.KindPortability),
			"scope":              string(domain.ScopeInstallation),
			"subject_account_id": subjectID.String(),
			"subject_email":      "anna@example.org",
			"due_at":             now.Add(72 * time.Hour).Format(time.RFC3339),
			"target_id":          targetID.String(),
			"notes":              "By email",
		})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}

	if out.String("kind") != string(domain.KindPortability) || out.String("scope") != string(domain.ScopeInstallation) {
		t.Errorf("the case came back as %v", out)
	}
	if out.String("subject_email") != "anna@example.org" || out.String("notes") != "By email" {
		t.Errorf("the subject came back as %v", out)
	}
	stored := h.requests.stored[shared.MustParseID(out.String("id"))]
	if !stored.DueAt.Equal(now.Add(72 * time.Hour)) {
		t.Errorf("the deadline reached the case as %s", stored.DueAt)
	}

	page, err := (ListDataSubjectRequests{Cases: h.cases()}).Descriptor().Handler.
		Invoke(context.Background(), actor(), usecase.Input{"size": 10})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := page["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("the list answered %d cases", len(rows))
	}

	moved, err := (UpdateDataSubjectRequest{Cases: h.cases()}).Descriptor().Handler.
		Invoke(context.Background(), operator(), usecase.Input{
			"request_id": out.String("id"), "status": string(domain.StatusInProgress),
			"handled_by": accountID.String(),
		})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if moved.String("status") != string(domain.StatusInProgress) {
		t.Errorf("the case moved to %v", moved["status"])
	}
	if moved.String("handled_by") != accountID.String() {
		t.Errorf("the case is handled by %v", moved["handled_by"])
	}
}

func TestAMalformedDeadlineIsRefusedWithItsField(t *testing.T) {
	h := newHarness()

	_, err := (CreateDataSubjectRequest{Cases: h.cases()}).Descriptor().Handler.
		Invoke(context.Background(), actor(), usecase.Input{
			"kind": string(domain.KindAccess), "subject_email": "anna@example.org",
			"due_at": "next Tuesday",
		})
	if err == nil {
		t.Fatal("a deadline that is not a date was accepted")
	}
	var problem *shared.Error
	if !errors.As(err, &problem) || problem.DetailCode != "privacy.due_at_malformed" {
		t.Errorf("the refusal came back as %v", err)
	}
}

// A malformed identifier is refused before anything is written.
func TestAMalformedIdentifierIsRefused(t *testing.T) {
	h := newHarness()

	for _, field := range []string{"subject_account_id", "target_id"} {
		if _, err := (CreateDataSubjectRequest{Cases: h.cases()}).Descriptor().Handler.
			Invoke(context.Background(), actor(), usecase.Input{
				"kind": string(domain.KindAccess), field: "not-an-identifier",
			}); err == nil {
			t.Errorf("%q was accepted as an identifier", field)
		}
	}
	if len(h.requests.stored) != 0 {
		t.Error("a case was written with a malformed identifier")
	}
}

// The list refuses a filter value that does not exist rather than answering an empty page, which a
// caller would read as "there are none".
func TestAFilterValueThatDoesNotExistIsRefused(t *testing.T) {
	h := newHarness()
	list := ListDataSubjectRequests{Cases: h.cases()}

	for _, query := range []ListQuery{{Status: "PONDERING"}, {Kind: "TELEPATHY"}} {
		if _, err := list.Execute(context.Background(), actor(), query); err == nil {
			t.Errorf("the filter %+v was accepted", query)
		}
	}
}

func TestThePageSizeIsClamped(t *testing.T) {
	for _, c := range []struct{ asked, want int }{
		{0, DefaultPageSize}, {10, 10}, {5000, MaxPageSize},
	} {
		if got := PageSize(c.asked); got != c.want {
			t.Errorf("a size of %d was clamped to %d, want %d", c.asked, got, c.want)
		}
	}
}

// Every kind names its own occasion, which is what `legal_basis` carries into the trail.
func TestEveryKindNamesItsOccasion(t *testing.T) {
	seen := map[string]bool{}
	for _, kind := range domain.Kinds() {
		basis := LegalBasisOf(kind)
		if basis == "dsr" || seen[basis] {
			t.Errorf("%s names the occasion %q", kind, basis)
		}
		seen[basis] = true
	}
	if LegalBasisOf("TELEPATHY") != "dsr" {
		t.Error("a kind this build does not know names something specific")
	}
}

// A case somebody deleted under the caller is a not-found rather than a silent success.
func TestACaseThatVanishedUnderTheCallerIsNotFound(t *testing.T) {
	h := newHarness()
	recorded, _ := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {}))
	h.requests.missing = true

	if _, err := (UpdateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), UpdateCommand{
			RequestID: recorded.ID, Status: domain.StatusInProgress,
		}); err == nil {
		t.Fatal("a case nobody could write reported success")
	}
}

// A refused permission leaves nothing behind - no case, no job, no entry.
func TestARefusedStepLeavesNothingBehind(t *testing.T) {
	h := newHarness()
	h.authorizer.refuse = shared.ErrForbidden.WithDetail("access.not_permitted")

	if _, err := (CreateDataSubjectRequest{Cases: h.cases()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {})); err == nil {
		t.Fatal("a case was recorded without permission")
	}
	if len(h.requests.stored) != 0 || len(h.audit.entries) != 0 || len(h.jobs.requests) != 0 {
		t.Error("a refused step left something behind")
	}
}

// queuedOf answers the jobs of one kind, so that a test about the work is not confused by the
// deadline watch every recorded case seeds.
func queuedOf(h *harness, kind queue.Kind) []queue.Request {
	var found []queue.Request
	for _, request := range h.jobs.requests {
		if request.Kind == kind {
			found = append(found, request)
		}
	}
	return found
}

func queuedKinds(h *harness) []queue.Kind {
	var kinds []queue.Kind
	for _, request := range h.jobs.requests {
		kinds = append(kinds, request.Kind)
	}
	return kinds
}

// The watch is seeded by the write, because nothing may enumerate tenants - and a second case
// joins the watch that is already running rather than starting another.
func TestRecordingACaseSeedsTheDeadlineWatch(t *testing.T) {
	h := newHarness()
	create := CreateDataSubjectRequest{Cases: h.cases()}

	for range 2 {
		if _, err := create.Execute(context.Background(), actor(), createCommand(func(*CreateCommand) {})); err != nil {
			t.Fatalf("recording: %v", err)
		}
	}

	watches := queuedOf(h, queue.KindPrivacyDeadlines)
	if len(watches) != 2 {
		t.Fatalf("%d watches were asked for", len(watches))
	}
	if watches[0].DedupeKey != watches[1].DedupeKey {
		t.Errorf("two cases asked for two watches: %q and %q", watches[0].DedupeKey, watches[1].DedupeKey)
	}
	if watches[0].TenantID != tenantID {
		t.Errorf("the watch is for %s", watches[0].TenantID)
	}
}
