// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/quota"
)

// The overrides carry §4's per-tenant rows, one pointer each - three states apiece: nil is the
// mode's default, a value is the workspace's own ceiling, 0 is a configured unlimited. The pin
// is the field list: a §4 row added to the engine without an override here would be a wall the
// operator cannot move, and this is the test that turns red first.
func TestTheOverridesNameEveryConfigurableRow(t *testing.T) {
	want := []string{
		"APIRequestsPerMinute", "AutomationRunsPerHour", "ExportJobs",
		"Items", "MediaBytes", "WebhookTargets",
	}

	shape := reflect.TypeOf(quota.Overrides{})
	got := make([]string, 0, shape.NumField())
	for i := 0; i < shape.NumField(); i++ {
		field := shape.Field(i)
		if field.Type.Kind() != reflect.Pointer || field.Type.Elem().Kind() != reflect.Int64 {
			t.Errorf("%s is %s - an override is a *int64, or it cannot say \"unset\"",
				field.Name, field.Type)
		}
		got = append(got, field.Name)
	}
	sort.Strings(got)

	if !reflect.DeepEqual(got, want) {
		t.Errorf("the overrides carry %v, §4 configures %v - change both together, and the "+
			"contract's TenantQuotas schema with them", got, want)
	}
}
