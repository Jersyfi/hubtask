// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// The same compiler settings as apps/webapp, for the same reasons (ADR-0030): a component that
// compiles one way here and another way there is a component the workbench cannot vouch for.
// This file configures the compiler only - the workbench's frame is workbench/vite.config.ts.
export default {
  preprocess: vitePreprocess(),
  compilerOptions: {
    // Runes only. Components in src/ are consumed by both apps; mixed-mode reactivity would be a
    // second dialect in a package whose whole purpose is that there is only one of everything.
    runes: true,
  },
};
