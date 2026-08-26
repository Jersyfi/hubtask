// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// Objection, as the one thing it can technically be (E-10, Art. 21): the optional processing stops
// and the core features keep working.

type consentStore struct {
	stored  []domain.Consent
	ended   []string
	endings int
}

func (c *consentStore) Withdraw(
	_ context.Context, _ shared.ID, purpose string, _ time.Time,
) (int, error) {
	c.ended = append(c.ended, purpose)
	return c.endings, nil
}

func (c *consentStore) Record(_ context.Context, consent domain.Consent) error {
	c.stored = append(c.stored, consent)
	return nil
}

func (c *consentStore) Latest(context.Context, shared.ID, string) (domain.Consent, error) {
	if len(c.stored) == 0 {
		return domain.Consent{}, shared.ErrNotFound.WithDetail(domain.CodeConsentNotFound)
	}
	return c.stored[len(c.stored)-1], nil
}

func newWithdrawal(consents *consentStore, authorizer *authorizerDouble, sink *auditSink) WithdrawConsent {
	return WithdrawConsent{
		Consents: consents, Authorizer: authorizer, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSource{},
	}
}

// One's own consent is self-service, the way a preference is.
func TestWithdrawingOnesOwnConsentAsksForNoPermission(t *testing.T) {
	consents, authorizer, sink := &consentStore{endings: 1}, &authorizerDouble{}, &auditSink{}

	consent, err := newWithdrawal(consents, authorizer, sink).
		Execute(context.Background(), actor(), WithdrawCommand{Purpose: "ai_processing"})
	if err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	if len(authorizer.requests) != 0 {
		t.Errorf("withdrawing one's own consent asked for %v", authorizer.requests)
	}
	if consent.AccountID != accountID || consent.Source != domain.SourceUser {
		t.Errorf("the record came back as %+v", consent)
	}
	if consent.InForce() {
		t.Error("the consent is still in force after being withdrawn")
	}
	if len(consents.ended) != 1 || consents.ended[0] != "ai_processing" {
		t.Errorf("the standing consents ended were %v", consents.ended)
	}

	entry := sink.entries[0]
	if entry.Action != ConsentWithdrawnAction || entry.LegalBasis != "dsr.objection" {
		t.Errorf("the withdrawal was recorded as %s / %q", entry.Action, entry.LegalBasis)
	}
}

// Somebody else's is the administrator's, and the record says who wrote it.
func TestWithdrawingSomebodyElsesConsentAsksForTheAdministratorsLine(t *testing.T) {
	consents, authorizer, sink := &consentStore{}, &authorizerDouble{}, &auditSink{}

	consent, err := newWithdrawal(consents, authorizer, sink).
		Execute(context.Background(), actor(), WithdrawCommand{
			AccountID: subjectID, Purpose: "metering",
		})
	if err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	if len(authorizer.requests) != 1 {
		t.Fatalf("the request asked %d permissions", len(authorizer.requests))
	}
	if authorizer.requests[0].Permission != domainservice.PermissionManageMembers {
		t.Errorf("withdrawing somebody else's consent asked for %s", authorizer.requests[0].Permission)
	}
	if consent.Source != domain.SourceTenantAdmin {
		t.Errorf("the record says the source was %q", consent.Source)
	}
}

// The person said no, and the record says they said no - rather than a gap where an operator has
// to guess.
func TestWithdrawingSomethingNobodyGrantedIsStillRecorded(t *testing.T) {
	consents, authorizer, sink := &consentStore{endings: 0}, &authorizerDouble{}, &auditSink{}

	if _, err := newWithdrawal(consents, authorizer, sink).
		Execute(context.Background(), actor(), WithdrawCommand{Purpose: "ai_processing"}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if len(consents.stored) != 1 {
		t.Fatalf("%d records were written", len(consents.stored))
	}

	// How many standing consents it ended is in the entry, because "objected to something never
	// agreed to" and "took a consent back" are different facts.
	ended, _ := sink.entries[0].Changes["ended"].(map[string]any)
	if ended["to"] != 0 {
		t.Errorf("the entry says %v consents were ended", ended)
	}
}

func TestAPurposeIsRequired(t *testing.T) {
	consents, authorizer, sink := &consentStore{}, &authorizerDouble{}, &auditSink{}

	if _, err := newWithdrawal(consents, authorizer, sink).
		Execute(context.Background(), actor(), WithdrawCommand{Purpose: "  "}); err == nil {
		t.Fatal("a withdrawal with no purpose was recorded")
	}
	if len(consents.stored) != 0 || len(sink.entries) != 0 {
		t.Error("a refused withdrawal left something behind")
	}
}

func TestTheConsentDescriptorTakesWhatTheControllerSends(t *testing.T) {
	descriptor := WithdrawConsent{}.Descriptor()

	if err := descriptor.ValidateInput(usecase.Input{
		"purpose": "ai_processing", "account_id": subjectID.String(), "reason": "Objection",
	}); err != nil {
		t.Fatalf("the input the REST controller builds is refused: %v", err)
	}
	if descriptor.ReadOnly || !descriptor.Audit.Required {
		t.Error("the withdrawal is declared as a read, or without its audit obligation")
	}
}

// The handler is what the three channels call.
func TestTheHandlerRecordsTheWithdrawal(t *testing.T) {
	consents, authorizer, sink := &consentStore{endings: 2}, &authorizerDouble{}, &auditSink{}

	out, err := newWithdrawal(consents, authorizer, sink).Descriptor().Handler.
		Invoke(context.Background(), actor(), usecase.Input{
			"purpose": "email_content", "reason": "By email",
		})
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out.String("purpose") != "email_content" || out["granted"] != false {
		t.Errorf("the record came back as %v", out)
	}
	if _, revoked := out["revoked_at"]; !revoked {
		t.Errorf("the record does not say when the processing stopped: %v", out)
	}
}
