// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package webui_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Jersyfi/hubtask/presentation/rest"
	"github.com/Jersyfi/hubtask/presentation/webui"
)

// bundle is what a real build produces: a document, a content-hashed asset, and one file whose
// name is stable because something outside the application asks for it by name.
func bundle() fstest.MapFS {
	return fstest.MapFS{
		"index.html":                {Data: []byte("<!DOCTYPE html><title>Hubtask</title>")},
		"assets/index-CBRxRnMw.js":  {Data: []byte("console.log(1)")},
		"assets/index-DIwrknDs.css": {Data: []byte(":root{}")},
		"favicon.ico":               {Data: []byte("icon")},
	}
}

func handler(t *testing.T) webui.Handler {
	t.Helper()
	h, err := webui.NewHandler(bundle(), rest.WriteSecurityHeaders)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	return h
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	return recorder
}

func TestTheDocumentIsRevalidatedAndTheHashedAssetIsNot(t *testing.T) {
	t.Parallel()
	h := handler(t)

	// The pairing of the two is what makes an update take effect on the next reload while a
	// browser still gets to keep the bundle. Either one alone is wrong.
	if got := get(t, h, "/").Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("index.html Cache-Control = %q, want no-cache", got)
	}
	asset := get(t, h, "/assets/index-CBRxRnMw.js").Header().Get("Cache-Control")
	if !strings.Contains(asset, "immutable") {
		t.Errorf("hashed asset Cache-Control = %q, want immutable", asset)
	}
}

func TestAFileWithAStableNameIsNeverImmutable(t *testing.T) {
	t.Parallel()

	// A year of `immutable` on a name that does not change is a file nobody can take back for a
	// year - which is how a wrong logo becomes permanent.
	got := get(t, handler(t), "/favicon.ico").Header().Get("Cache-Control")
	if strings.Contains(got, "immutable") {
		t.Errorf("favicon.ico Cache-Control = %q, want revalidation", got)
	}
}

func TestAnUnchangedReloadCostsA304(t *testing.T) {
	t.Parallel()
	h := handler(t)

	etag := get(t, h, "/").Header().Get("Etag")
	if etag == "" {
		t.Fatal("no Etag on the document - `no-cache` without a validator re-downloads every time")
	}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	request.Header.Set("If-None-Match", etag)
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusNotModified)
	}
}

func TestAnApplicationRouteResolvesToTheDocument(t *testing.T) {
	t.Parallel()

	// The client owns its own routes, so a deep link must survive a reload rather than 404.
	recorder := get(t, handler(t), "/containers/01JB/items/01JC")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "<title>Hubtask</title>") {
		t.Error("a route that the application owns did not resolve to the document")
	}
}

func TestAMissingAssetIsNotAnsweredWithTheDocument(t *testing.T) {
	t.Parallel()

	// The opposite of the case above, and the reason the two are told apart by the extension.
	// Answering a missing script with HTML makes the application fail later and somewhere else.
	for _, path := range []string{"/assets/index-GONE.js", "/assets/nothing.css"} {
		if recorder := get(t, handler(t), path); recorder.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, recorder.Code)
		}
	}
}

func TestTheSecurityHeadersAreOnEveryAnswer(t *testing.T) {
	t.Parallel()
	h := handler(t)

	for _, path := range []string{"/", "/assets/index-CBRxRnMw.js", "/assets/gone.js", "/deep/link"} {
		header := get(t, h, path).Header()
		if got := header.Get("Content-Security-Policy"); got != webui.ContentSecurityPolicy {
			t.Errorf("GET %s: CSP = %q, want the UI policy", path, got)
		}
		if got := header.Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("GET %s: X-Content-Type-Options = %q", path, got)
		}
	}
}

func TestThePolicyAllowsNoInlineScriptAndNoEval(t *testing.T) {
	t.Parallel()

	// This is a constraint on the framework decision that has not been taken (ADR-0028), so it is
	// asserted rather than left to whoever takes it.
	for _, forbidden := range []string{"unsafe-inline", "unsafe-eval"} {
		if strings.Contains(webui.ContentSecurityPolicy, forbidden) {
			t.Errorf("the UI policy contains %q", forbidden)
		}
	}
	for _, required := range []string{"default-src 'none'", "connect-src 'self'", "frame-ancestors 'none'"} {
		if !strings.Contains(webui.ContentSecurityPolicy, required) {
			t.Errorf("the UI policy is missing %q", required)
		}
	}
}

func TestOnlyReadMethodsAreServed(t *testing.T) {
	t.Parallel()
	h := handler(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), method, "/", nil))
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s / = %d, want 405", method, recorder.Code)
		}
		if got := recorder.Header().Get("Allow"); got != "GET, HEAD" {
			t.Errorf("%s /: Allow = %q", method, got)
		}
	}
}

func TestAPathCannotEscapeTheBundle(t *testing.T) {
	t.Parallel()
	h := handler(t)

	// The bundle is an fs.FS, which refuses an escaping name by itself - this is the test that
	// says so out loud, because the day somebody swaps it for an os.DirFS it stops being free.
	for _, path := range []string{"/../go.mod", "/assets/../../go.mod", "/%2e%2e/go.mod"} {
		recorder := get(t, h, path)
		if strings.Contains(recorder.Body.String(), "module github.com/Jersyfi/hubtask") {
			t.Errorf("GET %s escaped the bundle", path)
		}
	}
}

func TestThePlaceholderIsRecognised(t *testing.T) {
	t.Parallel()

	// What the committed placeholder looks like: a document and nothing else.
	if !webui.IsPlaceholder(fstest.MapFS{"index.html": {Data: []byte("<!DOCTYPE html>")}}) {
		t.Error("a bundle without assets was not recognised as the placeholder")
	}
	if webui.IsPlaceholder(bundle()) {
		t.Error("a real bundle was taken for the placeholder")
	}
}

func TestTheEmbeddedBundleIsUsable(t *testing.T) {
	t.Parallel()

	// The committed placeholder has to be servable, because it is what every `go build` produces.
	files, err := webui.FS()
	if err != nil {
		t.Fatalf("FS: %v", err)
	}
	h, err := webui.NewHandler(files, rest.WriteSecurityHeaders)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}
	if recorder := get(t, h, "/"); recorder.Code != http.StatusOK {
		t.Errorf("GET / = %d, want 200", recorder.Code)
	}
}
