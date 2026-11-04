// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// What a hold would keep, before the erasure starts (UC-PRV-03 check 11), through the registry: the
// door every channel shares.

type previewRig struct {
	requests   *requestStore
	authorizer *authorizerDouble
	erasure    *erasureHarness
	registry   *usecase.Registry
}

func newPreviewRig(t *testing.T, setup ...func(*erasureHarness)) *previewRig {
	t.Helper()
	rig := &previewRig{requests: newRequestStore(), authorizer: &authorizerDouble{}, erasure: newErasureHarness()}
	for _, s := range setup {
		s(rig.erasure)
	}
	cases := Cases{
		Requests: rig.requests, Authorizer: rig.authorizer, Audit: &auditSink{},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	registry, err := usecase.NewRegistry(nil,
		PreviewErasure{Cases: cases, Eraser: rig.erasure.eraserFor(rig.requests)}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	rig.registry = registry
	return rig
}

func (r *previewRig) preview(request domain.Request, in usecase.Input) (usecase.Output, error) {
	r.requests.stored[request.ID] = request
	if in == nil {
		in = usecase.Input{}
	}
	in["request_id"] = request.ID.String()
	return r.registry.Invoke(context.Background(), PreviewErasureName, actor(), in)
}

func TestThePreviewNamesWhatEachHoldWouldKeep(t *testing.T) {
	rig := newPreviewRig(t)
	rig.erasure.holds.holds = lifecycle.Holds{{ID: erasureHoldID, Scope: lifecycle.HoldContainer, ScopeID: erasureHub}}
	request := erasureCase("")
	request.Status = domain.StatusReceived

	out, err := rig.preview(request, usecase.Input{"mode": "FULL_DELETE"})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if out.String("mode") != "FULL_DELETE" {
		t.Errorf("the preview is for %q", out.String("mode"))
	}
	parts, _ := out["kept"].([]map[string]any)
	if len(parts) != 1 || parts[0]["comments"] != 2 || parts[0]["assignments"] != 3 ||
		parts[0]["hold_id"] != erasureHoldID.String() {
		t.Errorf("the preview answered %+v", parts)
	}
	if _, reason := parts[0]["reason"]; reason {
		t.Error("the hold's reason reached the preview")
	}
	// The list's right, asked of the case.
	asked := rig.authorizer.requests[0]
	if asked.Permission != domainservice.PermissionManageMembers || asked.TokenScope != privacyRead {
		t.Errorf("the preview asked for %s / %s", asked.Permission, asked.TokenScope)
	}
	// Nothing was touched: a preview reads.
	for _, step := range rig.erasure.storage.order {
		if step != "contributions" {
			t.Errorf("a preview ran %q", step)
		}
	}

	// Without a mode it is the case's, and without one there ANONYMIZE - which keeps no comment.
	out, err = rig.preview(request, nil)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	parts, _ = out["kept"].([]map[string]any)
	if out.String("mode") != "ANONYMIZE" || len(parts) != 1 || parts[0]["comments"] != 0 {
		t.Errorf("the default preview answered %v", out)
	}
}

func TestAPreviewOfWhatIsNoErasureIsRefused(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*domain.Request)
		code   string
	}{
		{"an access case", func(r *domain.Request) { r.Kind = domain.KindAccess }, domain.CodeNotAnErasure},
		{"a closed case", func(r *domain.Request) { r.Status = domain.StatusCompleted }, domain.CodeRequestClosed},
		{"an installation-wide case", func(r *domain.Request) { r.Scope = domain.ScopeInstallation }, domain.CodeInstallationScopeDenied},
	} {
		t.Run(c.name, func(t *testing.T) {
			rig := newPreviewRig(t)
			request := erasureCase(domain.ModeAnonymize)
			c.change(&request)
			_, err := rig.preview(request, nil)
			if shared.AsError(err).DetailCode != c.code {
				t.Errorf("refused with %v, want %s", err, c.code)
			}
		})
	}

	rig := newPreviewRig(t)
	if _, err := rig.preview(erasureCase(""), usecase.Input{"mode": "SHRED"}); err == nil {
		t.Error("a mode outside the enum was accepted")
	}
}

// A case known only by address: a workspace hold keeps what the address sent, so the preview names
// that hold, as the erasure records it - not "nothing reaches this person".
func TestThePreviewOfAnAddressCaseNamesTheWorkspaceHold(t *testing.T) {
	addresses := &addressStore{held: 3}
	rig := newPreviewRig(t, func(h *erasureHarness) { h.addresses = addresses })
	hold := shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	rig.erasure.holds.holds = lifecycle.Holds{{ID: hold, Scope: lifecycle.HoldTenant, Reason: "Litigation"}}

	out, err := rig.preview(addressCase(domain.ModeFullDelete), nil)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	parts, _ := out["kept"].([]map[string]any)
	if len(parts) != 1 || parts[0]["intake"] != 3 || parts[0]["hold_id"] != hold.String() {
		t.Errorf("the preview answered %+v", parts)
	}
	if len(addresses.erased) != 0 {
		t.Errorf("a preview erased %v", addresses.erased)
	}

	// Without a hold on the workspace, nothing is kept: a narrower hold does not reach an address.
	rig.erasure.holds.holds = lifecycle.Holds{{ID: hold, Scope: lifecycle.HoldContainer, ScopeID: erasureHub}}
	out, err = rig.preview(addressCase(domain.ModeFullDelete), nil)
	if parts, _ := out["kept"].([]map[string]any); err != nil || len(parts) != 0 {
		t.Errorf("a container hold kept %+v (%v)", parts, err)
	}
}

// Every kind the eraser handles has a line in the preview (user-experience.md §5.9): a field added
// to Erased without a line here fails, so the confirmation never says less than the erasure does.
func TestEveryKindTheEraserHandlesHasALineInThePreview(t *testing.T) {
	linesOf := map[string][]string{
		"Credentials": {"CREDENTIALS"}, "Notifications": {"NOTIFICATIONS"}, "AiDrafts": {"AI_DRAFTS"},
		"Kudos": {"KUDOS"}, "Arrangements": {"ARRANGEMENTS"}, "Time": {"TIME"},
		"Assignments": {"ASSIGNMENTS"}, "Comments": {"COMMENTS"}, "Intake": {"INTAKE"}, "Media": {"MEDIA"},
		"Writings":       {"PAGE_COMMENTS", "WHITEBOARD_COMMENTS", "GOAL_CHECK_INS", "PORTFOLIO_UPDATES", "PAGE_DRAFTS"},
		"AccountRemoved": {"ACCOUNT"}, "AccountAnonymised": {"ACCOUNT"}, "AccountKept": {"ACCOUNT"},
		// Not kinds: the mode the erasure ran in, and what the holds kept, which `kept` answers.
		"Mode": nil, "Kept": nil,
	}
	rig := newPreviewRig(t)
	request := erasureCase("")
	request.Status = domain.StatusReceived
	out, err := rig.preview(request, usecase.Input{"mode": "FULL_DELETE"})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	lines, _ := out["lines"].([]map[string]any)
	has := map[string]bool{}
	for _, line := range lines {
		kind, _ := line["kind"].(string)
		has[kind] = true
		if line["fate"] == "" {
			t.Errorf("the line of %s says no fate", kind)
		}
	}
	erased := reflect.TypeOf(Erased{})
	for i := range erased.NumField() {
		field := erased.Field(i).Name
		kinds, known := linesOf[field]
		if !known {
			t.Errorf("the eraser handles %s, which has no line in the preview", field)
		}
		for _, kind := range kinds {
			if !has[kind] {
				t.Errorf("%s has no %s line in the preview: %v", field, kind, lines)
			}
		}
	}
}
