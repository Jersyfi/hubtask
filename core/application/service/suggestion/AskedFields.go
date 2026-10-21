// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

// onlyAsked keeps the fields of a proposal the person asked about, where they named any (UC-AI-04
// check 6: the title's ✦ asks for a title). The prompt still asks for every field its allow list
// keeps - one prompt per question, and `promptanswers_test` holds the two together - and the
// narrowing happens here, after the allow list, so a field nobody asked about is never stored. An
// answer that proposes none of the asked fields is therefore empty, and ends as one that proposed
// nothing.
func onlyAsked(payload map[string]any, asked []string) map[string]any {
	if len(asked) == 0 || payload == nil {
		return payload
	}
	wanted := make(map[string]bool, len(asked))
	for _, field := range asked {
		wanted[field] = true
	}
	kept := make(map[string]any, len(asked))
	for field, value := range payload {
		if wanted[field] {
			kept[field] = value
		}
	}
	return kept
}
