// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"testing"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Every connection carries its role's statement budget and name, runs without JIT - a compilation
// longer than the statement is a tail no request should pay - and plans each statement for its own
// arguments: a generic plan of an optional argument read a 300 000-entry level whole for a page of
// fifty (capacity-10000000-2026-11-01.md).
func TestEveryConnectionRunsBoundedNamedAndWithoutJIT(t *testing.T) {
	params := map[string]string{}
	sessionParams(params, 5*time.Second, env.RoleAPI)

	want := map[string]string{"statement_timeout": "5000", "application_name": "hubtask/api", "jit": "off",
		"plan_cache_mode": "force_custom_plan",
		// A stalled transaction ends instead of pinning locks and the vacuum horizon for ever.
		"idle_in_transaction_session_timeout": "600000", "lock_timeout": "15000"}
	for key, value := range want {
		if params[key] != value {
			t.Errorf("%s = %q, want %q", key, params[key], value)
		}
	}
	worker := map[string]string{}
	sessionParams(worker, time.Minute, env.RoleWorker)
	if worker["idle_in_transaction_session_timeout"] != "1800000" || worker["lock_timeout"] != "60000" {
		t.Errorf("the worker runs with %q idle and %q lock bounds", worker["idle_in_transaction_session_timeout"], worker["lock_timeout"])
	}
}
