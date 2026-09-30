// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The instance level at the terminal — the second of ADR-0070 §5's three doors.
//
// "Die API ist das Produkt; hubctl und das Dashboard sind zwei Clients davon … Ein Betreiber wählt
// seine Tür, nicht seinen Funktionsumfang." So every verb the dashboard has is here: the values and
// their locks, the legal links, the operator register, and the providers the installation offers
// every workspace.
//
// **One switch at a time, and the read is whole.** `settings set` changes one key and sends the
// rest back untouched, because `PUT` replaces the level and a terminal that made somebody retype
// eighteen switches to change one would be a terminal nobody uses. The read-modify-write is the
// same one a screen does, and the same refusal catches a file in `enforce` mode.
const (
	adminSettingsPath  = "/admin/settings"
	adminOperatorsPath = "/admin/operators"
	adminProvidersPath = "/admin/identity-providers"
)

func adminSettings(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "show, set, clear"
	if len(args) == 0 {
		return usagef("admin settings needs a command: %s", verbs)
	}
	switch args[0] {
	case "show":
		return adminSettingsShow(ctx, cli, args[1:])
	case "set":
		return adminSettingsSet(ctx, cli, args[1:], true)
	case "clear":
		return adminSettingsSet(ctx, cli, args[1:], false)
	default:
		return usagef("admin settings has no command %q: %s", args[0], verbs)
	}
}

func adminSettingsShow(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin settings", "show", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var settings openapi.InstanceSettings
	if err := client.Get(ctx, adminSettingsPath, nil, &settings); err != nil {
		return err
	}
	return cli.Emit(settings, settingsTable(settings))
}

// adminSettingsSet changes one key, or clears it.
//
// The key is `area.name` — `sign_in.min_length`, `legal.imprint`, `quotas.export_jobs`,
// `localisation.locale` — which is exactly how the keys are stored and exactly what `show` prints,
// so somebody can copy one out of the table and into the command.
func adminSettingsSet(ctx context.Context, cli *CLI, args []string, setting bool) error {
	verb := "clear"
	if setting {
		verb = "set"
	}
	flags := commandFlags(cli, "admin settings", verb, "<area>.<name> [value]")
	locked := flags.Bool("locked", false,
		"the value applies to every workspace and none may change it")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 || (setting && len(rest) < 2) {
		return usagef("admin settings %s needs <area>.<name>%s", verb,
			map[bool]string{true: " and a value", false: ""}[setting])
	}

	area, name, split := strings.Cut(rest[0], ".")
	if !split || area == "" || name == "" {
		return usagef("%q is not an <area>.<name> key - try `admin settings show`", rest[0])
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	// Read the level whole, change the one key, send it back: `PUT` replaces, and anything left
	// out of the body is a value the installation stops deciding.
	var settings openapi.InstanceSettings
	if err := client.Get(ctx, adminSettingsPath, nil, &settings); err != nil {
		return err
	}

	target := areaOf(&settings, area)
	if target == nil {
		return usagef("%q is not an area of the instance level: sign_in, legal, localisation, quotas", area)
	}
	if setting {
		(*target)[name] = openapi.InstanceSetting{
			Set: true, Value: settingValue(rest[1]), Locked: *locked,
		}
	} else {
		delete(*target, name)
	}

	var written openapi.InstanceSettings
	if err := client.Put(ctx, adminSettingsPath, settings, &written); err != nil {
		return err
	}
	return cli.Emit(written, settingsTable(written))
}

// areaOf answers the map one of the four areas keeps its entries in, creating it where the read
// answered none.
func areaOf(settings *openapi.InstanceSettings, area string) *map[string]openapi.InstanceSetting {
	switch area {
	case "sign_in":
		return ensureArea(&settings.SignIn)
	case "legal":
		return ensureArea(&settings.Legal)
	case "localisation":
		return ensureArea(&settings.Localisation)
	case "quotas":
		return ensureArea(&settings.Quotas)
	default:
		return nil
	}
}

func ensureArea(held **map[string]openapi.InstanceSetting) *map[string]openapi.InstanceSetting {
	if *held == nil {
		fresh := map[string]openapi.InstanceSetting{}
		*held = &fresh
	}
	return *held
}

// settingValue reads a typed value out of one word of a command line.
//
// `true`, `12` and `en` are three different kinds and a terminal has only strings, so the shape is
// guessed in the order that cannot surprise: a flag, then a whole number, then the word itself. A
// comma-separated list is a list, which is what `sign_in.methods` takes.
func settingValue(raw string) any {
	if strings.Contains(raw, ",") {
		parts := strings.Split(raw, ",")
		list := make([]any, 0, len(parts))
		for _, part := range parts {
			list = append(list, strings.TrimSpace(part))
		}
		return list
	}
	if flag, err := strconv.ParseBool(raw); err == nil && (raw == "true" || raw == "false") {
		return flag
	}
	if number, err := strconv.Atoi(raw); err == nil {
		return number
	}
	return raw
}

// settingsTable prints every key of every area, decided or not — the same whole level the
// dashboard draws, because a terminal that showed only what was set could not tell somebody which
// switches exist.
func settingsTable(settings openapi.InstanceSettings) Table {
	rows := make([][]string, 0, 32)
	add := func(area string, held *map[string]openapi.InstanceSetting) {
		if held == nil {
			return
		}
		names := make([]string, 0, len(*held))
		for name := range *held {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			entry := (*held)[name]
			value := "-"
			if entry.Set {
				value = fmt.Sprintf("%v", entry.Value)
			}
			lock := "open"
			if entry.Locked {
				lock = "LOCKED"
			}
			rows = append(rows, []string{area + "." + name, value, lock})
		}
	}
	add("sign_in", settings.SignIn)
	add("legal", settings.Legal)
	add("localisation", settings.Localisation)
	add("quotas", settings.Quotas)
	return Table{Columns: []string{"key", "value", "workspace"}, Rows: rows}
}

func adminOperator(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, add, rm"
	if len(args) == 0 {
		return usagef("admin operator needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminOperatorList(ctx, cli, args[1:])
	case "add":
		return adminOperatorAdd(ctx, cli, args[1:])
	case "rm":
		return adminOperatorRemove(ctx, cli, args[1:])
	default:
		return usagef("admin operator has no command %q: %s", args[0], verbs)
	}
}

func adminOperatorList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin operator", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var register []openapi.Operator
	if err := client.Get(ctx, adminOperatorsPath, nil, &register); err != nil {
		return err
	}
	rows := make([][]string, 0, len(register))
	for _, operator := range register {
		rows = append(rows, []string{
			operator.AccountId.String(), operator.TenantId.String(),
			operator.AddedAt.Format("2006-01-02"),
		})
	}
	return cli.Emit(register, Table{
		Columns: []string{"account", "workspace", "added"}, Rows: rows,
	})
}

// adminOperatorAdd names the account the way a person can: the workspace, and the address.
//
// The identifier form is in the contract for a script that has one, and `--account` is it. What a
// terminal usually has is the other pair, because no screen and no command may list accounts across
// workspaces — `account` is behind row level security, so there is nothing to search.
func adminOperatorAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin operator", "add", "--workspace <slug> --email <address>")
	workspace := flags.String("workspace", "", "the workspace the account is in")
	email := flags.String("email", "", "the address the account signs in with")
	account := flags.String("account", "", "the account's identifier, where a caller has one")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	body := openapi.OperatorAdd{}
	switch {
	case *account != "":
		parsed, err := uuid.Parse(*account)
		if err != nil {
			return usagef("%q is not an account identifier", *account)
		}
		body.AccountId = &parsed
	case *workspace != "" && *email != "":
		body.Workspace = workspace
		address := openapitypes.Email(*email)
		body.Email = &address
	default:
		return usagef("admin operator add needs --workspace and --email, or --account")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Post(ctx, adminOperatorsPath, body, nil); err != nil {
		return err
	}
	return adminOperatorList(ctx, cli, nil)
}

func adminOperatorRemove(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin operator", "rm", "<account>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return usagef("admin operator rm needs an account - `admin operator ls` prints them")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := client.Delete(ctx, adminOperatorsPath+"/"+rest[0], ""); err != nil {
		return err
	}
	return adminOperatorList(ctx, cli, nil)
}

func adminProvider(ctx context.Context, cli *CLI, args []string) error {
	const verbs = "ls, add, rm"
	if len(args) == 0 {
		return usagef("admin provider needs a command: %s", verbs)
	}
	switch args[0] {
	case "ls":
		return adminProviderList(ctx, cli, args[1:])
	case "add":
		return adminProviderAdd(ctx, cli, args[1:])
	case "rm":
		return adminProviderRemove(ctx, cli, args[1:])
	default:
		return usagef("admin provider has no command %q: %s", args[0], verbs)
	}
}

func adminProviderList(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin provider", "ls", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	client, err := cli.client()
	if err != nil {
		return err
	}
	var providers []openapi.IdentityProvider
	if err := client.Get(ctx, adminProvidersPath, nil, &providers); err != nil {
		return err
	}
	rows := make([][]string, 0, len(providers))
	for _, provider := range providers {
		admits := strings.Join(provider.AllowedDirectories, ", ")
		if admits == "" {
			admits = strings.Join(provider.AllowedEmailDomains, ", ")
		}
		rows = append(rows, []string{
			provider.Id.String(), provider.DisplayName, string(provider.Provisioning), admits,
		})
	}
	return cli.Emit(providers, Table{
		Columns: []string{"id", "name", "admits", "listed"}, Rows: rows,
	})
}

// adminProviderAdd offers every workspace a way in.
//
// `--directory` is repeatable and is what `DOMAINS` reads for a provider that names organisations —
// a Microsoft tenant id, a Google Workspace domain. `--domain` is the other list, for an issuer
// with no such claim (ADR-0071 §2).
func adminProviderAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin provider", "add", "--issuer <url> --client-id <id>")
	issuer := flags.String("issuer", "", "the provider's issuer address")
	clientID := flags.String("client-id", "", "this installation's registration with the provider")
	secret := flags.String("client-secret", "", "sealed on the way in and never answered again")
	name := flags.String("name", "", "the name on the button; the issuer's host where empty")
	kind := flags.String("kind", "", "GENERIC, GOOGLE or MICROSOFT; read from the issuer where empty")
	admits := flags.String("admits", "", "INVITED_ONLY, DOMAINS or ANY")
	directories := flags.String("directory", "",
		"the organisations this provider admits, comma separated: Microsoft tenant ids, Google Workspace domains")
	domains := flags.String("domain", "",
		"the email domains it admits, for an issuer that names no organisation")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if *issuer == "" || *clientID == "" || *secret == "" {
		return usagef("admin provider add needs --issuer, --client-id and --client-secret")
	}

	body := openapi.IdentityProviderConfiguration{
		Issuer:              *issuer,
		ClientId:            *clientID,
		ClientSecret:        secret,
		AllowedDirectories:  commaList(*directories),
		AllowedEmailDomains: commaList(*domains),
	}
	if *name != "" {
		body.DisplayName = name
	}
	if *kind != "" {
		chosen := openapi.IdentityProviderKind(*kind)
		body.Kind = &chosen
	}
	if *admits != "" {
		mode := openapi.IdentityProviderProvisioning(*admits)
		body.Provisioning = &mode
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	// A way in offered to every workspace asks for a fresh proof (ADR-0071's addendum, E2): the
	// request goes once, and again with the proof if that is what it was refused for.
	var added openapi.IdentityProvider
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, adminProvidersPath, body, stepUpProof(stepUp), &added)
	}); err != nil {
		return err
	}
	return adminProviderList(ctx, cli, nil)
}

func adminProviderRemove(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin provider", "rm", "<id>")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return usagef("admin provider rm needs a provider - `admin provider ls` prints them")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	if err := cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.DeleteWithHeader(ctx, adminProvidersPath+"/"+rest[0], stepUpProof(stepUp))
	}); err != nil {
		return err
	}
	return adminProviderList(ctx, cli, nil)
}

// commaList reads a repeatable flag written the way a terminal writes one.
func commaList(raw string) *[]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	list := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			list = append(list, trimmed)
		}
	}
	return &list
}
