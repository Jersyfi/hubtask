// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// A person lets their own content back into AI; nobody agrees in another's place (P-14,
// UC-PRV-05 checks 4 and 7).

type seeds struct{ count int }

func (s *seeds) Reseed(context.Context, shared.ID) error {
	s.count++
	return nil
}

func newGiving(consents *consentStore, sink *auditSink, reindex Reindexer) GiveConsent {
	return GiveConsent{
		Consents: consents, Audit: sink, UnitOfWork: &unitOfWork{},
		Clock: clock.Fixed(now), IDs: &idSource{}, Reindex: reindex,
	}
}

func TestAPersonLetsTheirOwnContentBackIn(t *testing.T) {
	consents, sink, reseeded := &consentStore{}, &auditSink{}, &seeds{}
	registry, err := usecase.NewRegistry(nil, newGiving(consents, sink, reseeded).Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	out, err := registry.Invoke(t.Context(), GiveConsentName, actor(), usecase.Input{"purpose": domain.PurposeAi})
	if err != nil {
		t.Fatalf("giving: %v", err)
	}
	if out["granted"] != true || out.String("account_id") != accountID.String() || out.String("source") != domain.SourceUser {
		t.Errorf("answered %v", out)
	}
	if len(consents.stored) != 1 || !consents.stored[0].InForce() {
		t.Errorf("stored %+v", consents.stored)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != ConsentGivenAction {
		t.Errorf("recorded %+v", sink.entries)
	}
	if reseeded.count != 1 {
		t.Errorf("the embedding pass was seeded %d times, want once", reseeded.count)
	}

	// Another purpose moves nothing about whose entries may be embedded.
	if _, err := registry.Invoke(t.Context(), GiveConsentName, actor(), usecase.Input{"purpose": "metering"}); err != nil {
		t.Fatalf("giving for metering: %v", err)
	}
	if reseeded.count != 1 {
		t.Errorf("a purpose other than AI seeded the embedding pass")
	}
}

func TestNobodyGivesConsentInAnothersPlace(t *testing.T) {
	consents, sink := &consentStore{}, &auditSink{}
	registry, err := usecase.NewRegistry(nil, newGiving(consents, sink, nil).Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	_, err = registry.Invoke(t.Context(), GiveConsentName, actor(), usecase.Input{
		"purpose": domain.PurposeAi, "account_id": subjectID.String(),
	})
	if got := shared.AsError(err); got == nil || got.DetailCode != domain.CodeConsentOwnOnly || got.Category != shared.CategoryForbidden {
		t.Fatalf("answered %v", err)
	}
	if len(consents.stored) != 0 || len(sink.entries) != 0 {
		t.Error("a refused consent left something behind")
	}
	// Naming oneself is the same as naming nobody.
	if _, err := registry.Invoke(t.Context(), GiveConsentName, actor(), usecase.Input{
		"purpose": domain.PurposeAi, "account_id": accountID.String(),
	}); err != nil {
		t.Errorf("one's own id refused: %v", err)
	}
}

// Withdrawing for AI seeds the embedding pass as well: the person's entries leave it.
func TestWithdrawingForAiSeedsTheEmbeddingPass(t *testing.T) {
	reseeded := &seeds{}
	withdrawal := newWithdrawal(&consentStore{}, &authorizerDouble{}, &auditSink{})
	withdrawal.Reindex = reseeded
	if _, err := withdrawal.Execute(t.Context(), actor(), WithdrawCommand{Purpose: domain.PurposeAi}); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, err := withdrawal.Execute(t.Context(), actor(), WithdrawCommand{Purpose: "metering"}); err != nil {
		t.Fatalf("withdrawing for metering: %v", err)
	}
	if reseeded.count != 1 {
		t.Errorf("seeded %d times, want once", reseeded.count)
	}
}
