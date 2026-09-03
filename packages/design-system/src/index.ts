// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// What `@hubtask/design-system/components` is. One entry point rather than a subpath per file:
// fifty export lines in package.json is fifty chances for one of them to be wrong, and a component
// that is in the tree but not here is a component nobody outside the package can reach.
//
// `.` stays the tokens (ADR-0029). A consumer imports values from one place and components from
// the other, which is the same separation the package already has on disk.

// Wave 0 - the primitives.
export { default as Box } from './Box.svelte';
export { default as Inline } from './Inline.svelte';
export { default as Stack } from './Stack.svelte';
export { default as VisuallyHidden } from './VisuallyHidden.svelte';

// Wave 1 - the icon, and the eight a form is made of.
export { default as Avatar } from './Avatar.svelte';
export { default as AvatarGroup } from './AvatarGroup.svelte';
export { default as Badge } from './Badge.svelte';
export { default as Banner } from './Banner.svelte';
export { default as Button } from './Button.svelte';
export { default as Checkbox } from './Checkbox.svelte';
export { default as Dialog } from './Dialog.svelte';
export { default as Icon } from './Icon.svelte';
export { default as IconButton } from './IconButton.svelte';
export { default as Input } from './Input.svelte';
export { default as Menu } from './Menu.svelte';
export { default as Popover } from './Popover.svelte';
export { default as Radio } from './Radio.svelte';
export { default as Select } from './Select.svelte';
export { default as Spinner } from './Spinner.svelte';
export { default as Switch } from './Switch.svelte';
export { default as Textarea } from './Textarea.svelte';
export { default as Toast } from './Toast.svelte';
export { default as Tooltip } from './Tooltip.svelte';

// Wave 2 - the structure a screen is built out of.
export { default as Breadcrumb } from './Breadcrumb.svelte';
export { default as Drawer } from './Drawer.svelte';
export { default as SideNav } from './SideNav.svelte';
export { default as Tabs, type Tab } from './Tabs.svelte';
export { default as Toolbar } from './Toolbar.svelte';

export { STATUS_ICON } from './control.ts';
export type { Busyable, ButtonTone, ControlSize, Disableable, StatusTone } from './control.ts';

export type { Align, Justify, Space } from './space.ts';

export {
  anchorTo,
  logicalRect,
  positionArea,
  resolve,
  supportsAnchor,
  type Alignment,
  type Placement,
  type Side,
} from './positioning.ts';

export { escapeHandler, focusReturn, focusables, rovingIndex, typeAheadIndex } from './focus.ts';

export { openOverlay, type MenuItem, type OverlayOptions } from './overlay.ts';

export {
  collapseTrail,
  flattenTree,
  parentRow,
  treeIntent,
  type CollapsedTrail,
  type Crumb,
  type NavNode,
  type NavRow,
  type TreeIntent,
} from './structure.ts';

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
