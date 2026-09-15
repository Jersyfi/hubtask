// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared_test

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

func TestNFCTrimsAndComposesThroughThePort(t *testing.T) {
	got, err := shared.NFC("  Café  ", text.Composing{})
	if err != nil {
		t.Fatalf("normalising: %v", err)
	}
	if got != "Café" {
		t.Errorf("got %q, want the composed form", got)
	}
}

// ASCII is in normal form whatever happens, so a caller with no port loses nothing; anything else
// is refused rather than stored as it arrived, and as a defect of the installation rather than of
// the input - the client typed a perfectly good title.
func TestWithoutAPortOnlyASCIIPasses(t *testing.T) {
	got, err := shared.NFC("  Buy milk ", nil)
	if err != nil || got != "Buy milk" {
		t.Fatalf("ASCII without a port: %q, %v", got, err)
	}

	_, err = shared.NFC("Café", nil)
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.Category != shared.CategoryInternal ||
		domainErr.DetailCode != "text.normalizer_missing" {
		t.Fatalf("non-ASCII without a port: %v, want text.normalizer_missing", err)
	}
}
