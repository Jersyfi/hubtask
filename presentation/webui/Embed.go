// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package webui serves the built to-do application out of the binary.
//
// It is an inbound adapter like rest, mcp and calendar (ADR-0001), and it is the thinnest one:
// it answers with bytes and reaches nothing. No use case is registered for it, because serving a
// file is not a business operation - `usecase.Registry` stays the catalogue of things a person or
// an agent can ask the product to do.
//
// Why the bundle is in the binary at all is ADR-0028. In short: the README promises self-hosting
// in two containers, and a UI container plus a proxy would be four - and would make it possible
// to run a UI against an API of a different version. Both come from the same commit here, so that
// cannot happen.
package webui

import (
	"embed"
	"io/fs"
)

// bundle is the built application. `all:` is required rather than decorative: without it `embed`
// skips every file whose name begins with a dot or an underscore, and a bundler is free to emit
// one.
//
// The directory always contains at least the committed placeholder, which is what makes this
// compile - and therefore `go build ./...` succeed - in a checkout where Node.js has never been
// installed (ADR-0028, project-structure.md §6).
//
//go:embed all:dist
var bundle embed.FS

// FS is the bundle rooted at the directory the files actually live in, so that a request for
// `/assets/x.js` is looked up as `assets/x.js` rather than as `dist/assets/x.js`.
func FS() (fs.FS, error) {
	return fs.Sub(bundle, "dist")
}

// IsPlaceholder reports whether the binary was built without a real UI bundle.
//
// The marker is the absence of the asset directory every build produces, not a magic string in
// the HTML: a bundle is a bundle because it has assets, and a check that reads the page would
// break the moment somebody edits a sentence in the placeholder.
func IsPlaceholder(fsys fs.FS) bool {
	entries, err := fs.ReadDir(fsys, "assets")
	return err != nil || len(entries) == 0
}
