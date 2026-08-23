// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func TestTheGuardSniffsAndReplaysTheWholeStream(t *testing.T) {
	png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x0}, 700)...)

	inspection, err := Inspect(bytes.NewReader(png), "image/png", 1<<20)
	if err != nil {
		t.Fatalf("a valid upload was refused: %v", err)
	}
	if inspection.ContentType != "image/png" {
		t.Errorf("stored type %q", inspection.ContentType)
	}

	replayed, err := io.ReadAll(inspection.Content)
	if err != nil {
		t.Fatalf("consuming the stream: %v", err)
	}
	if !bytes.Equal(replayed, png) {
		t.Fatalf("the stream lost bytes: %d of %d", len(replayed), len(png))
	}
}

func TestASmuggledClaimIsRefused(t *testing.T) {
	html := strings.NewReader("<!DOCTYPE html><script>alert(1)</script>")

	_, err := Inspect(html, "image/png", 1<<20)
	if got := shared.AsError(err).DetailCode; got != "media.type_mismatch" {
		t.Fatalf("refused as %q, want media.type_mismatch", got)
	}
}

// countingReader proves what "refused without allocating it" means: the guard may read up to
// the boundary byte and not one more.
type countingReader struct {
	source io.Reader
	read   int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.source.Read(p)
	c.read += int64(n)
	return n, err
}

func TestTheLimitRefusesAtTheBoundaryByteWhileStreaming(t *testing.T) {
	const limit = 4096
	// An endless stream: if the guard buffered the object to measure it, this test would never
	// return.
	endless := &countingReader{source: io.LimitReader(neverEnding{}, 1<<30)}

	inspection, err := Inspect(endless, "", limit)
	if err != nil {
		t.Fatalf("the guard refused before the limit: %v", err)
	}

	consumed, err := io.Copy(io.Discard, inspection.Content)
	if got := shared.AsError(err).DetailCode; got != "media.too_large" {
		t.Fatalf("the stream ended with %v after %d bytes, want media.too_large", err, consumed)
	}
	if endless.read > limit+1 {
		t.Fatalf("the guard read %d bytes for a limit of %d - it buffered past the boundary",
			endless.read, limit)
	}
}

func TestAnUploadAtTheLimitPasses(t *testing.T) {
	const limit = 1024
	content := bytes.Repeat([]byte{0xAB}, limit)

	inspection, err := Inspect(bytes.NewReader(content), "", limit)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := io.ReadAll(inspection.Content)
	if err != nil {
		t.Fatalf("an upload at exactly the limit was refused: %v", err)
	}
	if len(replayed) != limit {
		t.Fatalf("replayed %d bytes, want %d", len(replayed), limit)
	}
}

func TestAnEmptyUploadIsRefusedByName(t *testing.T) {
	_, err := Inspect(bytes.NewReader(nil), "", 1<<20)
	if got := shared.AsError(err).DetailCode; got != "media.content_required" {
		t.Fatalf("refused as %q", got)
	}
}

func TestAReaderThatFailsIsNotAValidationProblem(t *testing.T) {
	_, err := Inspect(failingReader{}, "", 1<<20)
	if got := shared.AsError(err).DetailCode; got != "media.content_unreadable" {
		t.Fatalf("refused as %q", got)
	}
}

type neverEnding struct{}

func (neverEnding) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("wire torn") }
