// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
)

// firstStartTimeout bounds the start's one database round trip for the setup code (rule 7).
const firstStartTimeout = 10 * time.Second

// setupIssuer is the first start's use case, as this file needs it.
type setupIssuer interface {
	Execute(ctx context.Context) (identity.IssuedSetupCode, bool, error)
}

// firstStart issues the setup code of an empty installation and writes it to standard error as a
// banner (UC-INS-01 check 1).
//
// Standard error, and not the structured logger: the logger redacts secrets and is tested to carry
// no token (T-18), and this is the one secret meant to be read on the console - by whoever can read
// the process's output, which is the person who could reach its database too (security.md, T-24).
// `docker compose logs`, `kubectl logs` and the system journal show standard error beside the log
// lines, so the banner is in the log as an operator reads it. The logger records only that a code
// was issued and until when.
//
// A failure is logged and the start goes on: an installation whose database cannot be read is
// reported by the readiness check, and a missing banner is the least of it.
func firstStart(
	ctx context.Context, issuer setupIssuer, render func(code string, params map[string]string) string,
	address string,
) {
	ctx, cancel := context.WithTimeout(ctx, firstStartTimeout)
	defer cancel()

	issued, ok, err := issuer.Execute(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "no setup code could be issued", slog.String("error", err.Error()))
		return
	}
	if !ok {
		return
	}
	slog.InfoContext(ctx, "the installation is not set up yet; a setup code is on standard error",
		slog.Time("expires_at", issued.ExpiresAt))
	writeSetupBanner(os.Stderr, render, issued, address)
}

// writeSetupBanner draws the code so it stands out from the log lines around it.
func writeSetupBanner(
	out io.Writer, render func(code string, params map[string]string) string,
	issued identity.IssuedSetupCode, address string,
) {
	params := map[string]string{
		"code":    issued.Code.Display(),
		"until":   issued.ExpiresAt.UTC().Format("15:04 MST"),
		"address": address,
	}
	sentence := render("setup.console_banner", params)
	if address == "" {
		sentence = render("setup.console_banner_no_address", params)
	}
	rule := strings.Repeat("=", 72)
	_, _ = fmt.Fprintf(out, "\n%s\n\n  %s\n\n      %s\n\n  %s\n\n%s\n\n",
		rule, sentence, issued.Code.Display(), render("setup.console_banner_renew", nil), rule)
}

// applyOperatorsFromEnvironment puts HUBTASK_OPERATORS into the register at start, under the
// start's deadline. A failure does not stop the start: the scheduler applies the list again, and
// the health report names what is still waiting.
func applyOperatorsFromEnvironment(ctx context.Context, applying adminservice.OperatorsFromEnvironment) {
	if len(applying.Entries) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, firstStartTimeout)
	defer cancel()
	waiting, err := applying.Apply(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "the operators named in the environment could not be applied",
			slog.String("error", err.Error()))
		return
	}
	if len(waiting) > 0 {
		slog.InfoContext(ctx, "operators named in the environment wait for their account",
			slog.Int("count", len(waiting)))
	}
}
