// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
)

type people int

func (p people) CountPeople(context.Context) (int, error) { return int(p), nil }

// UC-ID-16 check 8 through the catalogue: a workspace of one person is offered no private hub, a
// workspace of two is, and a build that cannot count offers none.
func TestPrivateHubsAreOfferedOnlyWhereSomebodyElseIs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		people *people
		want   bool
	}{
		{"one person", ptr(people(1)), false},
		{"two people", ptr(people(2)), true},
		{"nobody counting", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newWorkspaceFixture(at())
			if tc.people != nil {
				f.writer.People = *tc.people
			}
			registry, err := usecase.NewRegistry(nil, ReadWorkspace{Writer: f.writer}.Descriptor())
			if err != nil {
				t.Fatal(err)
			}
			out, err := registry.Invoke(t.Context(), ReadWorkspaceName, workspaceActor(), usecase.Input{})
			if err != nil {
				t.Fatal(err)
			}
			if out["private_hubs_offered"] != tc.want {
				t.Errorf("private_hubs_offered = %v, want %v", out["private_hubs_offered"], tc.want)
			}
		})
	}
}

func ptr[T any](value T) *T { return &value }
