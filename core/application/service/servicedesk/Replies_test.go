// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/text"
)

type replyStore struct {
	rows map[string]domain.ReplyTemplate
}

func (s *replyStore) Insert(_ context.Context, t domain.ReplyTemplate) error {
	s.rows[t.Key] = t
	return nil
}
func (s *replyStore) Update(_ context.Context, t domain.ReplyTemplate, _ int) error {
	t.Version++
	s.rows[t.Key] = t
	return nil
}
func (s *replyStore) Find(_ context.Context, key string) (domain.ReplyTemplate, error) {
	t, ok := s.rows[key]
	if !ok {
		return domain.ReplyTemplate{}, shared.ErrNotFound
	}
	return t, nil
}
func (s *replyStore) List(context.Context) ([]domain.ReplyTemplate, error) { return nil, nil }
func (s *replyStore) Delete(_ context.Context, key string) error {
	delete(s.rows, key)
	return nil
}

type accountStore struct{ identityrepo.Accounts }

func (accountStore) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	if id != requesterID {
		return identity.Account{}, shared.ErrNotFound
	}
	return identity.Account{ID: id, DisplayName: "Mia Berger"}, nil
}

// UC-SVC-06 checks 1 and 2: a template naming an unknown placeholder is refused, and a known one
// renders with the entry's title, the requester's and the handler's names.
func TestACannedReplyIsFilledForTheEntry(t *testing.T) {
	f := newFixture(t)
	f.items.item.Title = "VPN drops every hour"
	store := &replyStore{rows: map[string]domain.ReplyTemplate{}}
	authorizer := &gate{allow: true}
	desk := Desk{
		Replies: store, Authorizer: authorizer, UnitOfWork: inline{}, Clock: at(monday),
		IDs: f.ids, Text: text.Composing{}, Audit: discard{},
	}
	define := DefineReplyTemplate{Desk: desk}
	if _, err := define.Execute(context.Background(), reader(), DefineReplyTemplateCommand{
		Key: "vpn", Name: "VPN", Body: "Hi {{requester.name}}, {{entry.secret}}",
	}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("an unknown placeholder: %v", err)
	}
	if _, err := define.Execute(context.Background(), reader(), DefineReplyTemplateCommand{
		Key: "vpn", Name: "VPN", Body: "Hi {{requester.name}}, about {{entry.title}}. {{agent.name}}",
	}); err != nil {
		t.Fatal(err)
	}
	if asked := authorizer.asked[len(authorizer.asked)-1]; asked.Permission != service.PermissionStructure {
		t.Errorf("defining asked %s, want STRUCTURE", asked.Permission)
	}

	actor := reader()
	actor.AccountName = "Jonas"
	render := RenderReplyTemplate{Desk: desk, Items: f.items, Containers: containerStore{}, Accounts: accountStore{}}
	body, err := render.Execute(context.Background(), actor, itemID, "vpn")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Hi Mia Berger, about VPN drops every hour. Jonas"; body != want {
		t.Errorf("rendered %q, want %q", body, want)
	}
	if asked := authorizer.asked[len(authorizer.asked)-1]; asked.Permission != service.PermissionRead || asked.TargetID != itemID {
		t.Errorf("rendering asked %+v, want READ on the entry", asked)
	}
	authorizer.allow = false
	if _, err := render.Execute(context.Background(), actor, itemID, "vpn"); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who may not read the entry rendered: %v", err)
	}
}
