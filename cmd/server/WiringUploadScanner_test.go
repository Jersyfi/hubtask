// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"testing"

	envport "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/infrastructure/clamd"
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

// A scanner named beside an allowlist that does not list it is still dialled: every upload would
// otherwise be held for a refusal nobody sees until A-23. Its host alone is added.
func TestTheNamedScannerPassesTheAllowlist(t *testing.T) {
	cfg := envport.Config{
		Outbound: envport.OutboundConfig{AllowedHosts: []string{"api.mistral.ai", "imap.gmail.com"}},
		Media:    envport.MediaConfig{ScannerAddress: "clamav:3310"},
	}
	built, err := uploadScanner(cfg)
	if err != nil {
		t.Fatal(err)
	}
	scanner, ok := built.(clamd.Scanner)
	if !ok {
		t.Fatalf("built %T", built)
	}
	if err := scanner.Guard.CheckHost("clamav"); err != nil {
		t.Errorf("the named scanner is refused: %v", err)
	}
	if err := scanner.Guard.CheckHost("elsewhere.example.org"); err == nil {
		t.Error("a host neither listed nor named passed")
	}
}
