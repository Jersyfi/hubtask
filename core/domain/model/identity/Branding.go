// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// HeaderShows is what a workspace's header and sign-in card show (UC-INS-22, ADR-0112).
type HeaderShows string

const (
	// HeaderProduct is the product's name, as before any workspace chose: the default.
	HeaderProduct HeaderShows = "PRODUCT"
	// HeaderWorkspaceName is the workspace's own name, beside its symbol where it has one.
	HeaderWorkspaceName HeaderShows = "WORKSPACE_NAME"
	// HeaderLogo is the workspace's own logo alone, or its symbol where it has no logo.
	HeaderLogo HeaderShows = "LOGO"
)

// ParseHeaderShows reads a submitted choice.
func ParseHeaderShows(value string) (HeaderShows, error) {
	switch choice := HeaderShows(value); choice {
	case HeaderProduct, HeaderWorkspaceName, HeaderLogo:
		return choice, nil
	}
	return "", shared.ErrValidation.
		WithDetail("workspace.header_unknown").
		WithParams(map[string]string{"value": value}).
		WithFields(shared.FieldError{Path: "/header", Code: "workspace.header_unknown"})
}

// Picture names one of a workspace's three pictures.
type Picture string

const (
	PictureLogoLight Picture = "LOGO_LIGHT"
	PictureLogoDark  Picture = "LOGO_DARK"
	PictureSymbol    Picture = "SYMBOL"
)

// Pictures is the three, in the order the trail and a screen name them.
func Pictures() []Picture { return []Picture{PictureLogoLight, PictureLogoDark, PictureSymbol} }

// ParsePicture reads a picture named in a request.
func ParsePicture(value string) (Picture, error) {
	for _, picture := range Pictures() {
		if string(picture) == value {
			return picture, nil
		}
	}
	return "", shared.ErrValidation.
		WithDetail("workspace.picture_unknown").
		WithParams(map[string]string{"value": value}).
		WithFields(shared.FieldError{Path: "/picture", Code: "workspace.picture_unknown"})
}

// Field is the picture's name in the trail, as the workspace's answer spells it.
func (p Picture) Field() string {
	switch p {
	case PictureLogoLight:
		return "logo_light_media_id"
	case PictureLogoDark:
		return "logo_dark_media_id"
	}
	return "symbol_media_id"
}

// Branding is a workspace's own name and logo as stored: three optional pictures, each a media
// object of the workspace, and what the header shows. Whether the workspace may show them is the
// sign-in rule's switch `own_branding`, decided above it; this value holds what the workspace chose.
type Branding struct {
	Header    HeaderShows
	LogoLight shared.ID
	LogoDark  shared.ID
	Symbol    shared.ID
}

// Of answers the picture's media object, zero where there is none.
func (b Branding) Of(picture Picture) shared.ID {
	switch picture {
	case PictureLogoLight:
		return b.LogoLight
	case PictureLogoDark:
		return b.LogoDark
	}
	return b.Symbol
}

// HasPicture reports whether any of the three is set.
func (b Branding) HasPicture() bool {
	return !b.LogoLight.IsZero() || !b.LogoDark.IsZero() || !b.Symbol.IsZero()
}

// WithHeader answers the branding with another choice. The logo alone needs a picture to show -
// a logo for either theme, or the symbol it falls back to (the approved draft, G8).
func (b Branding) WithHeader(header HeaderShows) (Branding, error) {
	if header == HeaderLogo && !b.HasPicture() {
		return Branding{}, shared.ErrValidation.
			WithDetail("workspace.logo_alone_needs_a_picture").
			WithFields(shared.FieldError{Path: "/header", Code: "workspace.logo_alone_needs_a_picture"})
	}
	b.Header = header
	return b, nil
}

// Settled answers the branding as it may be shown: a choice nobody made, or one this build does not
// know, is the product's name, and so is the logo alone once no picture is left - which a picture
// whose medium was erased leaves behind without a write here.
func (b Branding) Settled() Branding {
	if _, err := ParseHeaderShows(string(b.Header)); err != nil {
		b.Header = HeaderProduct
	}
	if b.Header == HeaderLogo && !b.HasPicture() {
		b.Header = HeaderProduct
	}
	return b
}

// WithPicture answers the branding with one picture set, zero to remove it. Removing the last
// picture while the header shows the logo alone returns the header to the product's name in the
// same value, so that no stored state asks for a logo that is not there.
func (b Branding) WithPicture(picture Picture, mediaID shared.ID) Branding {
	switch picture {
	case PictureLogoLight:
		b.LogoLight = mediaID
	case PictureLogoDark:
		b.LogoDark = mediaID
	default:
		b.Symbol = mediaID
	}
	if b.Header == HeaderLogo && !b.HasPicture() {
		b.Header = HeaderProduct
	}
	if b.Header == "" {
		b.Header = HeaderProduct
	}
	return b
}
