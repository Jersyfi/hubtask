// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	openapitypes "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The instance level at the terminal — the second of ADR-0070 §5's three doors.
//
// The API is the product, and hubctl and the dashboard are two clients of it: an operator chooses a
// door, not a set of features. So every verb the dashboard has is here: the values and their
// locks, the legal links, the operator register, the providers the installation offers every
// workspace, and the hour a person raises their own session for.
//
// **One switch at a time, and the read is whole.** `settings set` changes one key and sends the
// rest back untouched, because `PUT` replaces the level and a terminal that made somebody retype
// eighteen switches to change one would be a terminal nobody uses. The read-modify-write is the
// same one a screen does, and the same refusal catches a file in `enforce` mode.
const (
	adminSettingsPath  = "/admin/settings"
	adminOperatorsPath = "/admin/operators"
	adminProvidersPath = "/admin/identity-providers"
	elevatePath        = "/auth/sessions:elevate"
)

// alreadyElevated is the refusal of a raise while the session's hour still runs; it names the end.
const alreadyElevated = "auth.session_already_elevated"

// adminElevate raises this profile's session to the control plane for an hour (UC-INS-04 check 6).
//
// A session's act, like every step-up: a personal access token cannot prove a person afresh, and a
// machine holds a token with the scope instead. Nothing new is stored in the profile - the hour is
// the session row's, and the profile's own refresh rotates the same session, so every later
// `hubctl admin` command on this profile reaches the control plane until the hour ends.
func adminElevate(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin", "elevate", "")
	if err := parseCommand(flags, args); err != nil {
		return err
	}
	if cli.Profile.Session.IsEmpty() {
		message, _ := cli.Catalogue.Message("auth.step_up_session_required", nil)
		return errorString(message + "\n  Sign in as a person first - `hubctl login` - and raise that session.")
	}

	client, err := cli.client()
	if err != nil {
		return err
	}
	var raised openapi.SessionElevation
	err = cli.proveAgain(ctx, client, func(stepUp string) error {
		return client.PostWithHeader(ctx, elevatePath, nil, stepUpProof(stepUp), &raised)
	})
	var refusal APIError
	if errors.As(err, &refusal) && refusal.DetailCode == alreadyElevated {
		// The hour runs already: it is not extended and not started again, and its end is the
		// answer the person was after.
		until, parsed := time.Parse(time.RFC3339, refusal.Params["until"])
		if parsed != nil {
			return err
		}
		raised = openapi.SessionElevation{ElevatedUntil: until, RemainingSeconds: int(time.Until(until).Seconds())}
		err = nil
	}
	if err != nil {
		return err
	}
	return cli.Emit(raised, Table{
		Columns: []string{"elevated until", "minutes left"},
		Rows: [][]string{{
			raised.ElevatedUntil.Local().Format("2006-01-02 15:04"),
			strconv.Itoa((raised.RemainingSeconds + 59) / 60),
		}},
	})
}

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
// The key is `area.name` — `sign_in.min_length`, `legal.imprint_url`, `quotas.export_jobs`,
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
		return usagef("%q is not an area of the instance level: sign_in, legal, localisation, quotas, ai", area)
	}
	held, known := (*target)[name]
	if !known {
		// The read answers every key of every area, decided or not, so a key it did not answer is a
		// typo - and the server would skip it as a newer client's key, saving nothing.
		return usagef("%q is not a key of the instance level - try `admin settings show`", rest[0])
	}
	if setting {
		(*target)[name] = openapi.InstanceSetting{
			Set: true, Value: settingValue(held.Kind, rest[1]), Locked: *locked,
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

// areaOf answers the map one of the five areas keeps its entries in, creating it where the read
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
	case "ai":
		return ensureArea(&settings.Ai)
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

// settingValue reads a typed value out of one word of a command line, by the kind the level
// declares for the key.
//
// A terminal has only strings. A word that does not read as the declared kind is sent as the word,
// so the server refuses it exactly as it refuses the same word from the instance file - the
// terminal decides nothing the other doors do not.
func settingValue(kind *openapi.InstanceSettingKind, raw string) any {
	declared := openapi.InstanceSettingKind("")
	if kind != nil {
		declared = *kind
	}
	switch declared {
	case openapi.InstanceSettingKindCOUNT:
		if number, err := strconv.Atoi(raw); err == nil {
			return number
		}
	case openapi.InstanceSettingKindFLAG:
		if raw == "true" || raw == "false" {
			return raw == "true"
		}
	case openapi.InstanceSettingKindCHOICES:
		parts := strings.Split(raw, ",")
		list := make([]any, 0, len(parts))
		for _, part := range parts {
			list = append(list, strings.TrimSpace(part))
		}
		return list
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
				value = printedValue(entry.Value)
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
	add("ai", settings.Ai)
	return Table{Columns: []string{"key", "value", "workspace"}, Rows: rows}
}

// printedValue prints a value as somebody would type it back. A JSON number arrives as a float64,
// which `%v` prints in exponent form from a million up; a whole one is printed in digits.
func printedValue(value any) string {
	if number, isNumber := value.(float64); isNumber && number == math.Trunc(number) {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", value)
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
	return cli.Emit(register, operatorTable(register))
}

// operatorTable names each operator the way the screen does: a name, what they sign in with, and
// their workspace - and the identifier last, because `admin operator rm` takes it.
func operatorTable(register []openapi.Operator) Table {
	rows := make([][]string, 0, len(register))
	for _, operator := range register {
		rows = append(rows, []string{
			valueOr(operator.DisplayName, "-"), operatorAddress(operator),
			valueOr(operator.WorkspaceSlug, operator.TenantId.String()),
			operator.AddedAt.Format("2006-01-02"), operator.AccountId.String(),
		})
	}
	return Table{Columns: []string{"name", "signs in with", "workspace", "added", "account"}, Rows: rows}
}

func operatorAddress(operator openapi.Operator) string {
	switch {
	case operator.Email != nil && *operator.Email != "":
		return string(*operator.Email)
	case operator.SignInName != nil && *operator.SignInName != "":
		return *operator.SignInName
	case operator.Kind != nil && *operator.Kind == openapi.OperatorKindSERVICEACCOUNT:
		return "service account"
	}
	return "-"
}

func valueOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}

// adminOperatorAdd names the account the way a person can: the workspace, and the address.
//
// The identifier form is in the contract for a script that has one, and `--account` is it. What a
// terminal usually has is the other pair, because no screen and no command may list accounts across
// workspaces — `account` is behind row level security, so there is nothing to search.
func adminOperatorAdd(ctx context.Context, cli *CLI, args []string) error {
	flags := commandFlags(cli, "admin operator", "add",
		"(--workspace <slug> --email <address> | --service-account <name> | --account <id>)")
	workspace := flags.String("workspace", "", "the workspace the account is in")
	email := flags.String("email", "", "the address the account signs in with")
	machine := flags.String("service-account", "",
		"a service account of the workspace you are signed in to, by its name")
	account := flags.String("account", "", "the account's identifier, where a caller has one")
	if err := parseCommand(flags, args); err != nil {
		return err
	}

	client, err := cli.client()
	if err != nil {
		return err
	}

	body := openapi.OperatorAdd{}
	switch {
	case *machine != "":
		found, err := serviceAccountNamed(ctx, client, *machine)
		if err != nil {
			return err
		}
		body.AccountId = &found
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
		return usagef("admin operator add needs --workspace and --email, --service-account, or --account")
	}

	if err := client.Post(ctx, adminOperatorsPath, body, nil); err != nil {
		return err
	}
	return adminOperatorList(ctx, cli, nil)
}

// serviceAccountNamed finds a service account of the caller's own workspace by its name (UC-INS-06
// check 1). The workspace's own list, read with the permission that manages its members - never
// another workspace's accounts, which no door at this level lists.
func serviceAccountNamed(ctx context.Context, client *Client, name string) (uuid.UUID, error) {
	var accounts []openapi.Account
	if err := client.Get(ctx, serviceAccountPath, nil, &accounts); err != nil {
		return uuid.UUID{}, err
	}
	var matches []openapi.Account
	for _, each := range accounts {
		if strings.EqualFold(strings.TrimSpace(each.DisplayName), strings.TrimSpace(name)) {
			matches = append(matches, each)
		}
	}
	switch len(matches) {
	case 0:
		return uuid.UUID{}, usagef("no service account of this workspace is called %q - `hubctl service-account ls` lists them", name)
	case 1:
		return matches[0].Id, nil
	}
	return uuid.UUID{}, usagef("%d service accounts are called %q - name one by --account <id> from `hubctl service-account ls`", len(matches), name)
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
		// What the installation's lock on the ways in does to the row (identity.md §18): on in every
		// workspace, or each workspace's own switch.
		lock := ""
		if provider.InstallationLock != nil {
			lock = string(*provider.InstallationLock)
		}
		rows = append(rows, []string{
			provider.Id.String(), provider.DisplayName, string(provider.Provisioning), admits, lock,
		})
	}
	return cli.Emit(providers, Table{
		Columns: []string{"id", "name", "admits", "listed", "lock"}, Rows: rows,
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
