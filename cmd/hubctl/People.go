// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The people of a workspace (UC-ID-14, UC-ID-20, P-08): who is invited, who holds which role
// where, and an account without an address. Every rule is the server's - who may grant what, the
// step-up an OWNER grant demands, whether managed accounts are allowed - and this group only asks
// and says what came back. Two answers are shown once and never again, an invitation link where
// the installation sends no mail and a managed account's start password: both go to standard
// output, and the warning that they are shown once to standard error.

const (
	invitePath        = "/accounts:invite"
	createManagedPath = "/accounts:create-managed"
)

func peopleGroup() group {
	return group{
		name:    "people",
		summary: "invite people, give and take away roles, and create accounts without an address",
		commands: []command{
			{
				name:    "ls",
				usage:   "[--scope TENANT|HUB|COLLECTION|ITEM --scope-id <id>]",
				summary: "who holds which role at a scope",
				run:     peopleList,
			},
			{
				name:    "invite",
				usage:   "--email <address> [--name <text>] --role <role> [--scope … --scope-id <id>]",
				summary: "invite somebody and give them a role - the link is shown once where no mail is sent",
				run:     peopleInvite,
			},
			{
				name:    "add-managed",
				usage:   "--name <text> --sign-in <name> --role <role> [--scope … --scope-id <id>]",
				summary: "create an account without an address - its start password is shown once",
				run:     peopleAddManaged,
			},
		},
	}
}

// placementFlags are the two flags every placement takes, read into the contract's pair.
type placementFlags struct {
	scope, id *string
}

func addPlacementFlags(flags interface {
	String(name, value, usage string) *string
}) placementFlags {
	return placementFlags{
		scope: flags.String("scope", "TENANT", "TENANT, HUB, COLLECTION or ITEM"),
		id:    flags.String("scope-id", "", "the hub, collection or entry; absent for TENANT"),
	}
}

// read answers the scope as the contract spells it. TENANT names nothing, and every other scope
// names one thing: half a pair is a mistake this client can name before the server does.
func (s placementFlags) read(cli *CLI) (openapi.MembershipScope, *openapitypes.UUID, error) {
	scope := openapi.MembershipScope(strings.ToUpper(*s.scope))
	if scope == openapi.MembershipScopeTENANT {
		if *s.id != "" {
			return "", nil, usagef("--scope TENANT names nothing; leave --scope-id out")
		}
		return scope, nil, nil
	}
	if *s.id == "" {
		return "", nil, usagef("--scope %s needs --scope-id", scope)
	}
	id, err := cli.parseID("--scope-id", *s.id)
	if err != nil {
		return "", nil, err
	}
	return scope, &id, nil
}

func peopleList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "people", "ls", "[--scope … --scope-id <id>]")
	where := addPlacementFlags(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	scope, id, err := where.read(cli)
	if err != nil {
		return err
	}
	query := url.Values{"scope_type": {string(scope)}}
	if id != nil {
		query.Set("scope_id", id.String())
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var page openapi.MembershipPage
	if err := client.Get(ctx, membershipsPath, query, &page); err != nil {
		return err
	}
	return cli.Emit(page, membershipTable(page.Data))
}

// peopleInvite is the two calls the web app makes (people.svelte.ts): the invitation, which takes
// no role, and the grant on the account it answered. A grant refused after the invitation leaves
// the account invited - as on the screen - and says so, rather than undoing an invitation somebody
// may already have been mailed.
func peopleInvite(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "people", "invite",
		"--email <address> [--name <text>] --role <role> [--scope … --scope-id <id>]")
	email := flags.String("email", "", "where the invitation goes")
	name := flags.String("name", "", "how they are shown until they choose their own")
	role := flags.String("role", "", "the role they are given: OWNER, ADMIN, MEMBER, CONTRIBUTOR, VIEWER, GUEST or AUDITOR")
	where := addPlacementFlags(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *email == "" || *role == "" {
		return usagef("people invite needs --email and --role")
	}
	scope, scopeID, err := where.read(cli)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	body := openapi.AccountInvite{Email: openapitypes.Email(*email)}
	if *name != "" {
		body.DisplayName = name
	}
	var invited openapi.InvitedAccount
	if err := client.Post(ctx, invitePath, body, &invited); err != nil {
		return err
	}

	var membership openapi.Membership
	grantErr := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, membershipsPath, openapi.MembershipGrant{
			AccountId: &invited.Id, Role: openapi.MembershipRole(strings.ToUpper(*role)),
			ScopeType: scope, ScopeId: scopeID,
		}, stepUpProof(stepUp), &membership)
	})

	if err := cli.Emit(invited, Table{
		Columns: []string{"id", "email", "name", "status"},
		Rows:    [][]string{{invited.Id.String(), emailText(invited.Email), invited.DisplayName, string(invited.Status)}},
	}); err != nil {
		return err
	}
	if invited.InvitationLink != nil && *invited.InvitationLink != "" {
		printf(cli.Out, "%s\n", *invited.InvitationLink)
		printf(cli.Err, "that invitation link is shown once and works once, for fourteen days: "+
			"hand it to them, and renew the invitation if it is lost\n")
	}
	if grantErr != nil {
		return fmt.Errorf("the account is invited, and the role was not given: %w", grantErr)
	}
	return nil
}

func peopleAddManaged(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "people", "add-managed",
		"--name <text> --sign-in <name> --role <role> [--scope … --scope-id <id>]")
	name := flags.String("name", "", "how the person is shown")
	signIn := flags.String("sign-in", "", "the name they sign in with, unique in the workspace")
	role := flags.String("role", "", "the role: OWNER, ADMIN, MEMBER, CONTRIBUTOR, VIEWER, GUEST or AUDITOR")
	where := addPlacementFlags(flags)
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *name == "" || *signIn == "" || *role == "" {
		return usagef("people add-managed needs --name, --sign-in and --role")
	}
	scope, scopeID, err := where.read(cli)
	if err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var created openapi.ManagedAccountCreated
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, createManagedPath, openapi.ManagedAccountCreate{
			DisplayName: *name, SignInName: *signIn, Role: openapi.MembershipRole(strings.ToUpper(*role)),
			ScopeType: scope, ScopeId: scopeID,
		}, stepUpProof(stepUp), &created)
	}); err != nil {
		return err
	}
	if cli.JSON {
		return cli.Emit(created, Table{})
	}
	cli.emitTable(Table{
		Columns: []string{"id", "name", "role", "scope"},
		Rows: [][]string{{
			created.Account.Id.String(), created.Account.DisplayName,
			string(created.Membership.Role), placementText(created.Membership.ScopeType, created.Membership.ScopeId),
		}},
	})
	printf(cli.Out, "%s\n", created.StartPassword)
	printf(cli.Err, "that start password is shown once: hand it to them, and they choose their own "+
		"at the first sign-in; an administrator issues a new one if it is lost\n")
	return nil
}

func membershipTable(memberships []openapi.Membership) Table {
	rows := make([][]string, 0, len(memberships))
	for _, each := range memberships {
		holder := "-"
		switch {
		case each.AccountId != nil:
			holder = "account " + each.AccountId.String()
		case each.GroupId != nil:
			holder = "group " + each.GroupId.String()
		}
		rows = append(rows, []string{each.Id.String(), holder, string(each.Role), placementText(each.ScopeType, each.ScopeId)})
	}
	return Table{Columns: []string{"id", "holder", "role", "scope"}, Rows: rows}
}

func placementText(scope openapi.MembershipScope, id *openapitypes.UUID) string {
	if id == nil {
		return string(scope)
	}
	return string(scope) + " " + id.String()
}

func emailText(email *openapitypes.Email) string {
	if email == nil {
		return "-"
	}
	return string(*email)
}
