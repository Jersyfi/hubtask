// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package expression

import (
	"errors"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/expression"
)

// The deadline is the backstop for a shape the depth limit does not catch. Proved with a deadline
// shortened far below an accepted compile rather than with an input slow enough for the real one,
// which would be a test of today's checker speed.
func TestACompilePastItsDeadlineIsRefused(t *testing.T) {
	previous := compileTimeout
	compileTimeout = 0
	t.Cleanup(func() { compileTimeout = previous })

	text := "size([1]" + strings.Repeat(".map(a,[a])", 60) + ") > 0"
	environment := port.Environment{Variables: []port.Variable{{Name: "item", Kind: port.KindMap}}}

	_, err := New().Compile(text, environment, port.Boolean)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}
	var coded *shared.Error
	if !errors.As(err, &coded) || coded.DetailCode != port.CodeTooComplex {
		t.Errorf("error %v, want %s", err, port.CodeTooComplex)
	}
}
