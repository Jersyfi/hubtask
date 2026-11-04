// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

type resetIssuerFake struct {
	link  string
	err   error
	asked [][2]string
}

func (i *resetIssuerFake) Execute(_ context.Context, slug, email string) (string, error) {
	i.asked = append(i.asked, [2]string{slug, email})
	return i.link, i.err
}

// Fifth answers 5: `hubtask account reset-link` prints the link the machine minted, says it works
// once and how long, refuses an unknown pair with the one sentence, and tells wrong use apart by
// its exit code.
func TestTheAccountResetLinkCommand(t *testing.T) {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("the catalogue: %v", err)
	}
	render := func(code string, params map[string]string) string { return renderer.Render("en", code, params) }
	link := "https://tasks.example.eu/reset#token=abc"

	cases := []struct {
		name     string
		args     []string
		issuer   *resetIssuerFake
		connErr  error
		exit     int
		out, err string
	}{
		{
			name: "printed", args: []string{"reset-link", "--workspace", "demo", "--email", "jerome@example.eu"},
			issuer: &resetIssuerFake{link: link}, exit: 0, out: "It works once",
		},
		{
			name: "no such account", args: []string{"reset-link", "--workspace", "demo", "--email", "nobody@example.eu"},
			issuer: &resetIssuerFake{err: shared.ErrValidation.WithDetail("admin.operator_unknown_account")},
			exit:   1, err: "There is no such account on this installation.",
		},
		{
			name: "provider only", args: []string{"reset-link", "--workspace", "demo", "--email", "jerome@example.eu"},
			issuer: &resetIssuerFake{err: shared.ErrValidation.WithDetail("admin.reset_link_provider")},
			exit:   1, err: "signs in through its provider",
		},
		{name: "no subcommand", args: nil, exit: 2, err: "Usage: hubtask account reset-link"},
		{name: "no address", args: []string{"reset-link", "--workspace", "demo"}, exit: 2, err: "Usage:"},
		{
			name: "no database", args: []string{"reset-link", "--workspace", "demo", "--email", "a@b.eu"},
			connErr: errors.New("refused"), exit: 1, err: "The database could not be reached",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			issuer := tc.issuer
			if issuer == nil {
				issuer = &resetIssuerFake{}
			}
			exit := accountCommand(t.Context(), tc.args, &stdout, &stderr, render,
				func(context.Context) (resetLinkIssuer, func(), error) {
					if tc.connErr != nil {
						return nil, nil, tc.connErr
					}
					return issuer, func() {}, nil
				})
			if exit != tc.exit {
				t.Errorf("exit %d, want %d (stderr %q)", exit, tc.exit, stderr.String())
			}
			if tc.out != "" && !strings.Contains(stdout.String(), tc.out) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.out)
			}
			if tc.exit == 0 && !strings.Contains(stdout.String(), link) {
				t.Errorf("stdout = %q, want the link", stdout.String())
			}
			if tc.err != "" && !strings.Contains(stderr.String(), tc.err) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.err)
			}
		})
	}
}
