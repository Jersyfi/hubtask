// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"testing"
	"time"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// Every connection carries its role's statement budget and name, and runs without JIT: a
// compilation longer than the statement is a tail no request should pay.
func TestEveryConnectionRunsBoundedNamedAndWithoutJIT(t *testing.T) {
	params := map[string]string{}
	sessionParams(params, 5*time.Second, env.RoleAPI)

	want := map[string]string{"statement_timeout": "5000", "application_name": "hubtask/api", "jit": "off"}
	for key, value := range want {
		if params[key] != value {
			t.Errorf("%s = %q, want %q", key, params[key], value)
		}
	}
}
