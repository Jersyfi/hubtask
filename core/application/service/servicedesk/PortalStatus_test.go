// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"testing"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// deskWorkflows answers one workflow the desk runs for every type.
type deskWorkflows struct {
	workrepo.Workflows
	desk shared.ID
}

func (deskWorkflows) List(context.Context) ([]work.Workflow, error) {
	return []work.Workflow{{Key: "desk", Published: work.WorkflowShape{
		Statuses: []work.WorkflowStatus{
			{Key: "new", Name: "Neu", Category: work.CategoryTodo},
			{Key: "waiting_for_customer", Name: "Wartet auf Sie", Category: work.CategoryInProgress},
			{Key: "resolved", Name: "Gelöst", Category: work.CategoryDone},
		},
		Initial: "new",
	}}}, nil
}

func (d deskWorkflows) Schemes(context.Context) ([]work.WorkflowScheme, error) {
	return []work.WorkflowScheme{{ContainerID: d.desk, WorkflowKey: "desk"}}, nil
}

// UC-SVC-05 check 3 and P-12: a request carries its status as the desk's workflow names it, with
// its category, never only the key; where no workflow runs, neither.
func TestTheRequesterReadsTheStatusByItsName(t *testing.T) {
	desk := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007c1")
	elsewhere := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007c2")
	waiting := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007c3")
	plain := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007c4")
	portal := Portal{
		Items: portalItems{rows: map[shared.ID]work.WorkItem{
			waiting: {ID: waiting, CreatedBy: requesterID, CollectionID: desk, Title: "Drucker", Status: "waiting_for_customer"},
			plain:   {ID: plain, CreatedBy: requesterID, CollectionID: elsewhere, Title: "Toner"},
		}},
		Containers: containerStore{}, Comments: portalComments{},
		Workflow: workservice.WorkflowGate{Workflows: deskWorkflows{desk: desk}},
		Desk:     Desk{Records: &recordStore{}, Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)},
	}
	read := func(id shared.ID) usecase.Output {
		t.Helper()
		out, err := ReadMyRequest{Portal: portal}.Descriptor().Handler.Invoke(context.Background(), requester(), usecase.Input{"item_id": id.String()})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := read(waiting); out["status"] != "waiting_for_customer" || out["status_name"] != "Wartet auf Sie" ||
		out["status_category"] != "IN_PROGRESS" {
		t.Errorf("the request reads %v / %v / %v", out["status"], out["status_name"], out["status_category"])
	}
	if out := read(plain); out["status_name"] != nil || out["status_category"] != nil {
		t.Errorf("a request without a workflow reads %v / %v", out["status_name"], out["status_category"])
	}
}

// UC-SVC-05 check 10 and P-12: the request a requester's own was merged into is named as its
// workflow names it, as long as the reader may read it at all.
func TestTheSurvivingRequestIsNamedByItsStatus(t *testing.T) {
	desk := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007d1")
	merged := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007d2")
	survivor := shared.MustParseID("01936f2a-7c1e-7000-8000-0000000007d3")
	portal := Portal{
		Items: portalItems{rows: map[shared.ID]work.WorkItem{
			merged:   {ID: merged, CreatedBy: requesterID, CollectionID: desk, Title: "Drucker"},
			survivor: {ID: survivor, CreatedBy: colleagueID, CollectionID: desk, Title: "Drucker 2. OG", Status: "waiting_for_customer"},
		}},
		Containers: containerStore{}, Comments: portalComments{},
		Links:    portalLinks{{Type: work.LinkTypeDuplicates, SourceID: merged, TargetID: survivor}},
		Workflow: workservice.WorkflowGate{Workflows: deskWorkflows{desk: desk}},
		Desk:     Desk{Records: &recordStore{}, Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)},
	}
	out, err := ReadMyRequest{Portal: portal}.Descriptor().Handler.Invoke(context.Background(), requester(), usecase.Input{"item_id": merged.String()})
	if err != nil {
		t.Fatal(err)
	}
	into, _ := out["merged_into"].(usecase.Output)
	if into == nil || into["status_name"] != "Wartet auf Sie" || into["status_category"] != "IN_PROGRESS" {
		t.Errorf("the surviving request reads %+v", into)
	}
}
