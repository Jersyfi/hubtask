// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
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
	// Credit names who made an image library's picture, shown at the cover; empty otherwise.
	Credit string
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

// MaxCreditLength bounds the name of whoever made an image library's picture, as the library gives it.
const MaxCreditLength = 120

// WithCredit names who made the picture, for an image taken from an image library (UC-DOC-16
// check 5); only an image carries one, and an empty credit names nobody.
func (c Cover) WithCredit(credit string) (Cover, error) {
	credit = strings.TrimSpace(credit)
	if credit == "" {
		c.Credit = ""
		return c, nil
	}
	if c.Source != CoverImage {
		return Cover{}, invalid("/cover/credit", "pages.cover_credit_not_image")
	}
	if utf8.RuneCountInString(credit) > MaxCreditLength || strings.ContainsAny(credit, "\n\r\t") {
		return Cover{}, invalid("/cover/credit", "pages.cover_credit_malformed")
	}
	c.Credit = credit
	return c, nil
}

// CoverImagesKey is the workspace's choice of an image library offered as a cover's fourth source
// (E-18): none by default, since every search leaves the installation.
const CoverImagesKey setting.Key = "knowledge.cover_images"

// The choices of CoverImagesKey: none, or the one library Hubtask can ask.
const (
	CoverImagesOff         = "OFF"
	CoverImagesOpenLibrary = "OPEN_LIBRARY"
)

// CoverImagesDefinition declares the choice: off by default; the installation and a plan may set
// and lock it, the workspace chooses inside that.
func CoverImagesDefinition() setting.Definition {
	return setting.Definition{
		Key: CoverImagesKey, Kind: setting.KindChoice,
		Options:  []string{CoverImagesOpenLibrary, CoverImagesOff},
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayReplace, Default: CoverImagesOff,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
	}
}

// CoverImagesOf reads the resolved choice; anything but a known library is off.
func CoverImagesOf(r setting.Resolution) string {
	if chosen, _ := r.Effective.(string); chosen == CoverImagesOpenLibrary {
		return chosen
	}
	return CoverImagesOff
}
