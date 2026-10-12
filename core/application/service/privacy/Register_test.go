// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The consent register (UC-PRV-05 check 12): who keeps their content out, for the people who
// manage members, and nobody else.

func registerOf(t *testing.T, consents *consentStore, authorizer *authorizerDouble) *usecase.Registry {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, ListConsents{
		Consents: consents, Authorizer: authorizer, UnitOfWork: &unitOfWork{},
	}.Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry
}

func TestTheRegisterNamesWhoKeepsTheirContentOut(t *testing.T) {
	at := clock.Fixed(now).Now()
	consents := &consentStore{stored: []domain.Consent{
		{AccountID: subjectID, Purpose: domain.PurposeAi, GrantedAt: at, RevokedAt: at, Source: domain.SourceTenantAdmin},
		{AccountID: accountID, Purpose: domain.PurposeAi, GrantedAt: at, RevokedAt: at, Source: domain.SourceUser},
		{AccountID: accountID, Purpose: domain.PurposeAi, Granted: true, GrantedAt: at.Add(time.Hour), Source: domain.SourceUser},
	}}
	authorizer := &authorizerDouble{}
	out, err := registerOf(t, consents, authorizer).Invoke(t.Context(), ListConsentsName, actor(),
		usecase.Input{"purpose": domain.PurposeAi})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["items"].([]usecase.Output)
	if len(rows) != 1 || rows[0].String("account_id") != subjectID.String() || rows[0].String("source") != domain.SourceTenantAdmin {
		t.Fatalf("listed %v: the one who gave consent again is not keeping out", rows)
	}
	if len(authorizer.requests) != 1 || authorizer.requests[0].Permission != domainservice.PermissionManageMembers {
		t.Errorf("asked %v", authorizer.requests)
	}
}

func TestAMemberCannotReadTheRegister(t *testing.T) {
	refused := shared.ErrForbidden.WithDetail("access.not_permitted")
	_, err := registerOf(t, &consentStore{}, &authorizerDouble{refuse: refused}).Invoke(t.Context(), ListConsentsName, actor(),
		usecase.Input{"purpose": domain.PurposeAi})
	if got := shared.AsError(err); got == nil || got.DetailCode != "access.not_permitted" {
		t.Errorf("answered %v", err)
	}
}
