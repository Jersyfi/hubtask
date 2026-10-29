<!-- SPDX-License-Identifier: Apache-2.0
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
  // decision 6). Not the bar: a display name is the address until somebody introduces themselves
  // — the server's own convention — so a fresh workspace would put its owner's e-mail in the frame
  // on every screen. An address is not navigation. The name joins the
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
    /** The picture the person shows, drawn in place of the initials where there is one. */
    picture?: string;
    /** A sheet from the bottom on `compact`; a menu behind the avatar above it. */
    isSheet: boolean;
    /** The sheet's state, owned by the frame because the bottom bar is what opens it. */
    isSheetOpen?: boolean;
    /**
     * Where a row's area lives instead, for a build that leaves it to the web app: the row's second
     * line ("In the web app at …"), and the outward mark after it (UC-SYN-07 check 7).
     */
    elsewhere?: (id: string) => string | undefined;
    onchoose: (id: string) => void;
  }

  let { destinations, name, email, picture, isSheet, isSheetOpen = $bindable(false), elsewhere, onchoose }: Props = $props();

  /**
   * The list in three bands: the places that are the reader's own, the ways through, and signing
   * out - which is the last row and stands alone (ADR-0065 decision 5).
   *
   * Both rules are about the *shape* of the list rather than about which row is which, so this
   * component still knows no ids: a separator opens the row the list marks as starting the ways
   * through (`startsBand`), and another opens the last row, because the last thing a reader does
   * in a session has nothing under it.
   */
  const items = $derived(
    destinations.map((destination, index) => ({
      id: destination.id,
      label: t(destination.code),
      icon: destination.icon,
      sub: elsewhere?.(destination.id),
      hasSeparatorBefore:
        index > 0 && (destination.startsBand === true || index === destinations.length - 1),
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
       nothing else does: two rules under the name would be one too many. -->
  <div class="who" data-sheet={isSheet ? '' : undefined}>
    <Avatar {name} src={picture} size="md" />
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
              {#if item.sub}
                <span class="label">{item.label}<span class="sub">{item.sub}</span></span>
              {:else}
                {item.label}
              {/if}
              {#snippet trailing()}{#if item.sub}<Icon name="external-link" size="sm" />{/if}{/snippet}
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
      <!-- The avatar alone at every width, named by the person: the name and the address head
           the menu it opens (design-system.md §11.2). -->
      <button type="button" class="account" aria-label={name} {...props}>
        <span aria-hidden="true"><Avatar {name} src={picture} size="sm" /></span>
      </button>
    {/snippet}
  </Menu>
{/if}

<style>
  /* The bar's square control, the size and hover shape of every other one beside it (§13, G2). */
  .account {
    display: inline-grid;
    place-items: center;
    inline-size: var(--density-control-sm-min);
    block-size: var(--density-control-sm-min);
    padding: 0;
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }

  .account:hover { background: var(--bg-surface-hover); }

  .account:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
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
     on white and disappears on the dark theme's surface - the person's own name, greyed out.
     The address under it stays subtle, because it is the second line. */
  .who .name { color: var(--text-primary); font-weight: var(--fw-semibold); overflow-wrap: anywhere; }

  .email { color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }

  .rows { margin: 0; padding: 0; list-style: none; }

  .label { display: flex; flex-direction: column; min-width: 0; }

  .sub { color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }

  .apart {
    margin-block-start: var(--sp-100);
    padding-block-start: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }
</style>
