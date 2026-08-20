// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package locales carries the source message catalogue as data, and nothing else.
//
// It exists because go:embed cannot climb out of its own directory. The catalogue is
// locales/en.json (i18n-l10n.md §3) and it is the source language, so anything that has to render
// a message code without a server to ask - the CLI above all - needs it compiled in. A copy under
// infrastructure/ would be a second catalogue to forget, which is the one thing a source of truth
// must not have.
//
// The renderer lives in infrastructure/i18n. This package holds no logic: a package that only
// carries bytes cannot be wrong about them.
package locales

import _ "embed"

// English is locales/en.json verbatim. Bytes rather than a parsed map, so that the parse and its
// error handling stay in one place.
//
//go:embed en.json
var English []byte
