// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Cover is how a card presents itself: a colour token, or an image (domain-model.md §3.4).
//
// Exactly one of the two, matching the kind - the same shape the database CHECK pins (migration
// 0013). The colour is a token rather than a value, for the reason a label's is: theming belongs
// to the client, and a hex value stored here would be a decision the design system cannot
// revisit (ADR-0029). Which media object may stand behind an image cover - READY, of usage
// COVER, of this tenant - is the application's question; this type only holds the reference.
type Cover struct {
	Kind CoverKind
	// ColorToken is set exactly for a COLOR cover.
	ColorToken string
	// MediaID is set exactly for an IMAGE cover.
	MediaID shared.ID
}

// CoverKind is which of the two a cover is.
type CoverKind string

const (
	CoverColor CoverKind = "COLOR"
	CoverImage CoverKind = "IMAGE"
)

// NewCover validates the exactly-one rule.
func NewCover(kind CoverKind, token string, mediaID shared.ID) (Cover, error) {
	switch kind {
	case CoverColor:
		cleaned, err := colorToken(token, "items.cover_token_malformed")
		if err != nil {
			return Cover{}, err
		}
		if cleaned == "" || !mediaID.IsZero() {
			return Cover{}, coverContradiction()
		}
		return Cover{Kind: CoverColor, ColorToken: cleaned}, nil

	case CoverImage:
		if mediaID.IsZero() || token != "" {
			return Cover{}, coverContradiction()
		}
		return Cover{Kind: CoverImage, MediaID: mediaID}, nil

	default:
		return Cover{}, shared.ErrValidation.
			WithDetail("items.cover_kind_unknown").
			WithParams(map[string]string{"value": string(kind)}).
			WithFields(shared.FieldError{Path: "/kind", Code: "items.cover_kind_unknown"})
	}
}

func coverContradiction() error {
	return shared.ErrValidation.
		WithDetail("items.cover_contradictory").
		WithFields(shared.FieldError{Path: "/", Code: "items.cover_contradictory"})
}

// Equal reports whether two covers say the same thing.
func (c Cover) Equal(other Cover) bool { return c == other }
