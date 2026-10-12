// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// operatorCommandTimeout bounds the command's whole run against the database (rule 7).
const operatorCommandTimeout = 30 * time.Second

// operatorRestorer is the machine's use case, as the command needs it.
type operatorRestorer interface {
	Execute(ctx context.Context, slug, email string) (adminservice.RestoredOperator, error)
}

// runOperatorCommand is `hubtask operator add --workspace <short name> --email <address>`
// (UC-INS-03): the way back into an installation nobody can run, from the machine it runs on. It
// needs nothing but the server's own configuration - the database connection the server already
// has - and there is no route for it: whoever can run this could reach the database anyway, and
// nobody else should be able to.
func runOperatorCommand(args []string, stdout, stderr io.Writer) int {
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
	return operatorCommand(ctx, args, stdout, stderr, render, func(ctx context.Context) (operatorRestorer, func(), error) {
		pool, err := postgres.NewPool(ctx, cfg, envport.RoleAPI)
		if err != nil {
			return nil, nil, err
		}
		writer := adminservice.InstanceWriter{
			Operators:  postgres.NewOperatorRepository(),
			Journal:    postgres.NewInstanceJournal(security.NewCursorCodec(cfg.SecretKey)),
			UnitOfWork: postgres.NewUnitOfWork(pool),
			Clock:      clockadapter.System{},
			IDs:        clockadapter.NewUUIDv7(clockadapter.System{}),
		}
		return adminservice.RestoreOperator{Writer: writer}, pool.Close, nil
	})
}

// operatorCommand parses, connects and restores. Exit codes: 0 done (also when the account already
// was an operator), 1 refused or failed, 2 used wrongly.
func operatorCommand(
	ctx context.Context, args []string, stdout, stderr io.Writer,
	render func(code string, params map[string]string) string,
	connect func(context.Context) (operatorRestorer, func(), error),
) int {
	if len(args) == 0 || args[0] != "add" {
		_, _ = fmt.Fprintln(stderr, render("cli.operator_usage", nil))
		return 2
	}
	flags := flag.NewFlagSet("hubtask operator add", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	workspace := flags.String("workspace", "", "")
	email := flags.String("email", "", "")
	if err := flags.Parse(args[1:]); err != nil || *workspace == "" || *email == "" || flags.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, render("cli.operator_usage", nil))
		return 2
	}

	restorer, closeConnection, err := connect(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, render("cli.operator_no_database", nil))
		return 1
	}
	defer closeConnection()

	params := map[string]string{"workspace": *workspace, "email": *email}
	restored, err := restorer.Execute(ctx, *workspace, *email)
	if err != nil {
		code := shared.AsError(err).DetailCode
		if code == "" {
			code = "cli.operator_failed"
		}
		_, _ = fmt.Fprintln(stderr, render(code, params))
		return 1
	}
	if restored.Added {
		_, _ = fmt.Fprintln(stdout, render("cli.operator_added", params))
	} else {
		_, _ = fmt.Fprintln(stdout, render("cli.operator_already", params))
	}
	return 0
}
