// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
	"github.com/Jersyfi/hubtask/infrastructure/httpclient"
)

// What is here is what BK-1 cannot see from the outside: which address a key is turned into, and
// that an archive larger than a part becomes a multipart upload rather than one enormous request.
// That a real S3 service accepts all of it is MinIO's job in test/backup - MinIO validates every
// signature strictly, which is the only proof of a hand-written signer worth having.

// permissive is the outbound configuration these tests need: an httptest server is on loopback,
// which the guard blocks by design and correctly. A backup target on a private network needs the
// same release in production, which is the point rather than a workaround.
func permissive() env.OutboundConfig {
	return env.OutboundConfig{
		Timeout: 5 * time.Second, ConnectTimeout: time.Second,
		MaxResponseBytes: 1 << 20, MaxRedirects: 0, AllowPrivateNetworks: true,
	}
}

func guardedClient() *httpclient.GuardedClient {
	cfg := permissive()
	return httpclient.NewGuardedClient(cfg, httpclient.NewGuard(cfg))
}

// recorder is an S3 that answers everything and remembers what it was asked.
type recorder struct {
	mu       sync.Mutex
	requests []string
	bodies   map[string][]byte
}

func newRecorder() *recorder { return &recorder{bodies: map[string][]byte{}} }

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	body, _ := io.ReadAll(req.Body)

	r.mu.Lock()
	r.requests = append(r.requests, req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery)
	r.bodies[req.Method+" "+req.URL.Path+"?"+req.URL.RawQuery] = body
	r.mu.Unlock()

	switch {
	case req.Method == http.MethodPost && req.URL.Query().Has("uploads"):
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(
			`<InitiateMultipartUploadResult><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`))
	case req.Method == http.MethodPut && req.URL.Query().Get("partNumber") != "":
		w.Header().Set("ETag", `"part-`+req.URL.Query().Get("partNumber")+`"`)
		w.WriteHeader(http.StatusOK)
	case req.Method == http.MethodPost:
		_, _ = w.Write([]byte(`<CompleteMultipartUploadResult></CompleteMultipartUploadResult>`))
	case req.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (r *recorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

func s3Store(t *testing.T, endpoint string, config backup.TargetConfig) *backupstorage.S3Store {
	t.Helper()

	config["endpoint"] = endpoint
	store, err := backupstorage.NewS3Store(guardedClient(), port.Spec{
		Kind:   backup.KindS3,
		Config: config,
		Credentials: map[string]secret.Secret{
			"access_key": secret.New("AKIAEXAMPLE"),
			"secret_key": secret.New("the-secret-access-key"),
		},
	}, func() time.Time { return time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	return store
}

// An archive that fits in one part is one request. A multipart upload leaves a half-finished
// upload behind if the process dies in the middle, and a bucket accumulating those costs money
// quietly - so it is used when it is needed and not before.
func TestASmallArchiveIsOneRequest(t *testing.T) {
	recorded := newRecorder()
	server := httptest.NewServer(recorded)
	defer server.Close()

	store := s3Store(t, server.URL, backup.TargetConfig{"bucket": "hubtask", "path": "instance"})

	written, err := store.Put(t.Context(), "2026/08/a.hbk", strings.NewReader("a short archive"))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if written != int64(len("a short archive")) {
		t.Fatalf("wrote %d bytes", written)
	}

	seen := recorded.seen()
	if len(seen) != 1 {
		t.Fatalf("%d requests: %v", len(seen), seen)
	}
	// Path style, the target's own prefix, then the key: everything self-hosted speaks it and
	// several services speak nothing else.
	if seen[0] != "PUT /hubtask/instance/2026/08/a.hbk?" {
		t.Fatalf("the request was %q", seen[0])
	}
}

// An archive larger than a part becomes a multipart upload: begin, parts, complete. The process
// holds one part rather than an archive, which is the whole memory story of this adapter.
func TestALargeArchiveIsUploadedInParts(t *testing.T) {
	recorded := newRecorder()
	server := httptest.NewServer(recorded)
	defer server.Close()

	store := s3Store(t, server.URL, backup.TargetConfig{"bucket": "hubtask"})

	// Two parts and a remainder, so that the last part being shorter than the floor is exercised.
	size := int64(backupstorage.PartSize)*2 + 1024
	written, err := store.Put(t.Context(), "big.hbk", io.LimitReader(zeroes{}, size))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if written != size {
		t.Fatalf("wrote %d bytes, want %d", written, size)
	}

	seen := recorded.seen()
	var begins, parts, completes int
	for _, request := range seen {
		switch {
		case strings.Contains(request, "uploads"):
			begins++
		case strings.Contains(request, "partNumber"):
			parts++
		case strings.HasPrefix(request, "POST"):
			completes++
		}
	}
	if begins != 1 || completes != 1 {
		t.Fatalf("%d begins and %d completions: %v", begins, completes, seen)
	}
	if parts != 3 {
		t.Fatalf("%d parts, want 3 - two full and the remainder", parts)
	}
}

// An upload that fails part way is aborted rather than left behind: a bucket holding parts nobody
// will finish is storage the operator pays for and never sees.
func TestAFailedUploadIsAborted(t *testing.T) {
	var mu sync.Mutex
	var aborted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case req.Method == http.MethodPost && req.URL.Query().Has("uploads"):
			_, _ = w.Write([]byte(
				`<InitiateMultipartUploadResult><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`))
		case req.Method == http.MethodDelete:
			mu.Lock()
			aborted = true
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	store := s3Store(t, server.URL, backup.TargetConfig{"bucket": "hubtask"})

	size := int64(backupstorage.PartSize) + 1
	if _, err := store.Put(t.Context(), "big.hbk", io.LimitReader(zeroes{}, size)); err == nil {
		t.Fatal("a failing upload reported success")
	}

	mu.Lock()
	defer mu.Unlock()
	if !aborted {
		t.Fatal("the upload was left half-finished at the target")
	}
}

// The two answers a caller acts on differently: a key that is not there, and a target that said
// no. Told apart by status, and never by a body - an S3 error document repeats the bucket and the
// key, and a key names a tenant and a moment.
func TestTheTargetsAnswersBecomeThePortsVocabulary(t *testing.T) {
	cases := map[int]func(error) bool{
		http.StatusNotFound:     func(err error) bool { return errors.Is(err, shared.ErrNotFound) },
		http.StatusForbidden:    func(err error) bool { return shared.AsError(err).DetailCode == port.CodeTargetRefused },
		http.StatusUnauthorized: func(err error) bool { return shared.AsError(err).DetailCode == port.CodeTargetRefused },
		http.StatusBadGateway:   func(err error) bool { return shared.AsError(err).DetailCode == port.CodeTargetFailed },
	}

	for status, matches := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`<Error><Key>tenant-a/2026/secret.hbk</Key></Error>`))
		}))

		store := s3Store(t, server.URL, backup.TargetConfig{"bucket": "hubtask"})
		_, err := store.Stat(t.Context(), "a.hbk")
		if err == nil || !matches(err) {
			t.Errorf("status %d became %v", status, err)
		}
		if err != nil && strings.Contains(err.Error(), "tenant-a") {
			t.Errorf("status %d quoted the key from the error document: %v", status, err)
		}
		server.Close()
	}
}

// The guard is the difference between this adapter and the media one, and it is not decorative:
// a backup target's endpoint arrives through the API rather than from the environment.
func TestATargetOnAPrivateNetworkIsBlockedUntilItIsReleased(t *testing.T) {
	server := httptest.NewServer(newRecorder())
	defer server.Close()

	strict := env.OutboundConfig{
		Timeout: time.Second, ConnectTimeout: time.Second,
		MaxResponseBytes: 1 << 20, AllowPrivateNetworks: false,
	}
	store, err := backupstorage.NewS3Store(
		httpclient.NewGuardedClient(strict, httpclient.NewGuard(strict)),
		port.Spec{
			Kind:   backup.KindS3,
			Config: backup.TargetConfig{"bucket": "hubtask", "endpoint": server.URL},
		},
		time.Now)
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}

	if _, err := store.Put(t.Context(), "a.hbk", strings.NewReader("x")); err == nil {
		t.Fatal("a target on loopback was written to without the release")
	}
}

func TestAnEndpointThatIsNotOneIsRefused(t *testing.T) {
	for _, endpoint := range []string{"not a url", "ftp://files.example.org", "file:///etc"} {
		_, err := backupstorage.NewS3Store(guardedClient(), port.Spec{
			Kind:   backup.KindS3,
			Config: backup.TargetConfig{"bucket": "hubtask", "endpoint": endpoint},
		}, time.Now)
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("the endpoint %q was accepted: %v", endpoint, err)
		}
	}
}

// zeroes is an endless stream. io.LimitReader over it is how a large archive is produced without
// holding one.
type zeroes struct{}

func (zeroes) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}
