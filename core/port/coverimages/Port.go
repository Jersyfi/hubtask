// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package coverimages is the port to an image library a page's cover may take a picture from
// (E-18, UC-DOC-16 check 5). The server asks it, never the browser: the reader's address and what
// they search reach the library from the installation alone, through the guarded client (rule 6).
package coverimages

import "context"

// Picture is one picture a search found: its id at the library, who made it, and a small preview.
type Picture struct {
	ID string
	// Creator is who made it, as the library names them; the cover names them (credit).
	Creator string
	// Preview is a small rendition, already fetched, with its media type.
	Preview     []byte
	PreviewType string
}

// Image is a picture's larger rendition, fetched for a cover.
type Image struct {
	Content     []byte
	ContentType string
	Creator     string
}

// Library searches pictures and fetches one. Each call carries its own deadline (rule 7); an
// error never holds the query or the library's key (rule 10).
type Library interface {
	Search(ctx context.Context, query string) ([]Picture, error)
	Fetch(ctx context.Context, id string) (Image, error)
}
