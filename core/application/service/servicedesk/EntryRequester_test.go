// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
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

type ownedCIs []cmdb.CI

func (o ownedCIs) CIsOwnedBy(_ context.Context, account shared.ID, limit int) ([]cmdb.CI, error) {
	var out []cmdb.CI
	for _, ci := range o {
		if ci.Compliance.OwnerAccount == account && len(out) < limit {
			out = append(out, ci)
		}
	}
	return out, nil
}

// assetsAt answers assets switched on at every hub but the one named.
type assetsAt struct{ off shared.ID }

func (m assetsAt) On(_ context.Context, _ appshared.ActorContext, hub shared.ID) (module.Set, error) {
	if hub == m.off {
		return module.Set{module.ServiceDesk}, nil
	}
	return module.Set{module.ServiceDesk, module.Assets}, nil
}

// UC-SVC-03 check 9: beside a request the handler sees the CIs its requester owns, only of the
// hubs where the handler may read CIs and assets are on.
func TestARequestsReaderSeesTheRequestersAssetsTheyMayRead(t *testing.T) {
	assetsOff := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005fb")
	owned := ownedCIs{
		{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a1"), HubID: hubID, Class: "laptop", Name: "Kim's laptop", Stage: cmdb.StageInUse, Compliance: cmdb.Compliance{OwnerAccount: requesterID}},
		{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a2"), HubID: otherDeskHub, Class: "laptop", Name: "Private", Stage: cmdb.StageInUse, Compliance: cmdb.Compliance{OwnerAccount: requesterID}},
		{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a3"), HubID: assetsOff, Class: "phone", Name: "Switched off", Stage: cmdb.StageInUse, Compliance: cmdb.Compliance{OwnerAccount: requesterID}},
		{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-0000000006a4"), HubID: hubID, Class: "laptop", Name: "Someone else's", Stage: cmdb.StageInUse, Compliance: cmdb.Compliance{OwnerAccount: handlerID}},
	}
	read := ReadEntryRequester{
		Desk:       Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Visibility: readsDeskHub{}},
		Items:      &itemStore{item: work.WorkItem{ID: itemID, TenantID: tenantID, CollectionID: itemID, CreatedBy: requesterID}},
		Containers: containerStore{}, Accounts: addressBook{emails: map[shared.ID]string{}},
		Assets: owned, Modules: assetsAt{off: assetsOff},
	}
	actor := reader()

	found, err := read.Execute(context.Background(), actor, itemID)
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Assets) != 1 || found.Assets[0].Name != "Kim's laptop" {
		t.Errorf("assets %+v, want the requester's laptop of the readable hub alone", found.Assets)
	}

	read.Assets = nil
	if found, err := read.Execute(context.Background(), actor, itemID); err != nil || found.Assets == nil || len(found.Assets) != 0 {
		t.Errorf("a build without the CMDB read %+v, %v; want an empty list", found.Assets, err)
	}
}

// UC-SVC-03 checks 7 and 9, UC-SVC-11 *Where it ends*: a request the mail intake filed for a
// requester without an account was raised by the administrator who opened the intake; beside it
// the handler reads no organisation and no configuration items, which would be the
// administrator's, never the requester's.
func TestARequestFiledByMailNamesNoOrganisationNorAssets(t *testing.T) {
	items := &itemStore{item: work.WorkItem{ID: itemID, TenantID: tenantID, CollectionID: itemID, CreatedBy: requesterID}}
	read := ReadEntryRequester{
		Desk: Desk{
			Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Visibility: readsDeskHub{},
			Organisations: &organisationStore{rows: map[string]domain.Organisation{
				"acme": {Key: "acme", Name: "Acme", Domains: []string{"acme.example"}},
			}},
		},
		Items: items, Containers: containerStore{},
		Accounts:   addressBook{emails: map[shared.ID]string{requesterID: "admin@acme.example"}},
		Assets:     ownedCIs{{ID: itemID, HubID: hubID, Name: "Admin laptop", Compliance: cmdb.Compliance{OwnerAccount: requesterID}}},
		Modules:    assetsAt{},
		Requesters: requesterBook{itemID: {{Address: "visitor@example.org"}}},
	}
	found, err := read.Execute(context.Background(), reader(), itemID)
	if err != nil {
		t.Fatal(err)
	}
	if found.Organisation != nil || len(found.Assets) != 0 {
		t.Errorf("a request filed by mail names %+v and %d assets", found.Organisation, len(found.Assets))
	}
}
