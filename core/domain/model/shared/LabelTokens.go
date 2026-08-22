// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Code generated from packages/design-system/tokens/tokens.json. DO NOT EDIT.

package shared

// LabelToken is the name of a label colour. The domain stores the name, never the colour: what a
// token looks like is a question for the theme the client is rendering in, and answering it here
// would put display information in the backend (ADR-0011, ADR-0029).
type LabelToken string

// The 10 label colours the design system defines.
const (
	LabelTokenSlate   LabelToken = "slate"
	LabelTokenBlue    LabelToken = "blue"
	LabelTokenTeal    LabelToken = "teal"
	LabelTokenGreen   LabelToken = "green"
	LabelTokenLime    LabelToken = "lime"
	LabelTokenAmber   LabelToken = "amber"
	LabelTokenOrange  LabelToken = "orange"
	LabelTokenRed     LabelToken = "red"
	LabelTokenMagenta LabelToken = "magenta"
	LabelTokenViolet  LabelToken = "violet"
)

// LabelTokens lists every label colour, in the order the design system declares them. A client
// that offers a colour picker gets the order from here rather than sorting the names.
var LabelTokens = []LabelToken{
	LabelTokenSlate,
	LabelTokenBlue,
	LabelTokenTeal,
	LabelTokenGreen,
	LabelTokenLime,
	LabelTokenAmber,
	LabelTokenOrange,
	LabelTokenRed,
	LabelTokenMagenta,
	LabelTokenViolet,
}

// IsLabelToken reports whether a name is one of the label colours.
//
// The check is a membership test over a generated list rather than a hand-written switch, so that
// an eleventh colour added to tokens.json cannot be accepted by the frontend and refused here.
func IsLabelToken(name string) bool {
	for _, token := range LabelTokens {
		if string(token) == name {
			return true
		}
	}
	return false
}
