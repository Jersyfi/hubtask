<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The account group of the one navigation list (ADR-0061 decision 1), drawn two ways.
  //
  // From `medium` up it is a `Menu` behind the avatar - where "signed in as …" was a sentence, the
  // person is now the control that opens what is theirs. On `compact` there is no avatar in the
  // bar; "You" in the bottom bar opens the same group as a sheet from the bottom. Two drawings,
  // one list: the rows are handed in and this component invents none.
  //
  // **The name and the address are the head of what it opens, in both drawings** (ADR-0063
  // decision 6). The bar used to carry the display name, and a display name is the address until
  // somebody introduces themselves — the server's own convention, so a fresh workspace put its
  // owner's e-mail in the frame on every screen. An address is not navigation. The name joins the
  // trigger again only from `large`, where there is room for it beside everything else.
  //
  // What choosing a row does is the frame's to decide - a route, the tour, signing out - so the
  // choice goes back as the destination's id.

  import { Avatar, Drawer, Icon, ListRow, Menu, Stack } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import type { Destination } from '../navigation.ts';

  interface Props {
    destinations: readonly Destination[];
    name: string;
    email?: string | null;
    /** A sheet from the bottom on `compact`; a menu behind the avatar above it. */
    isSheet: boolean;
    /** Whether the trigger carries the name beside the avatar. Only where there is room for it. */
    hasName?: boolean;
    /** The sheet's state, owned by the frame because the bottom bar is what opens it. */
    isSheetOpen?: boolean;
    onchoose: (id: string) => void;
  }

  let { destinations, name, email, isSheet, hasName = false, isSheetOpen = $bindable(false), onchoose }: Props = $props();

  /**
   * The list in three bands: the places that are the reader's own, the ways through, and signing
   * out - which is the last row and stands alone (ADR-0065 decision 5).
   *
   * Both rules are about the *shape* of the list rather than about which row is which, so this
   * component still knows no ids: a separator opens the first row that performs something rather
   * than going somewhere, and another opens the last row, because the last thing a reader does in
   * a session has nothing under it.
   */
  const firstAction = $derived(destinations.find((each) => each.target.kind === 'action')?.id);
  const items = $derived(
    destinations.map((destination, index) => ({
      id: destination.id,
      label: t(destination.code),
      icon: destination.icon,
      hasSeparatorBefore:
        index > 0 && (destination.id === firstAction || index === destinations.length - 1),
    })),
  );

  function choose(id: string) {
    isSheetOpen = false;
    onchoose(id);
  }
</script>

<!-- Who this is: the same head in both drawings, because the question "whose menu is this" has
     one answer. The address is here and nowhere else. -->
{#snippet who()}
  <!-- `Menu` draws the hairline under its own head, so this draws one only in the sheet, where
       nothing else does. Two rules under the name is what the walk found (issue 1019). -->
  <div class="who" data-sheet={isSheet ? '' : undefined}>
    <Avatar {name} size="md" />
    <div class="names">
      <span class="name">{name}</span>
      {#if email && email !== name}<span class="email">{email}</span>{/if}
    </div>
  </div>
{/snippet}

{#if isSheet}
  <Drawer bind:isOpen={isSheetOpen} edge="block-end" title={t('app.nav.you')} dismissLabel={t('app.dismiss')}>
    <Stack gap="200">
      {@render who()}
      <ul class="rows">
        {#each items as item (item.id)}
          <li class:apart={item.hasSeparatorBefore}>
            <ListRow onactivate={() => choose(item.id)}>
              {#snippet leading()}<Icon name={item.icon} size="sm" />{/snippet}
              {item.label}
            </ListRow>
          </li>
        {/each}
      </ul>
    </Stack>
  </Drawer>
{:else}
  <Menu label={t('app.nav.you')} {items} placement={{ side: 'block-end', align: 'end' }} onselect={choose}>
    {#snippet head()}
      {@render who()}
    {/snippet}
    {#snippet trigger(props)}
      <!-- The button is named by the person, once: the avatar is decoration here, because a
           control called "Jérôme Winkel Jérôme Winkel" is a control named twice. The name is drawn
           beside it only where there is room; the address never is. -->
      <button type="button" class="account" aria-label={hasName ? undefined : name} {...props}>
        <span aria-hidden="true"><Avatar {name} size="sm" /></span>
        {#if hasName}<span class="name">{name}</span>{/if}
      </button>
    {/snippet}
  </Menu>
{/if}

<style>
  .account {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    max-inline-size: 100%;
    min-block-size: var(--density-control-sm-min);
    padding: var(--sp-025);
    border: 0;
    border-radius: var(--r-full);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }

  /* With the name beside it the control is a pill and the air after the name is the pill's; with
     the avatar alone it is a **circle** around the avatar - the target is the control's square
     minimum and the shape is round, because what is inside it is round (issue 1019, 1022). The
     two glyph controls beside it are `IconButton`'s rounded square, which is what every other
     icon control in the product is. */
  .account:has(.name) { padding-inline-end: var(--sp-100); }

  .account:not(:has(.name)) {
    inline-size: var(--density-control-sm-min);
    justify-content: center;
    padding: 0;
  }

  .account:hover { background: var(--bg-surface-hover); }

  .account:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
  }

  /* One line; a long name loses its end, never the avatar beside it (rule 4). */
  .account .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .who {
    display: flex;
    align-items: center;
    gap: var(--sp-150);
  }

  .who[data-sheet] {
    padding-block-end: var(--sp-150);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .names { display: flex; flex-direction: column; min-width: 0; }

  /* The name is primary text: the menu's head inherits the surface's quieter colour, which reads
     on white and disappears on the dark theme's surface - the person's own name, greyed out
     (issue 1022). The address under it stays subtle, because it is the second line. */
  .who .name { color: var(--text-primary); font-weight: var(--fw-semibold); overflow-wrap: anywhere; }

  .email { color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }

  .rows { margin: 0; padding: 0; list-style: none; }

  .apart {
    margin-block-start: var(--sp-100);
    padding-block-start: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
</style>
