// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The screens loaded on demand, apart from the parts they load (parts.ts): a screen imports its
// parts, and a table that also loaded the screen would close a cycle through a dynamic import.
// On the Linux shell (WebKitGTK) a screen then ran before its module had finished evaluating.

/** The frame and the work's own screens: the doors before sign-in draw none of them. */
export const loadFrame = () => import('./AppFrame.svelte');
export const workViews = {
  TodayView: () => import('../../views/TodayView.svelte'),
  ItemView: () => import('../../views/ItemView.svelte'),
  ContainerView: () => import('../../views/ContainerView.svelte'),
};

/**
 * The rules' screens: the profile's own rules and the administration's, so one chunk the area
 * module asks for too rather than binding it in, which the Android build would leave out.
 */
export const ruleViews = {
  RulesView: () => import('../../views/RulesView.svelte'),
  RuleEditorView: () => import('../../views/RuleEditorView.svelte'),
};
