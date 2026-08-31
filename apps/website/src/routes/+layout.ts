// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Every page is prerendered (ADR-0030), and nothing hydrates: a brochure has no state to take
// over, so the built output carries no script at all - which is also what makes "no inline
// script" a property of the configuration rather than a hope, and build/check-static.js proves
// it on every build.
export const prerender = true;
export const csr = false;
// Directory-per-page output (`impressum/index.html` rather than `impressum.html`): the webspace
// is a plain Apache with no content negotiation to rely on, and a directory with an index is the
// one shape every static server serves at `/impressum/`.
export const trailingSlash = 'always';
