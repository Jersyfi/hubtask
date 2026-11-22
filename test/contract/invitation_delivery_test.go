// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// UC-ID-14 check 8 at the REST door: an invited person's row answered from the invitation record,
// past a newer mail of another kind, still is an InvitationDelivery - the decision added no field.

type deliveryJobs struct{ mails, invitations []repository.LastMail }

func (d deliveryJobs) Latest(context.Context, []shared.ID) ([]repository.InvitationJob, error) {
	return nil, nil
}

func (d deliveryJobs) LatestMail(context.Context, []shared.ID) ([]repository.LastMail, error) {
	return d.mails, nil
}

func (d deliveryJobs) LatestInvitationMail(context.Context, []shared.ID) ([]repository.LastMail, error) {
	return d.invitations, nil
}

func (d deliveryJobs) CancelOpen(context.Context, shared.ID, time.Time) (int, error) { return 0, nil }

// deliveryAccounts answers Find alone; the use case asks nothing else of the port.
type deliveryAccounts struct {
	repository.Accounts
	byID map[shared.ID]identity.Account
}

func (a deliveryAccounts) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	if account, ok := a.byID[id]; ok {
		return account, nil
	}
	return identity.Account{}, shared.ErrNotFound
}

type allowing struct{}

func (allowing) Authorize(context.Context, appshared.ActorContext, access.Request) error { return nil }

type passThrough struct{}

func (passThrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passThrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestAnInvitedPersonsDeliveryMatchesTheSchema(t *testing.T) {
	spec := contractSpec(t)
	invited := shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	member := shared.MustParseID("0192f000-0000-7000-8000-0000000000c2")
	at := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	jobs := deliveryJobs{
		mails: []repository.LastMail{
			{AccountID: invited, Category: "MEMBERSHIP", State: "SENT", RecordedAt: at.Add(time.Hour), SentAt: at.Add(time.Hour)},
			{AccountID: member, Category: "ASSIGNMENT", State: "FAILED", Reason: "mail.recipient_rejected", RecordedAt: at},
		},
		invitations: []repository.LastMail{
			{AccountID: invited, Category: "INVITATION", State: "PENDING", RecordedAt: at, DeliveryError: "mail.unavailable", DeliveryParams: map[string]string{"attempt": "2"}},
		},
	}
	accounts := deliveryAccounts{byID: map[shared.ID]identity.Account{
		invited: {ID: invited, Status: identity.AccountInvited},
		member:  {ID: member, Status: identity.AccountActive},
	}}
	registry, err := catalogue.NewRegistry(nil, identityservice.ReadInvitationDeliveries{
		Jobs: jobs, Accounts: accounts, Authorizer: allowing{}, UnitOfWork: passThrough{},
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	controller := rest.NewRestController()
	controller.UseCases = registry
	request := httptest.NewRequestWithContext(appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"members:read"},
	}), http.MethodPost, rest.APIBasePath+"/accounts:invitation-deliveries",
		strings.NewReader(`{"ids":["`+invited.String()+`","`+member.String()+`"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.Bytes())
	}

	var answer struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil || len(answer.Data) != 2 {
		t.Fatalf("answer %s (%v)", response.Body.Bytes(), err)
	}
	categories := map[string]bool{}
	for _, row := range answer.Data {
		problems, err := spec.validateAgainst("InvitationDelivery", row)
		if err != nil {
			t.Fatalf("%v", err)
		}
		for _, problem := range problems {
			t.Errorf("InvitationDelivery: %s", problem)
		}
		var fields struct {
			Category string `json:"category"`
		}
		_ = json.Unmarshal(row, &fields)
		categories[fields.Category] = true
	}
	if !categories["INVITATION"] || !categories["ASSIGNMENT"] {
		t.Errorf("the invited row is not the invitation's: %s", response.Body.Bytes())
	}
}
