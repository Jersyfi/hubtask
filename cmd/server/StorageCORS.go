// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	envadapter "github.com/Jersyfi/hubtask/infrastructure/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
)

// storageCommandTimeout bounds the command's whole run against the bucket (rule 7).
const storageCommandTimeout = 60 * time.Second

// Exit codes of `hubtask storage cors`: 0 done, 1 refused or failed, 2 used wrongly, 3 the
// provider sets CORS in its own way, 4 nothing written because rules of others would go.
const (
	exitCORSNotImplemented = 3
	exitCORSNeedsReplace   = 4
)

// bucketCORS is the adapter as the command needs it.
type bucketCORS interface {
	Preflight(ctx context.Context, origin string) storageadapter.Preflight
	BucketCORS(ctx context.Context) ([]storageadapter.CORSRule, error)
	PutBucketCORS(ctx context.Context, rules []storageadapter.CORSRule) error
}

// corsSetup is what the command reads off the server's configuration.
type corsSetup struct {
	bucket   string
	origins  storageadapter.UploadOrigins
	assigned bool
}

// runStorageCommand is `hubtask storage cors check` and `hubtask storage cors apply`:
// the bucket's CORS rule asked about and set from the machine the server runs on, with
// the server's own storage keys - which exist nowhere else (deployment.md §6.1).
func runStorageCommand(args []string, stdout, stderr io.Writer) int {
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
	if cfg.Storage.Kind != envport.StorageS3 {
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_local", nil))
		return 2
	}
	store, err := storageadapter.NewS3Storage(cfg.Storage, cfg.Request.Timeout)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "invalid configuration: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), storageCommandTimeout)
	defer cancel()
	return storageCommand(ctx, args, stdout, stderr, render, store, setupFrom(cfg), terminalConfirm(os.Stdin))
}

func setupFrom(cfg envport.Config) corsSetup {
	return corsSetup{
		bucket:   cfg.Storage.Bucket,
		origins:  storageadapter.OriginsFor(cfg.BaseURL, cfg.Tenancy, cfg.WorkspaceHosts),
		assigned: cfg.Tenancy == envport.TenancyMulti && cfg.WorkspaceHosts == envport.HostsAssigned,
	}
}

// terminalConfirm reads a typed answer where stdin is a terminal; elsewhere there is nobody to
// ask, and --yes is the only consent.
func terminalConfirm(stdin *os.File) func() (string, bool) {
	return func() (string, bool) {
		info, err := stdin.Stat()
		if err != nil || info.Mode()&os.ModeCharDevice == 0 {
			return "", false
		}
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", false
		}
		return strings.TrimSpace(line), true
	}
}

func storageCommand(
	ctx context.Context, args []string, stdout, stderr io.Writer,
	render func(code string, params map[string]string) string,
	store bucketCORS, setup corsSetup, confirm func() (string, bool),
) int {
	if len(args) < 2 || args[0] != "cors" {
		_, _ = fmt.Fprintln(stderr, render("cli.storage_usage", nil))
		return 2
	}
	flags := flag.NewFlagSet("hubtask storage cors", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	replace := flags.Bool("replace", false, "")
	yes := flags.Bool("yes", false, "")
	if err := flags.Parse(args[2:]); err != nil || flags.NArg() > 0 {
		_, _ = fmt.Fprintln(stderr, render("cli.storage_usage", nil))
		return 2
	}
	if len(setup.origins.Check) == 0 {
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_no_base_url", nil))
		return 1
	}

	var exit int
	switch args[1] {
	case "check":
		if *replace || *yes {
			_, _ = fmt.Fprintln(stderr, render("cli.storage_usage", nil))
			return 2
		}
		exit = corsCheck(ctx, stdout, stderr, render, store, setup)
	case "apply":
		exit = corsApply(ctx, stdout, stderr, render, store, setup, *replace, *yes, confirm)
	default:
		_, _ = fmt.Fprintln(stderr, render("cli.storage_usage", nil))
		return 2
	}
	if setup.assigned {
		_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_assigned_hosts", nil))
	}
	return exit
}

func corsCheck(
	ctx context.Context, stdout, stderr io.Writer, render func(string, map[string]string) string,
	store bucketCORS, setup corsSetup,
) int {
	exit := 0
	for _, origin := range setup.origins.Check {
		params := map[string]string{"bucket": setup.bucket, "origin": origin}
		switch store.Preflight(ctx, origin).Outcome {
		case storageadapter.PreflightAllowed:
			_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_allowed", params))
		case storageadapter.PreflightRefused:
			_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_refused", params))
			exit = 1
		default:
			_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_unreachable", params))
			exit = 1
		}
	}
	return exit
}

func corsApply(
	ctx context.Context, stdout, stderr io.Writer, render func(string, map[string]string) string,
	store bucketCORS, setup corsSetup, replace, yes bool, confirm func() (string, bool),
) int {
	ours := storageadapter.HubtaskRule(setup.origins.Rule)
	params := map[string]string{
		"bucket":  setup.bucket,
		"origins": strings.Join(setup.origins.Rule, ", "),
		"minutes": strconv.Itoa(int(storageadapter.CORSCheckEvery / time.Minute)),
	}

	existing, err := store.BucketCORS(ctx)
	if err != nil {
		return corsRefused(stderr, render, err, ours, params)
	}
	plan := storageadapter.PlanCORS(existing, ours, replace)
	switch {
	case len(plan.Contradicting) > 0:
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_contradicting", params))
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_rules_present", params))
		printRules(stderr, existing)
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_rule_to_add", params))
		printRules(stderr, []storageadapter.CORSRule{ours})
		return exitCORSNeedsReplace
	case plan.Current:
		_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_current", params))
		return 0
	case plan.TooMany:
		params["count"] = strconv.Itoa(len(existing))
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_too_many", params))
		return 1
	}

	if len(plan.Removed) > 0 {
		_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_rules_removed", params))
		printRules(stdout, plan.Removed)
		if !yes {
			_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_confirm", params))
			answer, asked := confirm()
			if !asked || answer != setup.bucket {
				_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_not_confirmed", params))
				return exitCORSNeedsReplace
			}
		}
	}
	if err := store.PutBucketCORS(ctx, plan.Rules); err != nil {
		return corsRefused(stderr, render, err, ours, params)
	}
	if len(plan.Kept) > 0 {
		params["count"] = strconv.Itoa(len(plan.Kept))
		_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_kept", params))
	}
	_, _ = fmt.Fprintln(stdout, render("cli.storage_cors_applied", params))
	return 0
}

// corsRefused says what the bucket's answer means for the operator, with the rule to set by hand
// where the call cannot set it.
func corsRefused(
	stderr io.Writer, render func(string, map[string]string) string, err error,
	ours storageadapter.CORSRule, params map[string]string,
) int {
	var status *storageadapter.CORSStatusError
	switch {
	case errors.Is(err, storageadapter.ErrCORSForbidden):
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_forbidden", params))
		printRules(stderr, []storageadapter.CORSRule{ours})
		return 1
	case errors.Is(err, storageadapter.ErrCORSNotImplemented):
		_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_not_implemented", params))
		printRules(stderr, []storageadapter.CORSRule{ours})
		return exitCORSNotImplemented
	case errors.As(err, &status):
		params["status"] = strconv.Itoa(status.Status)
	default:
		params["status"] = "-"
	}
	_, _ = fmt.Fprintln(stderr, render("cli.storage_cors_failed", params))
	return 1
}

// printRules writes rules in the JSON a provider's console takes - configuration, not prose.
func printRules(out io.Writer, rules []storageadapter.CORSRule) {
	encoded, err := json.MarshalIndent(rules, "", "  ")
	if err != nil {
		return
	}
	_, _ = fmt.Fprintln(out, string(encoded))
}
