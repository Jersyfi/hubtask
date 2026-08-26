// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	port "github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// s3Against builds the adapter pointed at a test server.
func s3Against(t *testing.T, target string, pathStyle bool) *S3Storage {
	t.Helper()

	store, err := NewS3Storage(env.StorageConfig{
		Kind: env.StorageS3, Endpoint: target, Region: "eu-central-1", Bucket: "hubtask-media",
		AccessKey: secret.New("access"), SecretKey: secret.New("secret"),
		UsePathStyle: pathStyle,
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("building the adapter: %v", err)
	}
	return store
}

func TestTheTwoAddressingStylesBuildTheRightURLs(t *testing.T) {
	pathStyle := s3Against(t, "http://minio.internal:9000", true)
	url, err := pathStyle.objectURL("media/one")
	if err != nil || url != "http://minio.internal:9000/hubtask-media/media/one" {
		t.Errorf("path-style url = %q (%v)", url, err)
	}

	virtual := s3Against(t, "http://s3.example", false)
	url, err = virtual.objectURL("media/one")
	if err != nil || url != "http://hubtask-media.s3.example/media/one" {
		t.Errorf("virtual-host url = %q (%v)", url, err)
	}

	for _, key := range []string{"", "/lead", "a//b", "a/../b"} {
		if _, err := pathStyle.objectURL(key); shared.AsError(err).DetailCode != "storage.key_invalid" {
			t.Errorf("key %q was accepted: %v", key, err)
		}
	}
}

func TestTheAnswersMapToTheSharedErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "missing"):
			http.Error(w, "<Error><Key>secret-key-name</Key></Error>", http.StatusNotFound)
		case strings.Contains(r.URL.Path, "broken"):
			http.Error(w, "boom", http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "forbidden"):
			http.Error(w, "denied", http.StatusForbidden)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()
	store := s3Against(t, server.URL, true)

	if _, err := store.Get(t.Context(), "media/missing"); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing object answered %v", err)
	}

	_, err := store.Get(t.Context(), "media/broken")
	if got := shared.AsError(err).DetailCode; got != "dependency.unavailable" {
		t.Errorf("a 500 answered %q", got)
	}

	_, err = store.Get(t.Context(), "media/forbidden")
	if got := shared.AsError(err).DetailCode; got != "storage.io_failed" {
		t.Errorf("a 403 answered %q", got)
	}
	// T-18: the S3 error body can carry the key and the bucket; the coded error must not.
	if err != nil && strings.Contains(err.Error(), "secret-key-name") {
		t.Errorf("the error quotes the endpoint's body: %v", err)
	}

	if err := store.Delete(t.Context(), "media/missing"); err != nil {
		t.Errorf("deleting what is gone must succeed: %v", err)
	}
}

func TestARedirectIsRefusedNotFollowed(t *testing.T) {
	var followed bool
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
	}))
	defer evil.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL, http.StatusMovedPermanently)
	}))
	defer server.Close()

	store := s3Against(t, server.URL, true)
	_, err := store.Get(t.Context(), "media/one")
	if err == nil {
		t.Fatal("a redirect was accepted")
	}
	if followed {
		t.Fatal("the signed request followed the redirect - credentials went with it")
	}
}

func TestAPutStreamsAndAGuardRefusalSurvivesTheWire(t *testing.T) {
	var received int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read, _ := io.Copy(io.Discard, r.Body)
		received = read
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store := s3Against(t, server.URL, true)

	content := strings.Repeat("x", 4096)
	if err := store.Put(t.Context(), port.Upload{
		Key: "media/ok", Content: strings.NewReader(content), Size: int64(len(content)),
		ContentType: "application/pdf",
	}); err != nil {
		t.Fatalf("putting: %v", err)
	}
	if received != 4096 {
		t.Errorf("the endpoint received %d bytes", received)
	}

	// The guard refuses mid-stream; the refusal must come back out as itself rather than as a
	// generic transport failure.
	inspection, err := Inspect(io.LimitReader(neverEnding{}, 1<<20), "", 1024)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Put(t.Context(), port.Upload{
		Key: "media/too-big", Content: inspection.Content, Size: -1, ContentType: "text/plain",
	})
	if got := shared.AsError(err).DetailCode; got != "media.too_large" {
		t.Fatalf("the refusal arrived as %q (%v)", got, err)
	}
}
