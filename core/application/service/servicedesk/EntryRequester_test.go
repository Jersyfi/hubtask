// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// UC-SVC-03 check 7: a handler reads who raised the request and the organisation of their mail
// domain, never the address; somebody who may not read the entry is told it does not exist.
func TestARequestsReaderReadsItsRequesterAndTheirOrganisation(t *testing.T) {
	authorizer := &gate{allow: true}
	items := &itemStore{item: work.WorkItem{ID: itemID, TenantID: tenantID, CollectionID: itemID, CreatedBy: requesterID}}
	read := ReadEntryRequester{
		Desk: Desk{
			Authorizer: authorizer, UnitOfWork: inline{},
			Organisations: &organisationStore{rows: map[string]domain.Organisation{
				"acme": {Key: "acme", Name: "Acme", Domains: []string{"acme.example"}},
			}},
		},
		Items: items, Containers: containerStore{},
		Accounts: addressBook{emails: map[shared.ID]string{requesterID: "kim@acme.example"}},
	}

	found, err := read.Execute(context.Background(), reader(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	if found.AccountID != requesterID || found.Organisation == nil || found.Organisation.Key != "acme" {
		t.Errorf("read %+v, want the requester of Acme", found)
	}
	if asked := authorizer.asked[0]; asked.Permission != service.PermissionRead || asked.On.ID != itemID {
		t.Errorf("asked %+v, want READ on the entry", asked)
	}

	read.Accounts = addressBook{emails: map[shared.ID]string{requesterID: "kim@elsewhere.example"}}
	if found, err := read.Execute(context.Background(), reader(), itemID); err != nil || found.Organisation != nil {
		t.Errorf("a requester of no organisation read %+v, %v", found, err)
	}

	authorizer.allow = false
	if _, err := read.Execute(context.Background(), reader(), itemID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("somebody who may not read the entry got %v", err)
	}
}
