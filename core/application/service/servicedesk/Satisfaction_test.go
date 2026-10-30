// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

type ratingStore struct{ rows map[shared.ID]domain.Rating }

func (s *ratingStore) Save(_ context.Context, r domain.Rating) (domain.Rating, error) {
	if held, ok := s.rows[r.ItemID]; ok {
		r.ID, r.Version = held.ID, held.Version+1
	}
	s.rows[r.ItemID] = r
	return r, nil
}

func (s *ratingStore) OfItem(_ context.Context, id shared.ID) (domain.Rating, error) {
	r, ok := s.rows[id]
	if !ok {
		return domain.Rating{}, shared.ErrNotFound
	}
	return r, nil
}

type invocations struct{ calls []usecase.Input }

func (i *invocations) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if name != addCommentUseCase || actor.Channel != string(work.ChannelPortal) {
		return nil, errors.New("not through AddComment on the PORTAL door")
	}
	i.calls = append(i.calls, in)
	return usecase.Output{}, nil
}

func requester() appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenantID, AccountID: requesterID, Kind: appshared.ActorUser, Scopes: []string{deskRead, "comments:write"}}
}

// UC-SVC-07 checks 1 to 3: the requester rates their own resolved request, with words that become
// their comment; an open request and anybody else's request are refused; rating again replaces.
func TestTheRequesterRatesTheirResolvedRequest(t *testing.T) {
	f := newFixture(t)
	ratings := &ratingStore{rows: map[shared.ID]domain.Rating{}}
	comments := &invocations{}
	authorizer := &gate{allow: true}
	rate := RateMyRequest{Portal: Portal{
		Entries: comments, Items: f.items, Containers: containerStore{},
		Desk: Desk{Ratings: ratings, Authorizer: authorizer, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: f.ids},
	}}

	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 4}); !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an open request was rated: %v", err)
	}
	f.items.item.Completion.IsCompleted = true
	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 2, Comment: "Took long."}); err != nil {
		t.Fatal(err)
	}
	if asked := authorizer.asked[len(authorizer.asked)-1]; asked.Permission != service.PermissionRequest {
		t.Errorf("asked %s, want REQUEST", asked.Permission)
	}
	if len(comments.calls) != 1 || comments.calls[0]["body"] != "Took long." {
		t.Errorf("the words went %+v", comments.calls)
	}
	again, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 5})
	if err != nil || again.Score != 5 || again.Version != 2 || len(comments.calls) != 1 {
		t.Errorf("rating again: %+v %v", again, err)
	}
	if _, err := rate.Execute(context.Background(), requester(), RateMyRequestCommand{ItemID: itemID, Score: 9}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a score off the scale: %v", err)
	}
	if _, err := rate.Execute(context.Background(), reader(), RateMyRequestCommand{ItemID: itemID, Score: 3}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody else rated the request: %v", err)
	}
}
