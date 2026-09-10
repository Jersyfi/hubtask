// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"strings"
	"testing"
)

// The answer shape, at the edges (K-01). Inside the package, because what is being tested is the
// reading of a convention rather than anything the store answers.
//
// A shape written for a model is not JSON - it has ellipses where the values go - and must not be
// read as if it were; and a value that happens to be a string is not a key.
func TestTheAnswerShapeIsReadForItsNamesAndNotParsed(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		instruction string
		want        string
	}{
		{"a shape with ellipses", "Answer:\n\n```\n{\"a\": \"…\", \"b\": 1}\n```", "a,b"},
		{"strings in a list are values", "```\n{\"labels\": [\"one\", \"two\"]}\n```", "labels"},
		{"a nested object keeps its keys to itself",
			"```\n{\"children\": [{\"type\": \"…\", \"title\": \"…\"}]}\n```", "children"},
		{"a language tag on the fence", "```json\n{\"a\": \"…\"}\n```", "a"},
		{"the first block that holds an object",
			"```\nnot an object\n```\n\n```\n{\"a\": \"…\"}\n```", "a"},
		{"prose alone asks for no shape", "Write four short paragraphs.", ""},
		{"a bullet documenting a field is not the shape", "- `title`: a short title.", ""},
		{"a key carrying a quote", "```\n{\"a\\\"b\": \"…\"}\n```", `a"b`},
		{"an unclosed fence documents nothing", "```\n{\"a\": \"…\"}", ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := strings.Join(answerKeys(testCase.instruction), ","); got != testCase.want {
				t.Errorf("read %q, want %q", got, testCase.want)
			}
		})
	}
}
