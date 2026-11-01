// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
)

// With a scanner, no upload is accepted that the daemon would refuse for its size; without one, or
// with a limit the daemon takes, the configured limit stands.
func TestUploadsAreCappedToTheScannersStreamLimit(t *testing.T) {
	cfg := envport.Config{
		Request: envport.RequestConfig{MaxUploadBytes: 64 << 20},
		Media:   envport.MediaConfig{ScannerStreamMaxBytes: 25 << 20},
	}
	if got := scannableUploadLimit(cfg, true); got != 25<<20 {
		t.Errorf("with a scanner: %d, want the stream limit", got)
	}
	if got := scannableUploadLimit(cfg, false); got != 64<<20 {
		t.Errorf("without a scanner: %d, want the configured limit", got)
	}
	cfg.Request.MaxUploadBytes = 10 << 20
	if got := scannableUploadLimit(cfg, true); got != 10<<20 {
		t.Errorf("below the stream limit: %d", got)
	}
}
