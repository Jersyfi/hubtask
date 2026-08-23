// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media_test

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The delivery half of T-11: inline is earned by an allowlist, everything else is a download.
func TestOnlyTheImageAllowlistRendersInline(t *testing.T) {
	inline := []string{"image/png", "image/jpeg", "image/gif", "image/webp", "IMAGE/PNG",
		"image/png; some=param"}
	for _, contentType := range inline {
		if got := media.DeliveryFor(contentType); got != media.DispositionInline {
			t.Errorf("%s delivers as %s, want inline", contentType, got)
		}
	}

	attachment := []string{
		"image/svg+xml", "text/html", "text/html; charset=utf-8", "application/pdf",
		"application/octet-stream", "text/xml", "video/mp4", "", "not a type at all",
	}
	for _, contentType := range attachment {
		if got := media.DeliveryFor(contentType); got != media.DispositionAttachment {
			t.Errorf("%s delivers as %s, want a download", contentType, got)
		}
	}
}

// The claim half of T-11: the sniff wins, a lie about an inline-capable type is refused, and a
// sharper non-inline claim over a generic sniff is kept - it cannot buy a rendering path.
func TestTheClaimIsReconciledWithTheBytes(t *testing.T) {
	cases := []struct {
		name     string
		claimed  string
		sniffed  string
		want     string
		mismatch bool
	}{
		{name: "no claim keeps the sniff", sniffed: "image/png", want: "image/png"},
		{
			name:    "an agreeing claim keeps the sniff",
			claimed: "image/png", sniffed: "image/png", want: "image/png",
		},
		{
			name:    "parameters and case do not make a disagreement",
			claimed: "TEXT/HTML", sniffed: "text/html; charset=utf-8", want: "text/html",
		},
		{
			name:    "HTML claimed as an image is the smuggling the matrix catches",
			claimed: "image/png", sniffed: "text/html; charset=utf-8", mismatch: true,
		},
		{
			name:    "an image claimed over unrecognised bytes is refused",
			claimed: "image/jpeg", sniffed: "application/octet-stream", mismatch: true,
		},
		{
			name:    "SVG sharpens the XML sniff and stays a download",
			claimed: "image/svg+xml", sniffed: "text/xml; charset=utf-8", want: "image/svg+xml",
		},
		{
			name:    "a PDF claim over generic bytes is kept",
			claimed: "application/pdf", sniffed: "application/octet-stream",
			want: "application/pdf",
		},
		{
			name:    "two specific types that disagree are a lie either way",
			claimed: "application/pdf", sniffed: "image/png", mismatch: true,
		},
		{
			name:    "an unreadable claim is refused rather than guessed at",
			claimed: ";;;", sniffed: "image/png", mismatch: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored, err := media.AcceptClaim(tc.claimed, tc.sniffed)
			if tc.mismatch {
				if err == nil {
					t.Fatalf("accepted as %q, want media.type_mismatch", stored)
				}
				if got := shared.AsError(err).DetailCode; got != "media.type_mismatch" {
					t.Fatalf("refused as %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
			if stored != tc.want {
				t.Errorf("stored %q, want %q", stored, tc.want)
			}
		})
	}
}

func TestSVGNeverRendersInlineWhicheverWayItArrives(t *testing.T) {
	// Claimed honestly, sharpened from the XML sniff - and still a download (T-11: SVG is
	// rasterised or served as a download; rasterising is nobody's job yet).
	stored, err := media.AcceptClaim("image/svg+xml", "text/xml; charset=utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if media.DeliveryFor(stored) != media.DispositionAttachment {
		t.Fatal("an SVG earned an inline rendering path")
	}
}
