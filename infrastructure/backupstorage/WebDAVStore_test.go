// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backupstorage_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/backupstorage"
)

// What is here is the protocol shape BK-1 cannot see: that the collections above a key are made
// before the write, that a listing recurses at depth one rather than asking for infinite depth,
// and that the credential is in a header rather than in the URL. That a real server accepts all of
// it is the conformance suite's job.

// davServer is a WebDAV server that keeps its files in a map. Small enough to be honest about
// what the adapter sends, which is the point.
type davServer struct {
	mu          sync.Mutex
	files       map[string][]byte
	collections map[string]bool
	seen        []string
	authorized  string
	depths      []string
}

func newDavServer() *davServer {
	return &davServer{files: map[string][]byte{}, collections: map[string]bool{"": true}}
}

func (d *davServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	key := strings.Trim(strings.TrimPrefix(req.URL.Path, "/dav/"), "/")

	d.mu.Lock()
	d.seen = append(d.seen, req.Method+" "+key)
	d.authorized = req.Header.Get("Authorization")
	if depth := req.Header.Get("Depth"); depth != "" {
		d.depths = append(d.depths, depth)
	}
	d.mu.Unlock()

	switch req.Method {
	case "MKCOL":
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.collections[key] {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		d.collections[key] = true
		w.WriteHeader(http.StatusCreated)
	case http.MethodPut:
		body, _ := io.ReadAll(req.Body)
		d.mu.Lock()
		defer d.mu.Unlock()
		if parent := parentOf(key); !d.collections[parent] {
			// The behaviour that makes the MKCOL walk necessary rather than polite.
			w.WriteHeader(http.StatusConflict)
			return
		}
		d.files[key] = body
		w.WriteHeader(http.StatusCreated)
	case http.MethodGet:
		d.mu.Lock()
		body, found := d.files[key]
		d.mu.Unlock()
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	case http.MethodHead:
		d.mu.Lock()
		body, found := d.files[key]
		d.mu.Unlock()
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Header().Set("Last-Modified", "Wed, 26 Aug 2026 09:00:00 GMT")
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		d.mu.Lock()
		defer d.mu.Unlock()
		if _, found := d.files[key]; !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(d.files, key)
		w.WriteHeader(http.StatusNoContent)
	case "PROPFIND":
		d.propfind(w, req, key)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// propfind answers exactly one level, the way Apache does with infinite depth switched off.
func (d *davServer) propfind(w http.ResponseWriter, req *http.Request, key string) {
	if req.Header.Get("Depth") != "1" {
		// An adapter asking for infinite depth would work against Nextcloud and fail here, which
		// is the worst kind of adapter.
		w.WriteHeader(http.StatusForbidden)
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.collections[key] {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	var body strings.Builder
	body.WriteString(`<?xml version="1.0"?><multistatus xmlns="DAV:">`)
	body.WriteString(responseFor("/dav/"+key, true, 0))

	for name := range d.collections {
		if name != key && parentOf(name) == key {
			body.WriteString(responseFor("/dav/"+name, true, 0))
		}
	}
	for name, content := range d.files {
		if parentOf(name) == key {
			body.WriteString(responseFor("/dav/"+name, false, len(content)))
		}
	}
	body.WriteString(`</multistatus>`)

	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, body.String())
}

func responseFor(href string, collection bool, size int) string {
	resourceType := ""
	if collection {
		resourceType = "<collection/>"
	}
	return `<response><href>` + href + `</href><propstat><prop>` +
		`<getcontentlength>` + fmt.Sprint(size) + `</getcontentlength>` +
		`<getlastmodified>Wed, 26 Aug 2026 09:00:00 GMT</getlastmodified>` +
		`<resourcetype>` + resourceType + `</resourcetype>` +
		`</prop><status>HTTP/1.1 200 OK</status></propstat></response>`
}

func parentOf(key string) string {
	index := strings.LastIndex(key, "/")
	if index < 0 {
		return ""
	}
	return key[:index]
}

func davStore(t *testing.T, base string) *backupstorage.WebDAVStore {
	t.Helper()
	store, err := backupstorage.NewWebDAVStore(guardedClient(), port.Spec{
		Kind:        backup.KindWebDAV,
		Config:      backup.TargetConfig{"url": base + "/dav/", "username": "hubtask"},
		Credentials: map[string]secret.Secret{"password": secret.New("the-nas-password")},
	})
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}
	return store
}

func TestAnArchiveRoundTripsThroughItsCollections(t *testing.T) {
	dav := newDavServer()
	server := httptest.NewServer(dav)
	defer server.Close()
	store := davStore(t, server.URL)

	written, err := store.Put(t.Context(), "2026/08/a.hbk", strings.NewReader("the archive"))
	if err != nil {
		t.Fatalf("writing: %v", err)
	}
	if written != int64(len("the archive")) {
		t.Fatalf("wrote %d bytes", written)
	}

	// The collections above the key were made first. No server creates them on the way, and a
	// PUT into a collection that is not there answers 409.
	dav.mu.Lock()
	seen := strings.Join(dav.seen, " ")
	authorization := dav.authorized
	dav.mu.Unlock()
	if !strings.Contains(seen, "MKCOL 2026") || !strings.Contains(seen, "MKCOL 2026/08") {
		t.Fatalf("the collections were not prepared: %s", seen)
	}

	// The credential is a header. A URL carrying one is a credential in every log line and every
	// metric label that ever names the target.
	if !strings.HasPrefix(authorization, "Basic ") {
		t.Fatalf("the credential did not travel as a header: %q", authorization)
	}

	content, err := store.Get(t.Context(), "2026/08/a.hbk")
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	defer func() { _ = content.Close() }()
	read, _ := io.ReadAll(content)
	if string(read) != "the archive" {
		t.Fatalf("read %q", read)
	}
}

// A listing recurses at depth one. Apache refuses infinite depth by default, and this server
// answers 403 to anything else - which is what makes the assertion worth something.
func TestAListingRecursesRatherThanAskingForInfiniteDepth(t *testing.T) {
	dav := newDavServer()
	server := httptest.NewServer(dav)
	defer server.Close()
	store := davStore(t, server.URL)

	for _, key := range []string{"2026/08/a.hbk", "2026/08/b.hbk", "2026/09/c.hbk"} {
		if _, err := store.Put(t.Context(), key, strings.NewReader(key)); err != nil {
			t.Fatalf("writing %s: %v", key, err)
		}
	}

	entries, err := store.List(t.Context(), "")
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("%d entries: %v", len(entries), entries)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Key, "2026/") {
			t.Fatalf("the key %q is not the whole key", entry.Key)
		}
		if entry.Size == 0 || entry.ModifiedAt.IsZero() {
			t.Fatalf("the entry %v carries no size or no date", entry)
		}
	}

	dav.mu.Lock()
	depths := append([]string(nil), dav.depths...)
	dav.mu.Unlock()
	for _, depth := range depths {
		if depth != "1" {
			t.Fatalf("a listing asked for depth %q", depth)
		}
	}

	// And a prefix narrows it.
	narrowed, err := store.List(t.Context(), "2026/09")
	if err != nil || len(narrowed) != 1 {
		t.Fatalf("the narrowed listing is %v (%v)", narrowed, err)
	}
}

func TestAFreshCollectionListsEmptyRatherThanFailing(t *testing.T) {
	server := httptest.NewServer(newDavServer())
	defer server.Close()
	store := davStore(t, server.URL)

	entries, err := store.List(t.Context(), "2026/08")
	if err != nil {
		t.Fatalf("listing a collection nothing was written into: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("it listed %v", entries)
	}
}

func TestTheMissingArchiveAndTheRefusalAreToldApart(t *testing.T) {
	dav := newDavServer()
	server := httptest.NewServer(dav)
	defer server.Close()
	store := davStore(t, server.URL)

	if _, err := store.Stat(t.Context(), "nothing.hbk"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a missing archive is reported as %v", err)
	}
	if _, err := store.Get(t.Context(), "nothing.hbk"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("reading a missing archive is reported as %v", err)
	}
	// Deleting what is not there is the state the caller asked for.
	if err := store.Delete(t.Context(), "nothing.hbk"); err != nil {
		t.Fatalf("deleting nothing: %v", err)
	}

	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer refusing.Close()

	_, err := davStore(t, refusing.URL).Stat(t.Context(), "a.hbk")
	if shared.AsError(err).DetailCode != port.CodeTargetRefused {
		t.Fatalf("a refusal is reported as %v", err)
	}
}

// A URL that carries its own credential is stripped: it would otherwise appear in every log line,
// metric label and audit entry that names the target.
func TestACredentialInTheUrlIsNotKept(t *testing.T) {
	dav := newDavServer()
	server := httptest.NewServer(dav)
	defer server.Close()

	address := strings.Replace(server.URL, "http://", "http://someone:hunter2@", 1)
	store, err := backupstorage.NewWebDAVStore(guardedClient(), port.Spec{
		Kind:   backup.KindWebDAV,
		Config: backup.TargetConfig{"url": address + "/dav/"},
	})
	if err != nil {
		t.Fatalf("opening the target: %v", err)
	}

	if _, err := store.Put(t.Context(), "a.hbk", strings.NewReader("x")); err != nil {
		t.Fatalf("writing: %v", err)
	}
	dav.mu.Lock()
	defer dav.mu.Unlock()
	if dav.authorized != "" {
		t.Fatalf("the credential from the URL was used as one: %q", dav.authorized)
	}
}

func TestAUrlThatIsNotOneIsRefused(t *testing.T) {
	for _, address := range []string{"", "not a url", "ftp://nas.local/backups", "/relative"} {
		_, err := backupstorage.NewWebDAVStore(guardedClient(), port.Spec{
			Kind: backup.KindWebDAV, Config: backup.TargetConfig{"url": address},
		})
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("the address %q was accepted: %v", address, err)
		}
	}
}
