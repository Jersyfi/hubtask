// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	automationservice "github.com/Jersyfi/hubtask/core/application/service/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	recurrenceadapter "github.com/Jersyfi/hubtask/infrastructure/recurrence"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// workspaceRecoverer is the machine's use case, as the command needs it.
type workspaceRecoverer interface {
	ResumeBySlug(ctx context.Context, slug string) error
	CancelDeletionBySlug(ctx context.Context, slug string) (domain.TenantStatus, error)
}

// runWorkspaceCommand is `hubtask workspace resume|cancel-deletion --workspace <short name>`
// (UC-INS-03): the rest of the way back, for a workspace whose every operator its own state locks
// out - a single-mode installation's only workspace, suspended or set for deletion. Like
// `operator add` it needs only the server's configuration, and there is no route for it.
func runWorkspaceCommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := envadapter.New(version, commit).Load()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid configuration: %v\n", err)
		return 1
	}
	renderer, err := i18n.NewRendererFromConfig(cfg.Locale)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "message catalogue: %v\n", err)
		return 1
	}
	render := func(code string, params map[string]string) string {
		return renderer.Render(cfg.Locale.DefaultLocale, code, params)
	}

	ctx, cancel := context.WithTimeout(context.Background(), operatorCommandTimeout)
	defer cancel()
	return workspaceCommand(ctx, args, stdout, stderr, render, func(ctx context.Context) (workspaceRecoverer, func(), error) {
		pool, err := postgres.NewPool(ctx, cfg, envport.RoleAPI)
		if err != nil {
			return nil, nil, err
		}
		return machineRecoverer(postgres.NewUnitOfWork(pool), cfg.SecretKey), pool.Close, nil
	})
}

// machineRecoverer wires the two use cases as the server wires them for the control plane, so the
// machine's resume and cancel are the API's.
func machineRecoverer(uow persistence.UnitOfWork, secretKey secret.Secret) adminservice.RecoverWorkspace {
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	tenants := postgres.NewAdminTenantRepository()
	journal := postgres.NewInstanceJournal(security.NewCursorCodec(secretKey))
	sink := postgres.NewAuditSink(ids).Keyed(security.NewAuditFingerprinter(secretKey))
	return adminservice.RecoverWorkspace{
		Tenants: tenants, UnitOfWork: uow,
		Resume: adminservice.ResumeTenant{LifecycleShift: adminservice.LifecycleShift{
			Tenants: tenants, Journal: journal, Audit: sink, UnitOfWork: uow,
			Clock: clockadapter.System{}, IDs: ids,
		}},
		Cancel: adminservice.CancelTenantDeletion{
			Tenants: tenants, Journal: journal, Audit: sink, UnitOfWork: uow,
			Automations: automationservice.PausedRules{
				Paused: postgres.NewPausedAutomationRules(), Expander: recurrenceadapter.New(),
				Jobs: postgres.NewQueue(ids, clockadapter.System{}),
			},
			Clock: clockadapter.System{}, IDs: ids,
		},
	}
}

// workspaceCommand parses, connects and acts. Exit codes as `operator add`'s: 0 done (also when
// the workspace already stood where it was asked to), 1 refused or failed, 2 used wrongly.
func workspaceCommand(
	ctx context.Context, args []string, stdout, stderr io.Writer,
	render func(code string, params map[string]string) string,
	connect func(context.Context) (workspaceRecoverer, func(), error),
) int {
	if len(args) == 0 || (args[0] != "resume" && args[0] != "cancel-deletion") {
		_, _ = fmt.Fprintln(stderr, render("cli.workspace_usage", nil))
		return 2
	}
	verb := args[0]
	flags := flag.NewFlagSet("hubtask workspace "+verb, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	workspace := flags.String("workspace", "", "")
	if err := flags.Parse(args[1:]); err != nil || *workspace == "" || flags.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, render("cli.workspace_usage", nil))
		return 2
	}

	recoverer, closeConnection, err := connect(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, render("cli.operator_no_database", nil))
		return 1
	}
	defer closeConnection()

	params := map[string]string{"workspace": *workspace}
	refused := func(err error) int {
		code := shared.AsError(err).DetailCode
		if code == "" {
			code = "cli.workspace_failed"
		}
		_, _ = fmt.Fprintln(stderr, render(code, params))
		if verb == "resume" && code == "admin.tenant_leaving" {
			_, _ = fmt.Fprintln(stderr, render("cli.workspace_cancel_first", params))
		}
		return 1
	}

	if verb == "resume" {
		if err := recoverer.ResumeBySlug(ctx, *workspace); err != nil {
			return refused(err)
		}
		_, _ = fmt.Fprintln(stdout, render("cli.workspace_resumed", params))
		return 0
	}
	status, err := recoverer.CancelDeletionBySlug(ctx, *workspace)
	if err != nil {
		return refused(err)
	}
	if status == domain.TenantActive {
		_, _ = fmt.Fprintln(stdout, render("cli.workspace_deletion_off_active", params))
	} else {
		_, _ = fmt.Fprintln(stdout, render("cli.workspace_deletion_off_suspended", params))
	}
	return 0
}
