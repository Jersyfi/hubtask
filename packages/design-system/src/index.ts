// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What `@hubtask/design-system/components` is. One entry point rather than a subpath per file:
// fifty export lines in package.json is fifty chances for one of them to be wrong, and a component
// that is in the tree but not here is a component nobody outside the package can reach.
//
// `.` stays the tokens (ADR-0029). A consumer imports values from one place and components from
// the other, which is the same separation the package already has on disk.

export { default as Box } from './Box.svelte';
export { default as Icon } from './Icon.svelte';
export { default as Inline } from './Inline.svelte';
export { default as Stack } from './Stack.svelte';
export { default as VisuallyHidden } from './VisuallyHidden.svelte';

export type { Align, Justify, Space } from './space.ts';

export { BASE_ICONS, CUSTOM_ICONS, ICON_NAMES, ICONS, type IconName, type IconNode } from './icons/index.ts';

export {
  DISMISSIBLE_LAYERS,
  LayerRegister,
  handleEscape,
  layers,
  type DismissibleLayer,
  type LayerEntry,
  type LayerHandle,
} from './layers.ts';
