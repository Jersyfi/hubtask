// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// resetLinkIssuer is the machine's use case, as the command needs it.
type resetLinkIssuer interface {
	Execute(ctx context.Context, slug, email string) (string, error)
}

// runAccountCommand is `hubtask account reset-link --workspace <short name> --email <address>`
// (fifth answers 5): the reset link a self-hoster without mail would otherwise never receive,
// printed on the machine. Like `operator add` it needs only the server's configuration, and there
// is no route for it.
func runAccountCommand(args []string, stdout, stderr io.Writer) int {
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
	return accountCommand(ctx, args, stdout, stderr, render, func(ctx context.Context) (resetLinkIssuer, func(), error) {
		pool, err := postgres.NewPool(ctx, cfg, envport.RoleAPI)
		if err != nil {
			return nil, nil, err
		}
		return machineResetLinks(postgres.NewUnitOfWork(pool), cfg), pool.Close, nil
	})
}

// machineResetLinks wires the reset's own minting as the server wires it for the reset mail, so
// the printed link is the mailed one: the same pending credential, lifetime and supersession.
func machineResetLinks(uow persistence.UnitOfWork, cfg envport.Config) adminservice.IssueResetLink {
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	store := postgres.NewPasswordRepository()
	minter := identity.MintResetToken{Writer: identity.PasswordWriter{
		Session: identity.SessionWriter{Entropy: clockadapter.CryptoRandom{}, Text: textadapter.Forms{}},
		Resolver: identity.SignInPolicyResolver{
			Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Instance:   postgres.NewInstanceSettingRepository(),
			UnitOfWork: uow,
		},
		Accounts: store, Histories: store,
		Pending: postgres.NewMfaRepository(
			security.NewPendingTokenHasher(cfg.SecretKey), security.NewRecoveryCodeHasher(cfg.SecretKey)),
		WaysIn: identity.WaysIn{
			Providers:  postgres.NewIdentityProviderRepository(),
			Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Instance:   postgres.NewInstanceSettingRepository(),
			UnitOfWork: uow, Clock: clockadapter.System{},
		},
		Text:       textadapter.Forms{},
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
	}}
	return adminservice.IssueResetLink{
		Workspaces: adminservice.RecoverWorkspace{Tenants: postgres.NewAdminTenantRepository(), UnitOfWork: uow},
		Writer: adminservice.InstanceWriter{
			Operators:  postgres.NewOperatorRepository(),
			Journal:    postgres.NewInstanceJournal(security.NewCursorCodec(cfg.SecretKey)),
			UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids,
		},
		Minter:    machineMinter{mint: minter},
		Addresses: workspaceAddress(cfg),
	}
}

// machineMinter answers the reset's minting in the machine's terms: a token where a password is
// set or a provider connected, none where the mail would only point to a provider.
type machineMinter struct{ mint identity.MintResetToken }

func (m machineMinter) MintMachineReset(
	ctx context.Context, tenantID, accountID shared.ID,
) (adminservice.MintedReset, error) {
	link, err := m.mint.MintResetToken(ctx, tenantID, accountID)
	if err != nil {
		return adminservice.MintedReset{}, err
	}
	if !link.HasPassword && !link.First && !link.Connect {
		return adminservice.MintedReset{Token: secret.New("")}, nil
	}
	return adminservice.MintedReset{Token: link.Token, Connect: link.Connect}, nil
}

// accountCommand parses, connects and prints. Exit codes as `operator add`'s: 0 printed, 1 refused
// or failed, 2 used wrongly.
func accountCommand(
	ctx context.Context, args []string, stdout, stderr io.Writer,
	render func(code string, params map[string]string) string,
	connect func(context.Context) (resetLinkIssuer, func(), error),
) int {
	if len(args) == 0 || args[0] != "reset-link" {
		_, _ = fmt.Fprintln(stderr, render("cli.account_usage", nil))
		return 2
	}
	flags := flag.NewFlagSet("hubtask account reset-link", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	workspace := flags.String("workspace", "", "")
	email := flags.String("email", "", "")
	if err := flags.Parse(args[1:]); err != nil || *workspace == "" || *email == "" || flags.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, render("cli.account_usage", nil))
		return 2
	}

	issuer, closeConnection, err := connect(ctx)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, render("cli.operator_no_database", nil))
		return 1
	}
	defer closeConnection()

	params := map[string]string{"workspace": *workspace, "email": *email}
	link, err := issuer.Execute(ctx, *workspace, *email)
	if err != nil {
		code := shared.AsError(err).DetailCode
		if code == "" {
			code = "cli.account_failed"
		}
		_, _ = fmt.Fprintln(stderr, render(code, params))
		return 1
	}
	params["link"] = link
	_, _ = fmt.Fprintln(stdout, render("cli.account_reset_link", params))
	return 0
}
