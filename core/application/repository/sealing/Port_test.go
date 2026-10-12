// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing_test

import (
	"reflect"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/repository/sealing"
)

// The census the operator reads is the sum over the levels read one at a time, and a key one
// workspace names and another does not must count once, not vanish and not double.
func TestTheInstallationsCensusIsTheSumOfTheLevels(t *testing.T) {
	first := sealing.Counts{}
	first.Add("k1", sealing.StoreWebhookSubscription, 3)
	first.Add("gone", sealing.StoreAccountMfa, 1)
	second := sealing.Counts{}
	second.Add("k1", sealing.StoreWebhookSubscription, 2)
	second.Add("k1", sealing.StoreIdentityProvider, 1)
	second.Add("k2", sealing.StoreAiProvider, 5)
	second.Add("k2", sealing.StoreBackupTarget, 0)

	total := sealing.Sum(first, second, nil)
	if want := map[string]int64{"k1": 6, "k2": 5, "gone": 1}; !reflect.DeepEqual(total.ByKey(), want) {
		t.Errorf("summed %v, want %v", total.ByKey(), want)
	}
	want := []sealing.StoreCount{
		{Store: sealing.StoreIdentityProvider, Count: 1},
		{Store: sealing.StoreWebhookSubscription, Count: 5},
	}
	if got := total.Stores("k1"); !reflect.DeepEqual(got, want) {
		t.Errorf("k1 by store %v, want %v", got, want)
	}
	if got := total.Stores("k2"); len(got) != 1 {
		t.Errorf("a store with nothing under the key is listed: %v", got)
	}
	if empty := sealing.Sum(); empty == nil || len(empty) != 0 {
		t.Errorf("nothing summed to %v, want an empty census", empty)
	}
}
