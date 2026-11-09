<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Values that configure a place (page-system §7.3, PS-9, PS-10): the head, whose ⋯ carries the
  // destructive acts in a group of their own; the `SectionNav`; one `SettingsSection` at its own
  // address with its `SaveBar`. Two panes from `expanded` up; below, the list is the page and a
  // section opens alone with *‹ back*, the head giving way to it. Where the application's bar
  // carries the way back (a phone, the navigation concept §9), the page draws none: the list's
  // *‹ the level above* is the bar's, and an open section hands its *‹ Settings* to the bar.
  //
  // A product's settings are the last tab of its home (the navigation concept §4): `tabs` draws the
  // home's `ViewBar` under the head, the section list beside the section below it - never a second
  // strip (PS-2). An open section below `expanded` stands alone, so the bar gives way with the head.
  //
  // The concept calls this kind `SettingsPage`; the name `SettingsLayout` is kept from when the web
  // app had a settings shell of that name. Its `data-page-kind` is `settings`.
  import type { Snippet } from 'svelte';

  import type { SectionLink } from './SectionNav.svelte';
  import PageFrame from './_PageFrame.svelte';
  import TwoPane from './_TwoPane.svelte';
  import { pageBar, type PageBack, type PageHead } from './page.ts';

  interface Props {
    head: PageHead;
    /** The sections: `SectionNav`'s props. */
    nav: { label: string; sections: readonly SectionLink[]; current?: string; onselect?: (id: string) => void };
    /** At the settings' own address the list; at a section's address the section. */
    view?: 'list' | 'detail';
    back: PageBack;
    /** The home's `ViewBar` where the settings are one of its tabs. */
    tabs?: Snippet;
    refused?: Snippet;
    /** The open `SettingsSection`. */
    children: Snippet;
  }

  const { head, nav, view = 'detail', back, tabs, refused, children }: Props = $props();

  const bar = pageBar();
  $effect(() => {
    if (bar && view === 'detail') return bar.carry(back);
  });
  const isBackInBar = $derived(bar?.hasBack === true);
</script>

<div class="settings-layout" data-view={view}>
  <PageFrame kind="settings" {head} {refused} isTitleInBar={isBackInBar}>
    {#if tabs}<div class="tabs">{@render tabs()}</div>{/if}
    <TwoPane {nav} {view} {back} isBackHidden={isBackInBar}>{@render children()}</TwoPane>
  </PageFrame>
</div>

<style>
  /* Below `expanded` an open section stands alone: the page's head gives way to its *‹ back*. */
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width < 905px) {
    .settings-layout[data-view='detail'] :global([data-page-kind] > header),
    .settings-layout[data-view='detail'] .tabs { display: none; }
  }
</style>
