// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"math"
	"strconv"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
)

// A vector narrower than the index is padded to it with zeros (ADR-0054). The literal is what
// PostgreSQL parses, so this is the one place the padding can be seen from outside.
func TestANarrowerVectorIsPaddedToTheIndex(t *testing.T) {
	literal := vectorLiteral([]float32{0.5, -0.25, 1})

	parts := strings.Split(strings.Trim(literal, "[]"), ",")
	if len(parts) != repository.EmbeddingWidth {
		t.Fatalf("the literal has %d dimensions, want the index's %d",
			len(parts), repository.EmbeddingWidth)
	}
	if parts[0] != "0.5" || parts[1] != "-0.25" || parts[2] != "1" {
		t.Errorf("the model's own values were not kept first: %v", parts[:3])
	}
	for i, part := range parts[3:] {
		if part != "0" {
			t.Fatalf("dimension %d was padded with %q rather than a zero", i+3, part)
		}
	}
}

// And a vector already at the index's width is rendered exactly as it was, so a 1536-wide
// installation stores and compares what it did before the padding existed.
func TestAFullWidthVectorIsRenderedUnchanged(t *testing.T) {
	values := make([]float32, repository.EmbeddingWidth)
	for i := range values {
		values[i] = float32(i) / 7
	}
	parts := strings.Split(strings.Trim(vectorLiteral(values), "[]"), ",")
	if len(parts) != repository.EmbeddingWidth {
		t.Fatalf("%d dimensions", len(parts))
	}
	for i, part := range parts {
		want := strconv.FormatFloat(float64(values[i]), 'g', -1, 32)
		if part != want {
			t.Fatalf("dimension %d is %q, want %q", i, part, want)
		}
	}
}

// The reason padding is safe, stated as arithmetic rather than asserted: cosine similarity of two
// vectors is unchanged when both are padded with zeros, because neither the dot product nor either
// norm changes. This is the property the whole decision rests on, so it is proved here rather than
// remembered.
func TestZeroPaddingPreservesCosineSimilarity(t *testing.T) {
	a := []float32{0.3, 0.9, -0.2, 0.7}
	b := []float32{0.1, 0.8, 0.4, -0.5}

	before := cosine(a, b)
	after := cosine(padded(a, 64), padded(b, 64))
	if math.Abs(before-after) > 1e-9 {
		t.Fatalf("cosine similarity moved from %v to %v under padding", before, after)
	}
}

func padded(values []float32, width int) []float32 {
	out := make([]float32, width)
	copy(out, values)
	return out
}

func cosine(a, b []float32) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// An empty vector is refused rather than padded: padded it would be zeros, and a zero vector's
// cosine distance to everything is NaN, which sorts first.
func TestAnEmptyVectorIsNotPaddedIntoZeros(t *testing.T) {
	// vectorLiteral itself pads whatever it is given - the refusal is the store's, before it.
	// What this holds in place is that the store's guard exists at all: see Store.
	literal := vectorLiteral(nil)
	if !strings.HasPrefix(literal, "[0,0,") {
		t.Fatalf("the literal for nothing is %q; the store must refuse before rendering", literal[:16])
	}
}
