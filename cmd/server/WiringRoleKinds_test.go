// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"slices"
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	queueport "github.com/Jersyfi/hubtask/core/port/queue"
)

// The automation role is a bulkhead, not a name (deployment.md §2.2): its pods alone claim the
// rule jobs, a worker beside it claims every other kind, and one process holding both claims all.
func TestEachRoleClaimsItsOwnKinds(t *testing.T) {
	rules := queueport.AutomationKinds()
	cases := []struct {
		name         string
		roles        []envport.Role
		only, except []queueport.Kind
	}{
		{"worker", []envport.Role{envport.RoleWorker}, nil, rules},
		{"automation", []envport.Role{envport.RoleAutomation}, rules, nil},
		{"both", []envport.Role{envport.RoleWorker, envport.RoleAutomation}, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			only, except := claimedKinds(envport.Config{Roles: c.roles})
			if !slices.Equal(only, c.only) || !slices.Equal(except, c.except) {
				t.Errorf("claims only %v except %v, want only %v except %v", only, except, c.only, c.except)
			}
		})
	}
	if !runsBackgroundWork(envport.Config{Roles: []envport.Role{envport.RoleAPI, envport.RoleAutomation}}) {
		t.Error("an API process with the automation role gets no pool of its own for its runner")
	}
}
