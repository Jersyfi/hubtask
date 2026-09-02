// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { mount } from 'svelte';

// The generated stylesheet and the self-hosted fonts. Both come from `pnpm build`, which the
// `workbench` script runs first - a workbench rendering against stale tokens would be a
// workbench that agrees with itself and with nothing else.
import '../dist/fonts.css';
import '../dist/tokens.css';
import './chrome/chrome.css';

import Workbench from './Workbench.svelte';

// The chrome follows the system preference; the stage sets its own data-theme per pane, which is
// the theme axis. There is no :root fallback in the stylesheet on purpose (ADR-0029), so this
// line is load-bearing rather than a nicety.
const dark = window.matchMedia('(prefers-color-scheme: dark)');
const applyChromeTheme = () => {
  document.documentElement.dataset.theme = dark.matches ? 'dark' : 'light';
};
applyChromeTheme();
dark.addEventListener('change', applyChromeTheme);

const target = document.getElementById('workbench');
if (!target) throw new Error('#workbench is missing from index.html');
mount(Workbench, { target });
