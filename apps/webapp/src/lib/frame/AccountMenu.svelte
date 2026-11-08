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

  import { Avatar, Menu, NavItem, Sheet } from '@hubtask/design-system/components';

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
    /**
     * The workspaces this device knows, for the phone's sheet: the switch stands in it (the
     * navigation concept §9, UC-ID-25), the current one checked. Empty where there is none to list.
     */
    workspaces?: readonly { readonly host: string; readonly name: string; readonly isHere: boolean }[];
    onworkspace?: (host: string) => void;
    onchoose: (id: string) => void;
  }

  let { destinations, name, email, picture, isSheet, isSheetOpen = $bindable(false), elsewhere, workspaces = [], onworkspace, onchoose }: Props = $props();

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

  /** In the sheet the workspaces are listed, so *Switch workspace* is not a row of its own there. */
  const sheetItems = $derived(workspaces.length > 0 ? items.filter((item) => item.id !== 'switch-workspace') : items);

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
  <!-- The avatar in the bar on a phone too (user-experience.md §2.6): the bottom bar's fifth slot
       is *More*, so the account is opened where it is on every other width. -->
  <button type="button" class="account" aria-label={name} aria-haspopup="dialog" onclick={() => (isSheetOpen = true)}>
    <span aria-hidden="true"><Avatar {name} src={picture} size="sm" /></span>
  </button>
  <Sheet title={t('app.nav.you')} closeLabel={t('app.sheet_close')} bind:isOpen={isSheetOpen}>
    {@render who()}
    {#if workspaces.length > 0}
      <h3 class="group-label">{t('app.nav.workspace_group')}</h3>
      <ul class="rows" data-workspaces>
        {#each workspaces as each (each.host)}
          <NavItem label={each.name} icon={each.isHere ? 'check' : 'hub'} isCurrent={each.isHere} size="touch"
            onnavigate={() => { isSheetOpen = false; if (!each.isHere) onworkspace?.(each.host); }} />
        {/each}
        {#if items.some((item) => item.id === 'add-workspace')}
          <NavItem label={t('app.app_workspaces.add')} icon="plus" size="touch" onnavigate={() => choose('add-workspace')} />
        {/if}
      </ul>
    {/if}
    <ul class="rows">
      {#each sheetItems.filter((item) => workspaces.length === 0 || item.id !== 'add-workspace') as item (item.id)}
        <NavItem label={item.label} icon={item.icon} figure={item.sub} size="touch" onnavigate={() => choose(item.id)} />
      {/each}
    </ul>
  </Sheet>
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

  /* In the phone's sheet: the rows as one card each band. */
  .group-label {
    margin: var(--sp-150) var(--sp-050) var(--sp-050);
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-065);
    font-weight: var(--fw-medium);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .who[data-sheet] + .rows, .group-label + .rows, .rows + .rows { margin-block-start: var(--sp-100); }
  .rows + .rows { padding-block-start: var(--sp-100); border-block-start: var(--bw-hairline) solid var(--border-subtle); }

</style>
