// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

type recovererFake struct {
	status domain.TenantStatus
	err    error
	asked  []string
}

func (r *recovererFake) ResumeBySlug(_ context.Context, slug string) error {
	r.asked = append(r.asked, "resume "+slug)
	return r.err
}

func (r *recovererFake) CancelDeletionBySlug(_ context.Context, slug string) (domain.TenantStatus, error) {
	r.asked = append(r.asked, "cancel "+slug)
	return r.status, r.err
}

// The rest of UC-INS-03's way back: the command names the workspace by its short name, says where
// it stands afterwards and what to run next, passes a refusal on in its own sentence, and tells
// wrong use apart by its exit code.
func TestTheWorkspaceCommand(t *testing.T) {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("the catalogue: %v", err)
	}
	render := func(code string, params map[string]string) string { return renderer.Render("en", code, params) }

	cases := []struct {
		name     string
		args     []string
		fake     *recovererFake
		connErr  error
		exit     int
		asked    string
		out, err []string
	}{
		{
			name: "resumed", args: []string{"resume", "--workspace", "demo"}, fake: &recovererFake{},
			exit: 0, asked: "resume demo", out: []string{"demo is active", "from the machine"},
		},
		{
			name: "called off, back active", args: []string{"cancel-deletion", "--workspace", "demo"},
			fake: &recovererFake{status: domain.TenantActive}, exit: 0, asked: "cancel demo",
			out: []string{"no longer set for deletion and is active"},
		},
		{
			name: "called off, back suspended", args: []string{"cancel-deletion", "--workspace", "demo"},
			fake: &recovererFake{status: domain.TenantSuspended}, exit: 0, asked: "cancel demo",
			out: []string{"is suspended", "hubtask workspace resume --workspace demo"},
		},
		{
			name: "resume while pending", args: []string{"resume", "--workspace", "demo"},
			fake: &recovererFake{err: shared.ErrConflict.WithDetail("admin.tenant_leaving")},
			exit: 1, asked: "resume demo",
			err: []string{"pending deletion", "hubtask workspace cancel-deletion --workspace demo"},
		},
		{
			name: "purge begun", args: []string{"cancel-deletion", "--workspace", "demo"},
			fake: &recovererFake{err: shared.ErrConflict.WithDetail("admin.tenant_deletion_underway")},
			exit: 1, asked: "cancel demo", err: []string{"can no longer be called off"},
		},
		{
			name: "no such workspace", args: []string{"resume", "--workspace", "nope"},
			fake: &recovererFake{err: shared.ErrNotFound.WithDetail("admin.tenant_not_found")},
			exit: 1, asked: "resume nope", err: []string{"There is no such workspace."},
		},
		{
			name: "failed", args: []string{"resume", "--workspace", "demo"},
			fake: &recovererFake{err: errors.New("boom")}, exit: 1, asked: "resume demo",
			err: []string{"could not be changed"},
		},
		{name: "no subcommand", args: nil, exit: 2, err: []string{"Usage: hubtask workspace resume"}},
		{name: "unknown subcommand", args: []string{"suspend", "--workspace", "demo"}, exit: 2, err: []string{"Usage:"}},
		{name: "no workspace", args: []string{"resume"}, exit: 2, err: []string{"Usage:"}},
		{name: "extra argument", args: []string{"resume", "--workspace", "demo", "now"}, exit: 2, err: []string{"Usage:"}},
		{
			name: "no database", args: []string{"cancel-deletion", "--workspace", "demo"},
			connErr: errors.New("refused"), exit: 1, err: []string{"The database could not be reached"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			fake := tc.fake
			if fake == nil {
				fake = &recovererFake{}
			}
			exit := workspaceCommand(t.Context(), tc.args, &stdout, &stderr, render,
				func(context.Context) (workspaceRecoverer, func(), error) {
					if tc.connErr != nil {
						return nil, nil, tc.connErr
					}
					return fake, func() {}, nil
				})
			if exit != tc.exit {
				t.Errorf("exit %d, want %d (stderr %q)", exit, tc.exit, stderr.String())
			}
			for _, want := range tc.out {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			for _, want := range tc.err {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want %q", stderr.String(), want)
				}
			}
			if got := strings.Join(fake.asked, ","); got != tc.asked {
				t.Errorf("asked %q, want %q", got, tc.asked)
			}
		})
	}
}

// The machine's two use cases are wired whole: a cancel without its automation switch refuses
// (admin.automations_not_wired), and one without its trail or journal could not record the act.
func TestTheMachineRecovererIsWiredWhole(t *testing.T) {
	r := machineRecoverer(postgres.NewUnitOfWork(nil), secret.New("0123456789abcdef0123456789abcdef"))
	resume, cancel := r.Resume.LifecycleShift, r.Cancel
	if r.Tenants == nil || r.UnitOfWork == nil ||
		resume.Tenants == nil || resume.Journal == nil || resume.Audit == nil || resume.UnitOfWork == nil ||
		resume.Clock == nil || resume.IDs == nil ||
		cancel.Tenants == nil || cancel.Journal == nil || cancel.Audit == nil || cancel.UnitOfWork == nil ||
		cancel.Clock == nil || cancel.IDs == nil || cancel.Automations == nil {
		t.Errorf("a field is not wired: %+v", r)
	}
}
