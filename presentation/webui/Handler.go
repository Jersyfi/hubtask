// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package webui

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// ContentSecurityPolicy is the policy for the UI origin, decided in ADR-0028.
//
// security.md §9 specifies a `Content-Security-Policy` for the media origin and for nothing else,
// because until now nothing served HTML. The API's own policy - `default-src 'none'` - is right
// for a document that is never a document and fatal for one that is: under it a page may load no
// script, no stylesheet and no font. So the UI gets its own, and the API keeps its own unchanged.
//
// Every source is 'self'. That is the dividend of embedding: the bundle and the API come from one
// origin, so nothing needs a foreign one, and no fetch, font or image has anywhere else to go.
// There is deliberately no 'unsafe-inline' and no 'unsafe-eval' in it - which makes this a
// constraint on the frontend framework that has not been chosen yet, rather than a consequence of
// one that has.
const ContentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	// data: and blob: are for images the application makes itself - a pasted screenshot, a
	// generated avatar. A browser treats them as images, never as script.
	"img-src 'self' data: blob:; " +
	"font-src 'self'; " +
	// The API is the same origin, so this permits exactly the calls the application has to make
	// and nothing else. It is also what stops a compromised dependency from exfiltrating.
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	// An offline-capable client needs a service worker (ADR-0021), and a bundler ships one as a
	// blob: URL.
	"worker-src 'self' blob:; " +
	"base-uri 'none'; " +
	"form-action 'none'; " +
	"frame-ancestors 'none'"

// Cache-Control for the two kinds of file a bundle contains.
//
// The pairing is what makes an update take effect on the next reload while a browser still gets
// to keep the bundle: the document is always revalidated and names the assets of its version, and
// those carry their content hash in the file name, so a changed asset is a different URL and can
// safely be kept for a year.
const (
	immutableCache = "public, max-age=31536000, immutable"
	revalidate     = "no-cache"
)

// contentHashed matches the file name a bundler gives an asset it has hashed: a base name, a
// separator, and a hash before the extension - `index-CBRxRnMw.js`.
//
// Only such a file may be told to cache for a year, and this is why the check is a pattern rather
// than "everything that is not index.html". A favicon or a web manifest has a stable name, so a
// year of `immutable` would make it unchangeable for a year - the classic way to ship a logo one
// cannot take back.
var contentHashed = regexp.MustCompile(`-[A-Za-z0-9_-]{8,}\.[A-Za-z0-9]+$`)

// indexPath is the document every route that is not a file resolves to.
const indexPath = "index.html"

// SecurityHeaderWriter writes the response header set of security.md §9 with the given content
// security policy.
//
// It is an interface rather than a direct call into presentation/rest so that this adapter does
// not depend on that one; the composition root passes rest's implementation, and the header set
// keeps being defined in exactly one place.
type SecurityHeaderWriter func(header http.Header, contentSecurityPolicy string)

// Handler serves the embedded bundle.
//
// Everything it answers is a static file, so it needs no actor, no tenant and no transaction, and
// it sits outside the API's middleware chain: a page load fetches half a dozen assets, and
// spending the anonymous rate limit budget on them would make the first visit the last one.
type Handler struct {
	// Files is the bundle, rooted at its own directory.
	Files fs.FS
	// SecurityHeaders writes the common header set. Required; nil means a UI served without the
	// headers of security.md §9, which is not a configuration this project offers.
	SecurityHeaders SecurityHeaderWriter

	// etags maps a file to its entity tag. Computed once at construction, because the bundle is
	// in the binary and therefore cannot change while the process runs.
	etags map[string]string
}

// NewHandler reads the bundle once and computes an entity tag per file.
//
// The tags are what make `no-cache` on the document cheap. Without a validator, "always
// revalidate" degenerates into "always re-download": an embedded file has no modification time -
// every byte of it was fixed when the binary was linked - so there would be nothing for the
// browser to ask about. With one, a reload that changes nothing costs a 304.
func NewHandler(files fs.FS, headers SecurityHeaderWriter) (Handler, error) {
	if files == nil {
		return Handler{}, fmt.Errorf("webui: no bundle")
	}
	if headers == nil {
		return Handler{}, fmt.Errorf("webui: no security header writer - see security.md §9")
	}

	etags := map[string]string{}
	err := fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, readErr := fs.ReadFile(files, name)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(content)
		// Strong, and short enough to be worth sending: the first 128 bits of a SHA-256 are as
		// unique as anything a cache will ever need to tell two versions of a file apart.
		etags[name] = `"` + base64.RawURLEncoding.EncodeToString(sum[:16]) + `"`
		return nil
	})
	if err != nil {
		return Handler{}, fmt.Errorf("webui: reading the bundle: %w", err)
	}
	return Handler{Files: files, SecurityHeaders: headers, etags: etags}, nil
}

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// The policy is set before anything else, so that it is on the answer whatever happens below,
	// including the 404 and the 405.
	h.SecurityHeaders(w.Header(), ContentSecurityPolicy)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		// No body: the client asked for a file, and this adapter produces no display text
		// (ADR-0011). The API's problem documents are for the API.
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "" || name == "." {
		name = indexPath
	}

	if file, err := h.Files.Open(name); err == nil {
		defer func() { _ = file.Close() }()
		if info, statErr := file.Stat(); statErr == nil && !info.IsDir() {
			h.serve(w, r, name)
			return
		}
	}

	// Not a file. A path with an extension asked for one, so it gets a 404 - falling back to the
	// document there would answer a missing script with HTML, and the application would fail
	// later and somewhere else. A path without one is a route the application owns, and the
	// single-page application resolves it itself once it has loaded (ADR-0028).
	if path.Ext(name) != "" {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	h.serve(w, r, indexPath)
}

func (h Handler) serve(w http.ResponseWriter, r *http.Request, name string) {
	if contentHashed.MatchString(name) {
		w.Header().Set("Cache-Control", immutableCache)
	} else {
		w.Header().Set("Cache-Control", revalidate)
	}
	// Set before serving: net/http answers If-None-Match against whatever Etag is already on the
	// response, which is what turns an unchanged reload into a 304.
	if etag, known := h.etags[name]; known {
		w.Header().Set("Etag", etag)
	}
	http.ServeFileFS(w, r, h.Files, name)
}
