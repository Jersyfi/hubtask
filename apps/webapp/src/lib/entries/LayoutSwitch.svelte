<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which shape the entries are shown in: the head's second row (ADR-0061 decision 4).
  //
  // The manifest reports four layouts and the switch offers three. `LIST_COLLAPSED` and
  // `LIST_EXPANDED` are one place - the list - with "show what is inside" as a toggle within it,
  // because whether the children are shown is a property of the list rather than a fourth
  // rendering of the entries. Both stay the stored values: a saved view that says
  // `LIST_EXPANDED` still means what it meant, and the toggle is what chooses between them.
  //
  // Everything offered comes from `view_layouts`. A layout the installation reports and this
  // client cannot draw is offered with the reason, as the switcher has always done; a list
  // layout the installation does not report is not offered, and the toggle appears only when
  // both list layouts are.

  import { Checkbox, ViewSwitcher, type IconName, type View } from '@hubtask/design-system/components';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The stored layout: one of the four the contract names. */
    layout: string;
    /** The layouts this client can actually draw. */
    drawable: readonly string[];
    onlayout: (id: string) => void;
  }

  const { layout, drawable, onlayout }: Props = $props();

  /** The two list layouts, as the contract names them, and the one place they are drawn as. */
  const COLLAPSED = 'LIST_COLLAPSED';
  const EXPANDED = 'LIST_EXPANDED';
  const LIST = 'LIST';

  const isList = (id: string) => id === COLLAPSED || id === EXPANDED;

  const reported = $derived<readonly string[]>(manifest.value?.view_layouts ?? []);
  const hasBothLists = $derived(reported.includes(COLLAPSED) && reported.includes(EXPANDED));

  const ICONS: Record<string, IconName> = { [LIST]: 'menu', KANBAN: 'bucket', TIMELINE: 'calendar' };

  /** The three: the list once, in the position of the first list layout reported, then the rest. */
  const views = $derived<View[]>(
    reported
      .flatMap((id, index) => {
        if (isList(id)) {
          if (reported.findIndex(isList) !== index) return [];
          // The list is drawable if either of its layouts is.
          const drawn = reported.filter(isList).some((each) => drawable.includes(each));
          return [{ id: LIST, label: t(`app.view.${COLLAPSED}`), icon: ICONS[LIST], unavailableReason: drawn ? undefined : t('app.view.not_built', { layout: t(`app.view.${COLLAPSED}`) }) }];
        }
        return [{ id, label: t(`app.view.${id}`), icon: ICONS[id], unavailableReason: drawable.includes(id) ? undefined : t('app.view.not_built', { layout: t(`app.view.${id}`) }) }];
      }),
  );

  const selected = $derived(isList(layout) ? LIST : layout);
  /** Which list layout the toggle last chose, so choosing the list again returns to it. */
  let inside = $state(false);
  $effect(() => {
    if (isList(layout)) inside = layout === EXPANDED;
  });

  function choose(id: string) {
    if (id !== LIST) return onlayout(id);
    onlayout(inside && reported.includes(EXPANDED) ? EXPANDED : reported.includes(COLLAPSED) ? COLLAPSED : EXPANDED);
  }
</script>

<!-- `data-tour`: where the tour points for "the same entries, two ways" (F6-14). -->
<div class="layout-switch" data-tour="layouts">
  <ViewSwitcher label={t('app.view.label')} {views} {selected} onselect={choose} />
  {#if isList(layout) && hasBothLists}
    <Checkbox
      label={t('app.view.show_inside')}
      checked={layout === EXPANDED}
      onclick={() => onlayout(layout === EXPANDED ? COLLAPSED : EXPANDED)}
      onkeydown={(event: KeyboardEvent) => {
        if (event.key !== ' ' && event.key !== 'Enter') return;
        event.preventDefault();
        onlayout(layout === EXPANDED ? COLLAPSED : EXPANDED);
      }}
    />
  {/if}
</div>

<style>
  .layout-switch { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }
</style>
