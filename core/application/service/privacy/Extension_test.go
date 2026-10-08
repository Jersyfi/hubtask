// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The extension through the registry, the one door REST, MCP and automation share (UC-PRV-01
// check 9). Through registry.Invoke rather than the handler, because the registry's own refusals -
// a missing field, an unknown reason - come first and a handler test would never meet them.
//
// The harness's clock stands at 26 August 2026 10:00 UTC and its workspace counts in Berlin: a case
// recorded then is due 25 September 2026 10:00 UTC and can be extended to 26 November 2026.

func (h *harness) registry(t *testing.T) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		CreateDataSubjectRequest{Cases: h.cases()}.Descriptor(),
		ListDataSubjectRequests{Cases: h.cases()}.Descriptor(),
		UpdateDataSubjectRequest{Cases: h.cases()}.Descriptor(),
		ExtendDataSubjectRequest{Cases: h.cases()}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	return registry
}

// recorded is a case recorded through the registry, and what it answered.
func (h *harness) recorded(t *testing.T, as appshared.ActorContext, scope domain.Scope) usecase.Output {
	t.Helper()
	out, err := h.registry(t).Invoke(t.Context(), CreateDataSubjectRequestName, as, usecase.Input{
		"kind": string(domain.KindAccess), "scope": string(scope), "subject_email": "anna@example.org",
		"notes": "Asked by letter",
	})
	if err != nil {
		t.Fatalf("recording: %v", err)
	}
	return out
}

func extensionInput(id string) usecase.Input {
	return usecase.Input{
		"request_id": id, "due_on": "2026-11-26", "reason": string(domain.ReasonComplexity),
		"informed_on": "2026-08-26",
	}
}

func TestACaseIsExtendedOnceThroughTheRegistry(t *testing.T) {
	h := newHarness()
	recorded := h.recorded(t, actor(), domain.ScopeTenant)
	if recorded.String("extendable_until") != "2026-11-26" {
		t.Fatalf("a new case answers extendable_until %q, want 2026-11-26", recorded["extendable_until"])
	}

	// The day the case answered is sent back as the new deadline (P-05).
	in := extensionInput(recorded.String("id"))
	in["due_on"] = recorded.String("extendable_until")
	out, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), in)
	if err != nil {
		t.Fatalf("extending: %v", err)
	}

	if got := out["due_at"].(time.Time).Format(time.RFC3339); got != "2026-11-26T22:59:59Z" {
		t.Errorf("the deadline is %s, want the end of 26 November in Berlin", got)
	}
	if got := out["original_due_at"].(time.Time).Format(time.RFC3339); got != "2026-09-25T10:00:00Z" {
		t.Errorf("the original deadline is %s", got)
	}
	if out.String("extension_reason") != "COMPLEXITY" || out.String("informed_on") != "2026-08-26" {
		t.Errorf("the extension came back as %v", out)
	}
	if _, present := out["extendable_until"]; present {
		t.Error("an extended case still answers extendable_until")
	}

	// The entry: the action, the reason and both dates, the right as its legal basis, no notes.
	entry := h.audit.entries[len(h.audit.entries)-1]
	if entry.Action != RequestExtendedAction || entry.LegalBasis != "dsr.access" ||
		entry.Severity != audit.SeverityNotice {
		t.Errorf("the entry is %s / %s / %s", entry.Action, entry.LegalBasis, entry.Severity)
	}
	field := func(name string) map[string]any {
		value, _ := entry.Changes[name].(map[string]any)
		return value
	}
	if field("due_at")["from"] != "2026-09-25T10:00:00Z" || field("due_at")["to"] != "2026-11-26T22:59:59Z" ||
		field("extension_reason")["to"] != "COMPLEXITY" || field("informed_on")["to"] != "2026-08-26" {
		t.Errorf("the entry records %v", entry.Changes)
	}
	if _, present := entry.Changes["notes"]; present || strings.Contains(fmt.Sprint(entry.Changes), "Asked by letter") {
		t.Errorf("the entry carries the notes: %v", entry.Changes)
	}

	// A second extension is refused, and the list now answers both dates and no bound.
	again := extensionInput(recorded.String("id"))
	again["due_on"] = "2026-11-20"
	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), again); shared.AsError(err).DetailCode != domain.CodeAlreadyExtended {
		t.Errorf("a second extension was answered %v", err)
	}
	page, err := h.registry(t).Invoke(t.Context(), ListDataSubjectRequestsName, actor(), usecase.Input{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	row := page["data"].([]usecase.Output)[0]
	if _, present := row["extendable_until"]; present || row["original_due_at"] == nil {
		t.Errorf("the listed case reads %v", row)
	}
}

func TestTheExtensionAsksForTheAdministratorsLine(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")
	h.authorizer.requests = nil

	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), extensionInput(id)); err != nil {
		t.Fatalf("extending: %v", err)
	}
	asked := h.authorizer.requests[0]
	if asked.Permission != domainservice.PermissionManageMembers || asked.TokenScope != privacyManage ||
		asked.Action != RequestExtendedAction || asked.TargetID.String() != id {
		t.Errorf("the extension asked %+v", asked)
	}
}

// A refusal by the authorisation service - a member without MANAGE_MEMBERS, a token without
// privacy:manage - leaves nothing behind.
func TestARefusedExtensionLeavesNothingBehind(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")
	entries := len(h.audit.entries)
	h.authorizer.refuse = shared.ErrForbidden.WithDetail("access.token_scope_missing")

	_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), extensionInput(id))
	if shared.AsError(err).DetailCode != "access.token_scope_missing" {
		t.Fatalf("the refusal came back as %v", err)
	}
	if h.requests.stored[shared.MustParseID(id)].Extended() || len(h.audit.entries) != entries {
		t.Error("a refused extension wrote something")
	}
}

func TestTheRegistryRefusesAnIncompleteExtension(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")

	for field, code := range map[string]string{
		"due_on": "usecase.field_required", "reason": "usecase.field_required",
		"informed_on": "usecase.field_required",
	} {
		in := extensionInput(id)
		delete(in, field)
		_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), in)
		problem := shared.AsError(err)
		if problem == nil || problem.DetailCode != "usecase.input_invalid" ||
			len(problem.Fields) != 1 || problem.Fields[0].Path != "/"+field || problem.Fields[0].Code != code {
			t.Errorf("without %s: %v", field, err)
		}
	}

	in := extensionInput(id)
	in["reason"] = "HOLIDAYS"
	_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), in)
	if problem := shared.AsError(err); problem == nil || len(problem.Fields) != 1 ||
		problem.Fields[0].Code != "usecase.field_not_in_enum" {
		t.Errorf("an unknown reason: %v", err)
	}

	for field, code := range map[string]string{
		"due_on": "privacy.due_on_malformed", "informed_on": "privacy.informed_on_malformed",
	} {
		in := extensionInput(id)
		in[field] = "26.11.2026"
		_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), in)
		if problem := shared.AsError(err); problem == nil || problem.DetailCode != code {
			t.Errorf("a malformed %s: %v", field, err)
		}
	}
	if h.requests.stored[shared.MustParseID(id)].Extended() {
		t.Error("a refused extension was stored")
	}
}

// An installation-wide case is extended by the operator, whose credential carries admin:tenants.
func TestAnInstallationWideCaseIsExtendedOnlyByTheOperator(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, operator(), domain.ScopeInstallation).String("id")

	_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(), extensionInput(id))
	if shared.AsError(err).DetailCode != domain.CodeInstallationScopeDenied {
		t.Fatalf("an administrator without admin:tenants was answered %v", err)
	}
	if h.requests.stored[shared.MustParseID(id)].Extended() {
		t.Fatal("the refused extension was stored")
	}
	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, operator(), extensionInput(id)); err != nil {
		t.Errorf("the operator could not extend: %v", err)
	}
}

// Moving a date is not destructive: an agent extends without agent:destructive, which the update
// - it can start an erasure - still demands.
func TestAnAgentExtendsWithoutTheDestructiveCapability(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")
	agent := actor()
	agent.Kind = appshared.ActorAIAgent

	if _, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, agent, extensionInput(id)); err != nil {
		t.Errorf("an agent could not extend: %v", err)
	}
	_, err := h.registry(t).Invoke(t.Context(), UpdateDataSubjectRequestName, agent, usecase.Input{
		"request_id": id, "notes": "x",
	})
	if shared.AsError(err).DetailCode != "agent.destructive_not_permitted" {
		t.Errorf("the update let an agent through: %v", err)
	}
}

func TestAnExtensionOfNoCaseIsNotFound(t *testing.T) {
	h := newHarness()
	_, err := h.registry(t).Invoke(t.Context(), ExtendDataSubjectRequestName, actor(),
		extensionInput("0192f000-0000-7000-8000-0000000009ff"))
	if shared.AsError(err).DetailCode != domain.CodeRequestNotFound {
		t.Errorf("an unknown case was answered %v", err)
	}
	if _, err := (ExtendDataSubjectRequest{Cases: h.cases()}).Execute(t.Context(), actor(), ExtendCommand{}); shared.AsError(err).DetailCode != domain.CodeRequestNotFound {
		t.Errorf("no case at all was answered %v", err)
	}
}

// racingStore completes the case between the read and the conditional write, as a concurrent
// request would: the write finds nothing, and the answer is what is true by then.
type racingStore struct{ *requestStore }

func (s racingStore) Extend(ctx context.Context, request domain.Request, at time.Time) (bool, error) {
	stored := s.stored[request.ID]
	stored.Status = domain.StatusCompleted
	s.stored[request.ID] = stored
	return s.requestStore.Extend(ctx, request, at)
}

func TestAnExtensionThatLostARaceIsAnsweredWithWhatIsTrueNow(t *testing.T) {
	h := newHarness()
	id := h.recorded(t, actor(), domain.ScopeTenant).String("id")
	entries := len(h.audit.entries)

	cases := h.cases()
	cases.Requests = racingStore{h.requests}
	_, err := ExtendDataSubjectRequest{Cases: cases}.Descriptor().Handler.
		Invoke(t.Context(), actor(), extensionInput(id))
	if shared.AsError(err).DetailCode != domain.CodeRequestClosed {
		t.Errorf("the lost race was answered %v", err)
	}
	if len(h.audit.entries) != entries {
		t.Error("the lost extension was recorded")
	}
}
