// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

type restorerFake struct {
	restored adminservice.RestoredOperator
	err      error
	asked    [][2]string
}

func (r *restorerFake) Execute(_ context.Context, slug, email string) (adminservice.RestoredOperator, error) {
	r.asked = append(r.asked, [2]string{slug, email})
	return r.restored, r.err
}

// UC-INS-03 checks 1-3: the command names the account by workspace and address, prints what it did,
// refuses an unknown pair with the one sentence, and tells wrong use apart by its exit code.
func TestTheOperatorCommand(t *testing.T) {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("the catalogue: %v", err)
	}
	render := func(code string, params map[string]string) string { return renderer.Render("en", code, params) }

	cases := []struct {
		name     string
		args     []string
		restorer *restorerFake
		connErr  error
		exit     int
		out, err string
	}{
		{
			name: "added", args: []string{"add", "--workspace", "demo", "--email", "jerome@example.eu"},
			restorer: &restorerFake{restored: adminservice.RestoredOperator{Added: true}}, exit: 0,
			out: "jerome@example.eu in demo is now an operator",
		},
		{
			name: "already one", args: []string{"add", "--workspace", "demo", "--email", "jerome@example.eu"},
			restorer: &restorerFake{}, exit: 0, out: "already is an operator",
		},
		{
			name: "no such account", args: []string{"add", "--workspace", "demo", "--email", "nobody@example.eu"},
			restorer: &restorerFake{err: shared.ErrValidation.WithDetail("admin.operator_unknown_account")},
			exit:     1, err: "There is no such account on this installation.",
		},
		{name: "no subcommand", args: nil, exit: 2, err: "Usage: hubtask operator add"},
		{name: "no address", args: []string{"add", "--workspace", "demo"}, exit: 2, err: "Usage:"},
		{
			name: "no database", args: []string{"add", "--workspace", "demo", "--email", "a@b.eu"},
			connErr: errors.New("refused"), exit: 1, err: "The database could not be reached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			restorer := tc.restorer
			if restorer == nil {
				restorer = &restorerFake{}
			}
			exit := operatorCommand(t.Context(), tc.args, &stdout, &stderr, render,
				func(context.Context) (operatorRestorer, func(), error) {
					if tc.connErr != nil {
						return nil, nil, tc.connErr
					}
					return restorer, func() {}, nil
				})
			if exit != tc.exit {
				t.Errorf("exit %d, want %d (stderr %q)", exit, tc.exit, stderr.String())
			}
			if tc.out != "" && !strings.Contains(stdout.String(), tc.out) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.out)
			}
			if tc.err != "" && !strings.Contains(stderr.String(), tc.err) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.err)
			}
			if tc.exit == 0 && (len(restorer.asked) != 1 || restorer.asked[0][0] != "demo") {
				t.Errorf("asked %v", restorer.asked)
			}
		})
	}
}
