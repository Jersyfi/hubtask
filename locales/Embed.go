// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package locales carries the message catalogues as data, and nothing else.
//
// It exists because go:embed cannot climb out of its own directory. The catalogues are
// locales/<tag>.json (i18n-l10n.md §3), en.json being the source language, and anything that has
// to render a message code without a server to ask - the CLI above all - needs them compiled in.
// A copy under infrastructure/ would be a second catalogue to forget, which is the one thing a
// source of truth must not have.
//
// The renderer lives in infrastructure/i18n. This package holds no logic: a package that only
// carries bytes cannot be wrong about them.
package locales

import "embed"

// Files is every catalogue in this directory, by file name. A file system rather than one named
// variable per locale, so that a new language is a file and not a change here (arc42 QS-08): the
// renderer lists the directory and takes the tag from the name.
//
//go:embed *.json
var Files embed.FS
