// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	storageadapter "github.com/Jersyfi/hubtask/infrastructure/storage"
)

type bucketFake struct {
	allowed  map[string]bool
	rules    []storageadapter.CORSRule
	readErr  error
	writeErr error
	written  [][]storageadapter.CORSRule
}

func (b *bucketFake) Preflight(_ context.Context, origin string) storageadapter.Preflight {
	outcome := storageadapter.PreflightRefused
	if b.allowed[origin] {
		outcome = storageadapter.PreflightAllowed
	}
	return storageadapter.Preflight{Origin: origin, Outcome: outcome}
}

func (b *bucketFake) BucketCORS(context.Context) ([]storageadapter.CORSRule, error) {
	return b.rules, b.readErr
}

func (b *bucketFake) PutBucketCORS(_ context.Context, rules []storageadapter.CORSRule) error {
	if b.writeErr != nil {
		return b.writeErr
	}
	b.written = append(b.written, rules)
	b.rules = rules
	return nil
}

// deployment.md §6.1: `storage cors check` prints each origin's answer; `apply` writes Hubtask's
// rule - the wildcard beside the base origin in subdomain mode - keeps the rules of others,
// refuses to remove one without --replace, prints what --replace removes before it writes and
// writes only once confirmed, and tells a key without the right and a provider without the call
// apart by sentence and exit code.
func TestTheStorageCORSCommand(t *testing.T) {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("the catalogue: %v", err)
	}
	render := func(code string, params map[string]string) string { return renderer.Render("en", code, params) }

	single := corsSetup{bucket: "media", origins: storageadapter.OriginsFor(
		"https://tasks.example.org", envport.TenancySingle, envport.HostsSubdomain)}
	subdomain := corsSetup{bucket: "media", origins: storageadapter.OriginsFor(
		"https://hubtask.example.eu", envport.TenancyMulti, envport.HostsSubdomain)}
	assigned := corsSetup{bucket: "media", assigned: true, origins: storageadapter.OriginsFor(
		"https://hubtask.example.eu", envport.TenancyMulti, envport.HostsAssigned)}
	theirPut := storageadapter.CORSRule{ID: "uploads", AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"PUT"}, AllowedHeaders: []string{"*"}}
	theirGet := storageadapter.CORSRule{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET"}}

	cases := []struct {
		name     string
		args     []string
		setup    corsSetup
		bucket   *bucketFake
		answer   string
		asked    bool
		exit     int
		out, err []string
		written  []string // the origins of the first rule written; nil when nothing may be written
	}{
		{name: "check allowed", args: []string{"cors", "check"}, setup: single,
			bucket: &bucketFake{allowed: map[string]bool{"https://tasks.example.org": true}},
			exit:   0, out: []string{"media lets the browser upload from https://tasks.example.org."}},
		{name: "check refused in subdomain mode", args: []string{"cors", "check"}, setup: subdomain,
			bucket: &bucketFake{allowed: map[string]bool{"https://hubtask.example.eu": true}},
			exit:   1, err: []string{"refuses the browser's upload from https://hubtask-cors-check.hubtask.example.eu"}},
		{name: "fresh bucket", args: []string{"cors", "apply"}, setup: single, bucket: &bucketFake{},
			exit: 0, out: []string{"now lets the browser upload from https://tasks.example.org", "at the latest after 15 minutes"},
			written: []string{"https://tasks.example.org"}},
		{name: "fresh bucket in subdomain mode", args: []string{"cors", "apply"}, setup: subdomain, bucket: &bucketFake{},
			exit: 0, written: []string{"https://hubtask.example.eu", "https://*.hubtask.example.eu"}},
		{name: "assigned hosts", args: []string{"cors", "apply"}, setup: assigned, bucket: &bucketFake{},
			exit: 0, out: []string{"add each one to the rule's AllowedOrigins"}, written: []string{"https://hubtask.example.eu"}},
		{name: "rules of others kept", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirGet}},
			exit:   0, out: []string{"Rules somebody else set on media are kept beside it (1)."},
			written: []string{"https://tasks.example.org"}},
		{name: "a contradicting rule without replace", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirPut}},
			exit:   4, err: []string{"Nothing was written", `"AllowedOrigins": [`, "The rule Hubtask would add:"}},
		{name: "replace confirmed by --yes", args: []string{"cors", "apply", "--replace", "--yes"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirPut, theirGet}},
			exit:   0, out: []string{"These rules on media are removed:", `"AllowedHeaders": [`, "are kept beside it (1)"},
			written: []string{"https://tasks.example.org"}},
		{name: "replace confirmed on the terminal", args: []string{"cors", "apply", "--replace"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirPut}}, answer: "media", asked: true,
			exit: 0, out: []string{"Type the bucket's name, media"}, written: []string{"https://tasks.example.org"}},
		{name: "replace not confirmed", args: []string{"cors", "apply", "--replace"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirPut}}, answer: "no", asked: true,
			exit: 4, out: []string{"These rules on media are removed:"}, err: []string{"Nothing was written."}},
		{name: "replace with nobody to ask", args: []string{"cors", "apply", "--replace"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{theirPut}},
			exit:   4, err: []string{"--replace --yes"}},
		{name: "already set", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{rules: []storageadapter.CORSRule{storageadapter.HubtaskRule(single.origins.Rule)}},
			exit:   0, out: []string{"already lets the browser upload"}},
		{name: "forbidden", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{readErr: storageadapter.ErrCORSForbidden},
			exit:   1, err: []string{"The storage key may not read or set the CORS rules of media.", `"https://tasks.example.org"`, `"Content-Type"`}},
		{name: "not implemented", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{writeErr: storageadapter.ErrCORSNotImplemented},
			exit:   3, err: []string{"set this rule in the provider's own way", "can pass hubtask storage cors check all the same"}},
		{name: "another answer", args: []string{"cors", "apply"}, setup: single,
			bucket: &bucketFake{readErr: &storageadapter.CORSStatusError{Status: 404, Code: "NoSuchBucket"}},
			exit:   1, err: []string{"(answer 404)"}},
		{name: "no subcommand", args: []string{"cors"}, setup: single, bucket: &bucketFake{}, exit: 2, err: []string{"Usage: hubtask storage cors"}},
		{name: "check takes no flag", args: []string{"cors", "check", "--yes"}, setup: single, bucket: &bucketFake{}, exit: 2},
		{name: "no base URL", args: []string{"cors", "check"}, setup: corsSetup{bucket: "media"}, bucket: &bucketFake{},
			exit: 1, err: []string{"HUBTASK_BASE_URL is not set"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			confirm := func() (string, bool) { return tc.answer, tc.asked }
			exit := storageCommand(t.Context(), tc.args, &stdout, &stderr, render, tc.bucket, tc.setup, confirm)
			if exit != tc.exit {
				t.Errorf("exit = %d, want %d (stdout %q, stderr %q)", exit, tc.exit, stdout.String(), stderr.String())
			}
			for _, want := range tc.out {
				if !strings.Contains(stdout.String(), want) {
					t.Errorf("stdout %q lacks %q", stdout.String(), want)
				}
			}
			for _, want := range tc.err {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr %q lacks %q", stderr.String(), want)
				}
			}
			switch {
			case tc.written == nil && len(tc.bucket.written) > 0:
				t.Errorf("written %+v, want nothing", tc.bucket.written)
			case tc.written != nil:
				if len(tc.bucket.written) != 1 ||
					strings.Join(tc.bucket.written[0][0].AllowedOrigins, " ") != strings.Join(tc.written, " ") ||
					tc.bucket.written[0][0].ID != storageadapter.HubtaskRuleID {
					t.Errorf("written %+v, want Hubtask's rule for %v first", tc.bucket.written, tc.written)
				}
			}
		})
	}
}
