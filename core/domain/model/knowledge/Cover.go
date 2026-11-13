// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"regexp"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CoverSource is where a page's cover comes from (E-18, UC-DOC-16).
type CoverSource string

const (
	// CoverGradient is one of the design system's gradients, named by its key.
	CoverGradient CoverSource = "GRADIENT"
	// CoverGallery is a picture of the gallery the product ships, named by its key.
	CoverGallery CoverSource = "GALLERY"
	// CoverImage is an image of the media context, named by its id.
	CoverImage CoverSource = "IMAGE"
)

// MaxCoverPosition is the lowest a cover's picture can be shown from: 0 shows its top, 100 its foot.
const MaxCoverPosition = 100

// A gradient's or a gallery picture's key: what the clients draw it by, never a colour or a path.
var coverKey = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Cover is a page's picture over its content. Which keys exist is the clients' vocabulary; the
// domain holds only their shape, so a key a client does not know draws as no cover there.
type Cover struct {
	Source   CoverSource
	Key      string
	Position int
}

// NewCover checks a cover as asked for.
func NewCover(source CoverSource, key string, position int) (Cover, error) {
	switch source {
	case CoverGradient, CoverGallery:
		if !coverKey.MatchString(key) {
			return Cover{}, invalid("/cover/key", "pages.cover_key_malformed")
		}
	case CoverImage:
		id, err := shared.ParseID(key)
		if err != nil || id.IsZero() {
			return Cover{}, invalid("/cover/key", "pages.cover_key_malformed")
		}
		key = id.String()
	default:
		return Cover{}, invalid("/cover/source", "pages.cover_source_unknown")
	}
	if position < 0 || position > MaxCoverPosition {
		return Cover{}, invalid("/cover/position", "pages.cover_position_out_of_range")
	}
	return Cover{Source: source, Key: key, Position: position}, nil
}
