// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The libraries the page's blocks load on demand, for an application to fetch ahead: a page opened
 * offline draws its formulas, colours and diagrams from that copy. Its own entry point, so naming
 * them pulls nothing of the editor into a first screen. Mermaid's own diagram kinds stay lazy.
 */
export const blockLibraries = {
  katex: () => import('katex'),
  lowlight: () => import('lowlight'),
  mermaid: () => import('mermaid'),
};
