// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a page hands *All contents* when a label is chosen on it (UC-DOC-01 check 9): the label to
 * narrow the search to, taken once. Not the address: a label is the content's words, and a search
 * term never travels in a query string (`security.md` §9).
 */
export const contentsAsk: { label: string } = { label: '' };
