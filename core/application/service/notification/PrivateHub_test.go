// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// hiddenTo answers which accounts the entry's private hub is hidden from.
type hiddenTo map[shared.ID]bool

func (h hiddenTo) Hidden(_ context.Context, actor appshared.ActorContext, _ []identity.Scope) (bool, error) {
	return h[actor.AccountID], nil
}

// D6: a recipient who does not reach the private hub the entry is in - a former assignee, somebody
// left on its member list after leaving the hub - gets no record and no mail.
func TestNobodyOutsideAPrivateHubIsToldAboutItsEntries(t *testing.T) {
	fixture := consumer(t, bert, carla)
	fixture.consumer.Reach = hiddenTo{carla: true}

	if err := fixture.consumer.Deliver(t.Context(), commented(t, anna)); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	for _, record := range fixture.notifications.written() {
		if record.RecipientID == carla {
			t.Errorf("somebody outside the private hub has a record: %+v", record)
		}
	}
	told := false
	for _, record := range fixture.notifications.written() {
		told = told || record.RecipientID == bert
	}
	if !told {
		t.Error("the assignee inside the hub was not told")
	}
}

// The same for the retention warning: a workspace administrator outside the hub is not warned
// about an entry in it.
func TestARetentionWarningSkipsAnAdministratorOutsideThePrivateHub(t *testing.T) {
	service, notifications, _, _ := warningRecorder(
		[]shared.ID{anna},
		map[string][]shared.ID{"path": {bert}, "tenant": {carla}},
	)
	service.Reach = hiddenTo{carla: true}

	if err := service.Warn(t.Context(), warningFor(
		lifecycle.RecipientItemMembers, lifecycle.RecipientCollectionAdmins, lifecycle.RecipientTenantAdmins,
	)); err != nil {
		t.Fatalf("warning: %v", err)
	}
	for _, record := range notifications.written() {
		if record.RecipientID == carla {
			t.Error("the workspace's administrator outside the hub was warned")
		}
	}
	if len(notifications.written()) != 2 {
		t.Errorf("%d records, want the two inside the hub", len(notifications.written()))
	}
}
