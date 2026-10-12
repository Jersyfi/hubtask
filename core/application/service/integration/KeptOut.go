// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// KeptOutSource answers the people one reason keeps out of AI, inside the caller's transaction.
// The objection is one source; a restriction (Art. 18) is another, added beside it rather than
// as a second mechanism.
type KeptOutSource interface {
	KeptOut(ctx context.Context, tenantID shared.ID) ([]domain.KeptPerson, error)
}

// Exclusion reads every source into one KeptOut - what every path that hands content to a model
// asks, in the transaction it reads the material in.
type Exclusion struct {
	Sources []KeptOutSource
}

// Read asks every source. A failing source fails the read: the content of somebody who objected
// must not reach a model because a query could not say that they did.
func (e Exclusion) Read(ctx context.Context, tenantID shared.ID) (domain.KeptOut, error) {
	var people []domain.KeptPerson
	for _, source := range e.Sources {
		kept, err := source.KeptOut(ctx, tenantID)
		if err != nil {
			return domain.KeptOut{}, err
		}
		people = append(people, kept...)
	}
	return domain.NewKeptOut(people), nil
}

// RestrictionSource keeps out the people whose processing is restricted (Art. 18,
// data-protection.md §4 and §4.1): no gate - unlike an objection it holds whatever the workspace's
// position, alone or not, because it is the controller's duty rather than a choice the workspace
// can take away. Placed first in the list, so that somebody both restricted and objecting reads as
// restricted and is kept out as far as a restriction reaches.
type RestrictionSource struct {
	Restrictions privacyrepo.Restrictions
}

// KeptOut answers the restricted people.
func (s RestrictionSource) KeptOut(ctx context.Context, _ shared.ID) ([]domain.KeptPerson, error) {
	if s.Restrictions == nil {
		return nil, nil
	}
	restricted, err := s.Restrictions.RestrictedPeople(ctx)
	if err != nil {
		return nil, err
	}
	people := make([]domain.KeptPerson, 0, len(restricted))
	for _, person := range restricted {
		people = append(people, domain.KeptPerson{
			AccountID: person.AccountID, Reason: domain.KeptOutRestricted, Names: person.Names(),
		})
	}
	return people, nil
}

// ObjectionSource keeps out the people who objected, where their objection takes effect: the
// workspace offers keeping one's content out, and more than one person works here - alone, the
// workspace's decision is the person's own, and there is no switch to undo it (UC-PRV-05 check 8).
type ObjectionSource struct {
	Rules    *AiRules
	People   identityrepo.People
	Consents privacyrepo.Consents
}

// KeptOut answers the objecting people.
func (s ObjectionSource) KeptOut(ctx context.Context, tenantID shared.ID) ([]domain.KeptPerson, error) {
	takes, err := s.takes(ctx, tenantID)
	if err != nil || !takes {
		return nil, err
	}
	standing, err := s.Consents.Standing(ctx, privacydomain.PurposeAi)
	if err != nil {
		return nil, err
	}
	people := make([]domain.KeptPerson, 0, len(standing))
	for _, objector := range standing {
		people = append(people, domain.KeptPerson{
			AccountID: objector.AccountID, Reason: domain.KeptOutObjected,
			Names: []string{objector.DisplayName, objector.SignInName, objector.Email},
		})
	}
	return people, nil
}

// takes reports whether an objection takes effect in this workspace now.
func (s ObjectionSource) takes(ctx context.Context, tenantID shared.ID) (bool, error) {
	if s.Rules == nil || s.People == nil || s.Consents == nil {
		return false, nil
	}
	state, err := s.Rules.Resolve(ctx, tenantID)
	if err != nil || !state.Rule.OptOutOffered() {
		return false, err
	}
	people, err := s.People.CountPeople(ctx)
	return people > 1, err
}
