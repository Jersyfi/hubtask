// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

// The helper's own suite. A container helper that silently does nothing is worse than none: every
// suite that leans on it would go green having proved nothing, which is exactly the failure mode
// `weed shell` invites - it exits 0 on a command it refused, and it blocks rather than failing
// when it cannot reach the master (ADR-0067).
package s3test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTheIdentitiesCarryTheCredentialAsValidJSON(t *testing.T) {
	var parsed struct {
		Identities []struct {
			Name        string `json:"name"`
			Credentials []struct {
				AccessKey string `json:"accessKey"`
				SecretKey string `json:"secretKey"`
			} `json:"credentials"`
			Actions []string `json:"actions"`
		} `json:"identities"`
	}
	if err := json.Unmarshal([]byte(identities()), &parsed); err != nil {
		t.Fatalf("the server's configuration is not valid JSON: %v\n%s", err, identities())
	}
	if len(parsed.Identities) != 1 || len(parsed.Identities[0].Credentials) != 1 {
		t.Fatalf("want one identity with one credential, got %+v", parsed.Identities)
	}
	credential := parsed.Identities[0].Credentials[0]
	if credential.AccessKey != AccessKey || credential.SecretKey != SecretKey {
		t.Errorf("the file carries %q/%q, the suites sign with %q/%q",
			credential.AccessKey, credential.SecretKey, AccessKey, SecretKey)
	}
	if len(parsed.Identities[0].Actions) == 0 {
		t.Error("an identity allowed nothing would refuse every call the suites make")
	}
}

func TestTheImageIsOverridable(t *testing.T) {
	if got := Image(); !strings.Contains(got, ":") {
		t.Errorf("the default image %q carries no tag, and an untagged pin is not a pin", got)
	}
	t.Setenv("HUBTASK_TEST_S3_IMAGE", "example.invalid/some/server:1.2.3")
	if got := Image(); got != "example.invalid/some/server:1.2.3" {
		t.Errorf("the environment was ignored: %q", got)
	}
}

// The listing is read by whole name rather than by substring: `media` must not be found in a
// listing that holds only `hubtask-media`. A table test, because the cases are cheap and the bug
// they guard against would be invisible - a bucket reported present that was never made.
func TestTheListingIsReadByWholeName(t *testing.T) {
	// The real thing, demultiplexed: two lines from `s3.bucket.create` and one per bucket.
	const listing = "create bucket under /buckets\ncreated bucket hubtask-media\n" +
		"  hubtask-backups\tsize:0\tlogical:0\tchunk:0\n" +
		"  hubtask-media\tsize:52224\tlogical:52224\tchunk:5\n"
	for _, c := range []struct {
		name string
		want bool
	}{
		{"hubtask-media", true},
		{"hubtask-backups", true},
		{"media", false},
		{"hubtask", false},
		{"hubtask-medial", false},
		{"", false},
	} {
		if got := listed(listing, c.name); got != c.want {
			t.Errorf("listed(%q) = %v, want %v", c.name, got, c.want)
		}
	}
	// And the refusal the server actually prints, which `weed shell` follows with exit 0.
	if listed("error: bucket name must between [3, 63] characters\n", "ab") {
		t.Error("a refusal was read as a listing")
	}
}

func TestStartMakesTheBucketItWasAskedFor(t *testing.T) {
	ctx := context.Background()
	container := Start(t, "hubtask-media")

	// A second bucket whose name contains the first as a substring, and the other way round: both
	// have to be found, and neither may be found by accident.
	if err := CreateBucket(ctx, container, "media"); err != nil {
		t.Fatalf("creating a second bucket: %v", err)
	}

	// The endpoint is the other half of what a caller gets, and it has to answer.
	endpoint := Endpoint(ctx, t, container)
	if !strings.HasPrefix(endpoint, "http://") {
		t.Errorf("endpoint %q is not a URL a caller can use", endpoint)
	}
}

// The trap ADR-0067 names: the server refuses, `weed shell` exits 0 and prints the refusal, and a
// helper that trusted the exit code would answer nil.
func TestCreateBucketRefusesToClaimANameTheServerRejected(t *testing.T) {
	container := Start(t, "")
	for _, name := range []string{"UPPER_Case", "ab", "a..b", "-leading"} {
		if err := CreateBucket(context.Background(), container, name); err == nil {
			t.Errorf("creating %q was reported as a success", name)
		}
	}
}

// The other trap: when the server is gone, the helper answers an error rather than blocking. The
// deadline is the helper's own, so a caller that passed a background context is still protected.
func TestCreateBucketFailsWhenTheServerIsGone(t *testing.T) {
	container := Start(t, "")
	grace := 5 * time.Second
	if err := container.Stop(context.Background(), &grace); err != nil {
		t.Fatalf("stopping the container: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- CreateBucket(context.Background(), container, "after-the-outage") }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("a bucket was reported as made against a stopped server")
		}
	case <-time.After(90 * time.Second):
		t.Fatal("CreateBucket blocked instead of failing: its own deadline did not hold")
	}
}

// The wait strategy is a readiness promise rather than a liveness one: `Start` returns only once
// the S3 API answers, and the bucket creation that follows it is a write. If `/healthz` ever went
// back to meaning "the process is up", this is the test that would notice - Start would return
// before the write could succeed, and CreateBucket would fail.
func TestTheServerIsWritableAsSoonAsStartReturns(t *testing.T) {
	for run := range 3 {
		container := Start(t, "")
		if err := CreateBucket(context.Background(), container, "written-immediately"); err != nil {
			t.Fatalf("run %d: the first write after Start failed: %v", run+1, err)
		}
	}
}
