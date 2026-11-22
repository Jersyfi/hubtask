// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// fakeBucket is an S3 endpoint that answers preflights from its CORS rules the way S3 documents
// it (first matching rule, the asked origin echoed back) and keeps the configuration `?cors`
// writes. How a real provider answers is not proven here.
type fakeBucket struct {
	mu        sync.Mutex
	rules     []CORSRule
	status    int // answers every ?cors call with this status when set
	code      string
	puts      int
	preflight []*http.Request
	lastBody  []byte
	lastReq   *http.Request
}

func (b *fakeBucket) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.Method == http.MethodOptions {
		b.preflight = append(b.preflight, r.Clone(r.Context()))
		origin := r.Header.Get("Origin")
		method := r.Header.Get("Access-Control-Request-Method")
		for _, rule := range b.rules {
			if !matches(rule.AllowedOrigins, origin) || !contains(rule.AllowedMethods, method) {
				continue
			}
			allowed := origin
			if contains(rule.AllowedOrigins, "*") {
				allowed = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", allowed)
			w.Header().Set("Access-Control-Allow-Methods", strings.Join(rule.AllowedMethods, ", "))
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.URL.RawQuery != "cors" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if b.status != 0 {
		w.WriteHeader(b.status)
		_, _ = io.WriteString(w, "<Error><Code>"+b.code+"</Code><BucketName>secret-name</BucketName></Error>")
		return
	}
	switch r.Method {
	case http.MethodGet:
		if len(b.rules) == 0 {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "<Error><Code>NoSuchCORSConfiguration</Code></Error>")
			return
		}
		body, _ := xml.Marshal(corsConfiguration{Xmlns: s3Namespace, Rules: b.rules})
		_, _ = w.Write(body)
	case http.MethodPut:
		body, _ := io.ReadAll(r.Body)
		b.lastBody, b.lastReq = body, r.Clone(r.Context())
		var config corsConfiguration
		if err := xml.Unmarshal(body, &config); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b.rules = config.Rules
		b.puts++
		w.WriteHeader(http.StatusOK)
	}
}

// matches is S3's origin matching: exact, `*`, or one `*` inside the pattern.
func matches(patterns []string, origin string) bool {
	for _, pattern := range patterns {
		if pattern == "*" || pattern == origin {
			return true
		}
		if prefix, suffix, ok := strings.Cut(pattern, "*"); ok &&
			strings.HasPrefix(origin, prefix) && strings.HasSuffix(origin, suffix) &&
			len(origin) > len(prefix)+len(suffix) {
			return true
		}
	}
	return false
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if strings.EqualFold(v, value) {
			return true
		}
	}
	return false
}

// preflightAgainst answers every preflight with the given headers and status.
func preflightAgainst(t *testing.T, status int, headers map[string]string) (*S3Storage, *[]*http.Request) {
	t.Helper()
	var seen []*http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(r.Context()))
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return s3Against(t, server.URL, true), &seen
}

// deployment.md §6.1: the preflight passes when the bucket echoes the asked origin or answers
// `*`, with PUT among the methods; anything else is a refusal, and no answer is no finding.
func TestThePreflightReadsTheAnswerTheWayABrowserDoes(t *testing.T) {
	const origin = "https://tasks.example.org"
	for name, tc := range map[string]struct {
		status  int
		headers map[string]string
		want    PreflightOutcome
	}{
		"echoed origin": {200, map[string]string{
			"Access-Control-Allow-Origin": origin, "Access-Control-Allow-Methods": "GET, PUT"}, PreflightAllowed},
		"any origin": {200, map[string]string{
			"Access-Control-Allow-Origin": "*", "Access-Control-Allow-Methods": "PUT"}, PreflightAllowed},
		"another origin": {200, map[string]string{
			"Access-Control-Allow-Origin": "https://other.example.org", "Access-Control-Allow-Methods": "PUT"}, PreflightRefused},
		"no PUT": {200, map[string]string{
			"Access-Control-Allow-Origin": origin, "Access-Control-Allow-Methods": "GET, HEAD"}, PreflightRefused},
		"a pattern is not an answer": {200, map[string]string{
			"Access-Control-Allow-Origin": "https://*.example.org", "Access-Control-Allow-Methods": "PUT"}, PreflightRefused},
		"no rule":       {403, nil, PreflightRefused},
		"server broken": {503, nil, PreflightUnreachable},
	} {
		t.Run(name, func(t *testing.T) {
			store, seen := preflightAgainst(t, tc.status, tc.headers)
			got := store.Preflight(t.Context(), origin)
			if got.Outcome != tc.want {
				t.Fatalf("outcome = %v, want %v", got.Outcome, tc.want)
			}
			req := (*seen)[0]
			if req.Method != http.MethodOptions || req.Header.Get("Origin") != origin ||
				req.Header.Get("Access-Control-Request-Method") != "PUT" ||
				req.Header.Get("Access-Control-Request-Headers") != "content-type" {
				t.Errorf("the preflight asked %s %v", req.Method, req.Header)
			}
			// Unsigned: a browser sends no credential, and the answer must be the one it gets.
			if req.Header.Get("Authorization") != "" || req.Header.Get("x-amz-date") != "" {
				t.Errorf("the preflight carried a signature: %v", req.Header)
			}
		})
	}

	unreachable := s3Against(t, "http://127.0.0.1:1", true)
	if got := unreachable.Preflight(t.Context(), origin); got.Outcome != PreflightUnreachable {
		t.Errorf("an endpoint that does not answer = %v, want unreachable", got.Outcome)
	}

	redirecting, _ := preflightAgainst(t, http.StatusFound, map[string]string{"Location": "https://elsewhere.example"})
	if got := redirecting.Preflight(t.Context(), origin); got.Outcome != PreflightUnreachable {
		t.Errorf("a redirect was followed or judged: %v", got.Outcome)
	}
}

// deployment.md §6.1: in multi mode with subdomain hosts the check asks with a workspace's
// own origin and the rule names the wildcard; with assigned hosts and in single mode only the base
// origin. A default port is left out, as the browser leaves it out of the Origin it sends.
func TestTheOriginsComeFromTheBaseURLAndTheWorkspaceHosts(t *testing.T) {
	for name, tc := range map[string]struct {
		base        string
		tenancy     env.TenancyMode
		hosts       env.WorkspaceHosts
		check, rule []string
	}{
		"single": {"https://tasks.example.org/", env.TenancySingle, env.HostsSubdomain,
			[]string{"https://tasks.example.org"}, []string{"https://tasks.example.org"}},
		"multi, subdomain": {"https://hubtask.example.eu", env.TenancyMulti, env.HostsSubdomain,
			[]string{"https://hubtask.example.eu", "https://hubtask-cors-check.hubtask.example.eu"},
			[]string{"https://hubtask.example.eu", "https://*.hubtask.example.eu"}},
		"multi, assigned": {"https://hubtask.example.eu", env.TenancyMulti, env.HostsAssigned,
			[]string{"https://hubtask.example.eu"}, []string{"https://hubtask.example.eu"}},
		"a port kept": {"http://localhost:8080", env.TenancySingle, "",
			[]string{"http://localhost:8080"}, []string{"http://localhost:8080"}},
		"default port": {"https://tasks.example.org:443", env.TenancySingle, "",
			[]string{"https://tasks.example.org"}, []string{"https://tasks.example.org"}},
		"no base URL": {"", env.TenancySingle, "", nil, nil},
	} {
		got := OriginsFor(tc.base, tc.tenancy, tc.hosts)
		if strings.Join(got.Check, " ") != strings.Join(tc.check, " ") ||
			strings.Join(got.Rule, " ") != strings.Join(tc.rule, " ") {
			t.Errorf("%s: origins = %+v, want check %v rule %v", name, got, tc.check, tc.rule)
		}
	}
}

// deployment.md §6.1: in subdomain mode the rule apply writes lets a workspace's origin
// through - the bucket echoes it - and the base origin as well; a pattern alone would leave the
// base host out.
func TestAWorkspaceOriginPassesTheWildcardRule(t *testing.T) {
	origins := OriginsFor("https://hubtask.example.eu", env.TenancyMulti, env.HostsSubdomain)
	bucket := &fakeBucket{rules: []CORSRule{HubtaskRule(origins.Rule)}}
	server := httptest.NewServer(bucket)
	defer server.Close()
	store := s3Against(t, server.URL, true)

	for _, origin := range origins.Check {
		if got := store.Preflight(t.Context(), origin); got.Outcome != PreflightAllowed {
			t.Errorf("%s = %v, want allowed", origin, got.Outcome)
		}
	}
	if got := store.Preflight(t.Context(), "https://elsewhere.example"); got.Outcome != PreflightRefused {
		t.Errorf("a foreign origin = %v, want refused", got.Outcome)
	}
}

// deployment.md §6.1: the configuration is read and written on the bucket's `?cors`, signed
// over the real body with Content-MD5 beside it; a bucket without one reads as no rules.
func TestTheCORSConfigurationIsReadAndWrittenSigned(t *testing.T) {
	bucket := &fakeBucket{}
	server := httptest.NewServer(bucket)
	defer server.Close()
	store := s3Against(t, server.URL, true)

	rules, err := store.BucketCORS(t.Context())
	if err != nil || len(rules) != 0 {
		t.Fatalf("a bucket without rules = %v (%v)", rules, err)
	}
	ours := HubtaskRule([]string{"https://tasks.example.org"})
	if err := store.PutBucketCORS(t.Context(), []CORSRule{ours}); err != nil {
		t.Fatalf("writing: %v", err)
	}
	req := bucket.lastReq
	if req.URL.Path != "/hubtask-media" {
		t.Errorf("written at %s", req.URL.Path)
	}
	digest := md5.Sum(bucket.lastBody)
	if req.Header.Get("Content-MD5") != base64.StdEncoding.EncodeToString(digest[:]) {
		t.Errorf("Content-MD5 = %q", req.Header.Get("Content-MD5"))
	}
	hash := sha256.Sum256(bucket.lastBody)
	if req.Header.Get("x-amz-content-sha256") != hex.EncodeToString(hash[:]) {
		t.Errorf("the signed payload hash is not the body's")
	}
	if !strings.HasPrefix(req.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ") {
		t.Errorf("the write is not signed")
	}
	if !strings.Contains(string(bucket.lastBody), `xmlns="`+s3Namespace+`"`) ||
		!strings.Contains(string(bucket.lastBody), "<ID>hubtask-upload</ID>") {
		t.Errorf("body = %s", bucket.lastBody)
	}

	rules, err = store.BucketCORS(t.Context())
	if err != nil || len(rules) != 1 || !sameRule(rules[0], ours) || rules[0].ID != HubtaskRuleID {
		t.Fatalf("read back = %+v (%v)", rules, err)
	}
}

// deployment.md §6.1: a key without the right reads as forbidden, a provider without the call
// as not implemented - also when it says so by code - and anything else by its status alone.
func TestTheCORSRefusalsAreToldApart(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		code   string
		want   error
	}{
		"forbidden":               {403, "AccessDenied", ErrCORSForbidden},
		"not implemented":         {501, "NotImplemented", ErrCORSNotImplemented},
		"not implemented by code": {400, "NotImplemented", ErrCORSNotImplemented},
	} {
		bucket := &fakeBucket{status: tc.status, code: tc.code}
		server := httptest.NewServer(bucket)
		store := s3Against(t, server.URL, true)
		if _, err := store.BucketCORS(t.Context()); !errors.Is(err, tc.want) {
			t.Errorf("%s: read = %v, want %v", name, err, tc.want)
		}
		if err := store.PutBucketCORS(t.Context(), nil); !errors.Is(err, tc.want) {
			t.Errorf("%s: write = %v, want %v", name, err, tc.want)
		}
		server.Close()
	}

	bucket := &fakeBucket{status: 404, code: "NoSuchBucket"}
	server := httptest.NewServer(bucket)
	defer server.Close()
	_, err := s3Against(t, server.URL, true).BucketCORS(t.Context())
	var status *CORSStatusError
	if !errors.As(err, &status) || status.Status != 404 || strings.Contains(err.Error(), "secret-name") {
		t.Errorf("a missing bucket = %v", err)
	}
}

// deployment.md §6.1: rules of others are kept beside Hubtask's; one that
// answers the upload on its own terms is never removed without replace, and with replace it is
// named among the removed.
func TestThePlanKeepsTheRulesOfOthers(t *testing.T) {
	ours := HubtaskRule([]string{"https://tasks.example.org", "https://*.tasks.example.org"})
	getAll := CORSRule{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"GET"}}
	theirPut := CORSRule{ID: "uploads", AllowedOrigins: []string{"https://tasks.example.org"},
		AllowedMethods: []string{"PUT", "POST"}, AllowedHeaders: []string{"*"}}
	byHand := ours
	byHand.ID = ""
	stale := HubtaskRule([]string{"https://old.example.org"})

	fresh := PlanCORS(nil, ours, false)
	if len(fresh.Rules) != 1 || !sameRule(fresh.Rules[0], ours) {
		t.Errorf("a fresh bucket = %+v", fresh)
	}

	merged := PlanCORS([]CORSRule{getAll}, ours, false)
	if len(merged.Rules) != 2 || merged.Rules[0].ID != HubtaskRuleID || len(merged.Kept) != 1 {
		t.Errorf("a foreign GET rule = %+v, want ours first and theirs kept", merged)
	}

	refused := PlanCORS([]CORSRule{getAll, theirPut}, ours, false)
	if refused.Rules != nil || len(refused.Contradicting) != 1 || refused.Contradicting[0].ID != "uploads" {
		t.Errorf("a contradicting rule without replace = %+v, want nothing written", refused)
	}

	replaced := PlanCORS([]CORSRule{getAll, theirPut}, ours, true)
	if len(replaced.Removed) != 1 || replaced.Removed[0].ID != "uploads" ||
		len(replaced.Rules) != 2 || replaced.Rules[1].AllowedMethods[0] != "GET" {
		t.Errorf("with replace = %+v, want theirs removed and the GET rule kept", replaced)
	}

	if plan := PlanCORS([]CORSRule{byHand}, ours, false); !plan.Current || plan.Rules != nil {
		t.Errorf("the rule set by hand from the printed JSON = %+v, want current", plan)
	}
	if plan := PlanCORS([]CORSRule{ours, getAll}, ours, false); !plan.Current {
		t.Errorf("ours already first = %+v, want current", plan)
	}
	if plan := PlanCORS([]CORSRule{stale}, ours, false); len(plan.Rules) != 1 || !sameRule(plan.Rules[0], ours) {
		t.Errorf("an outdated rule of Hubtask's = %+v, want it rewritten", plan)
	}

	full := make([]CORSRule, corsRuleLimit)
	for i := range full {
		full[i] = getAll
	}
	if plan := PlanCORS(full, ours, false); !plan.TooMany || plan.Rules != nil {
		t.Errorf("a full configuration = %+v, want too many", plan)
	}
}

// What a rule of others carries beyond the elements named here survives the round trip.
func TestARuleOfOthersIsKeptWhole(t *testing.T) {
	document := `<CORSConfiguration xmlns="` + s3Namespace + `"><CORSRule><AllowedOrigin>*</AllowedOrigin>` +
		`<AllowedMethod>GET</AllowedMethod><MaxAgeSeconds>300</MaxAgeSeconds>` +
		`<ProviderExtra>kept</ProviderExtra></CORSRule></CORSConfiguration>`
	var config corsConfiguration
	if err := xml.Unmarshal([]byte(document), &config); err != nil {
		t.Fatal(err)
	}
	out, err := xml.Marshal(corsConfiguration{Xmlns: s3Namespace, Rules: config.Rules})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<MaxAgeSeconds>300</MaxAgeSeconds>", "kept</ProviderExtra>"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("%s lost from %s", want, out)
		}
	}
}
