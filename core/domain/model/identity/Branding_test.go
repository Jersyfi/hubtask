// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func brandingDetail(err error) string {
	if err == nil {
		return ""
	}
	return shared.AsError(err).DetailCode
}

// UC-INS-22 checks 3 and 4: the header shows the product's name, the workspace's name or the logo
// alone; the logo alone needs a picture to show and is refused with a code otherwise.
func TestTheHeaderShowsOneOfThreeAndTheLogoAloneNeedsAPicture(t *testing.T) {
	logo := shared.ID("0194f9a8-0000-7000-8000-000000000001")
	cases := []struct {
		name     string
		branding Branding
		header   HeaderShows
		want     string
	}{
		{"the product's name without a picture", Branding{}, HeaderProduct, ""},
		{"the workspace's name without a picture", Branding{}, HeaderWorkspaceName, ""},
		{"the logo alone without a picture", Branding{}, HeaderLogo, "workspace.logo_alone_needs_a_picture"},
		{"the logo alone with a light logo", Branding{LogoLight: logo}, HeaderLogo, ""},
		{"the logo alone with a dark logo only", Branding{LogoDark: logo}, HeaderLogo, ""},
		{"the logo alone with the symbol only, which it falls back to", Branding{Symbol: logo}, HeaderLogo, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.branding.WithHeader(tc.header)
			if detail := brandingDetail(err); detail != tc.want {
				t.Fatalf("answered %q, want %q", detail, tc.want)
			}
			if err == nil && got.Header != tc.header {
				t.Errorf("header %q, want %q", got.Header, tc.header)
			}
		})
	}
}

// UC-INS-22 check 4: removing the last picture while the logo alone is chosen returns the header to
// the product's name in the same value; removing one of two leaves the choice standing.
func TestRemovingTheLastPictureReturnsTheHeaderToTheProduct(t *testing.T) {
	logo := shared.ID("0194f9a8-0000-7000-8000-000000000001")
	symbol := shared.ID("0194f9a8-0000-7000-8000-000000000002")
	cases := []struct {
		name     string
		branding Branding
		remove   Picture
		want     HeaderShows
	}{
		{"the last logo under the logo alone", Branding{Header: HeaderLogo, LogoLight: logo}, PictureLogoLight, HeaderProduct},
		{"the last symbol under the logo alone", Branding{Header: HeaderLogo, Symbol: symbol}, PictureSymbol, HeaderProduct},
		{"one of two under the logo alone", Branding{Header: HeaderLogo, LogoLight: logo, Symbol: symbol}, PictureLogoLight, HeaderLogo},
		{"the last picture under the workspace's name", Branding{Header: HeaderWorkspaceName, Symbol: symbol}, PictureSymbol, HeaderWorkspaceName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.branding.WithPicture(tc.remove, "")
			if got.Header != tc.want {
				t.Errorf("header %q, want %q", got.Header, tc.want)
			}
			if !got.Of(tc.remove).IsZero() {
				t.Errorf("%s still set", tc.remove)
			}
		})
	}
}

func TestEachPictureIsSetInItsOwnPlace(t *testing.T) {
	id := shared.ID("0194f9a8-0000-7000-8000-000000000003")
	for _, picture := range Pictures() {
		got := Branding{}.WithPicture(picture, id)
		for _, other := range Pictures() {
			if (other == picture) != (got.Of(other) == id) {
				t.Errorf("setting %s moved %s", picture, other)
			}
		}
		if got.Header != HeaderProduct {
			t.Errorf("a first picture chose %q", got.Header)
		}
	}
}

// A choice nobody made reads as the product's name, and so does the logo alone that a removed
// medium left without a picture.
func TestAStoredBrandingIsSettledBeforeItIsShown(t *testing.T) {
	logo := shared.ID("0194f9a8-0000-7000-8000-000000000001")
	cases := []struct {
		name     string
		branding Branding
		want     HeaderShows
	}{
		{"nothing chosen", Branding{}, HeaderProduct},
		{"a choice this build does not know", Branding{Header: "BANNER"}, HeaderProduct},
		{"the logo alone without a picture", Branding{Header: HeaderLogo}, HeaderProduct},
		{"the logo alone with a logo", Branding{Header: HeaderLogo, LogoDark: logo}, HeaderLogo},
		{"the workspace's name without a picture", Branding{Header: HeaderWorkspaceName}, HeaderWorkspaceName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.branding.Settled().Header; got != tc.want {
				t.Errorf("settled as %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAChoiceOrAPictureNamedWrongIsRefusedWithACode(t *testing.T) {
	if _, err := ParseHeaderShows("BANNER"); brandingDetail(err) != "workspace.header_unknown" {
		t.Errorf("an unknown choice answered %v", err)
	}
	if _, err := ParsePicture("FAVICON"); brandingDetail(err) != "workspace.picture_unknown" {
		t.Errorf("an unknown picture answered %v", err)
	}
	for _, picture := range Pictures() {
		if parsed, err := ParsePicture(string(picture)); err != nil || parsed != picture {
			t.Errorf("%s parsed as %q, %v", picture, parsed, err)
		}
	}
}
