// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"testing"

	"github.com/Jersyfi/hubtask/infrastructure/storage"
)

// deployment.md §6.1: the bucket's CORS rule written and read through `?cors` and then honoured by
// the unsigned preflight, against the S3-compatible server the suites run. SeaweedFS validates
// every signature strictly, so the signed body hash is proven too; what AWS or a hosted provider
// answers is not proven here.
func TestTheBucketCORSRuleAgainstAnS3Server(t *testing.T) {
	store := startS3(t)
	ctx := t.Context()

	rules, err := store.BucketCORS(ctx)
	if err != nil || len(rules) != 0 {
		t.Fatalf("a fresh bucket's rules = %+v (%v)", rules, err)
	}
	const origin = "https://tasks.example.org"
	if err := store.PutBucketCORS(ctx, []storage.CORSRule{storage.HubtaskRule([]string{origin})}); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	rules, err = store.BucketCORS(ctx)
	if err != nil || len(rules) != 1 || rules[0].ID != storage.HubtaskRuleID {
		t.Fatalf("read back = %+v (%v)", rules, err)
	}
	if got := store.Preflight(ctx, origin); got.Outcome != storage.PreflightAllowed {
		t.Errorf("the preflight from %s = %+v, want allowed", origin, got)
	}
	if got := store.Preflight(ctx, "https://elsewhere.example"); got.Outcome != storage.PreflightRefused {
		t.Errorf("the preflight from another origin = %+v, want refused", got)
	}
}
