// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// Svelte 5 without SvelteKit, on purpose: SvelteKit boots through an injected inline <script>,
// which the CSP of ADR-0028 blocks, and its server machinery would idle behind the embedding
// Go binary anyway (ADR-0030). This file therefore configures only the compiler, not a frame.
export default {
  // TypeScript inside components, through the same esbuild Vite already runs — no extra tool.
  preprocess: vitePreprocess(),
  compilerOptions: {
    // Runes only. Mixed-mode components would reintroduce the implicit reactivity ADR-0030
    // chose runes to avoid.
    runes: true,
  },
};
