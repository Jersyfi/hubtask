<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The head of one administered object (page-system §6, §7.2): the way back to its list, the kind
  // as the eyebrow, the name, a status, one line of facts, *Edit* - a Dialog M of the object's own
  // fields - and ⋯ for the rest, the destructive acts last. No primary: an object's lists carry
  // their own *Add*, and the page has nothing to create.
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import IconButton from './IconButton.svelte';
  import Menu from './Menu.svelte';
  import type { MenuItem } from './overlay.ts';
  import type { PageAction } from './PageHeader.svelte';

  interface Props {
    /** *Group*. */
    kind: string;
    title: string;
    /** The way back: *Groups*. */
    back?: { label: string; href: string; onnavigate?: () => void };
    status?: { label: string; tone?: 'success' | 'warning' | 'danger' | 'info' | 'neutral' };
    /** *2 members · 1 role · changed 2 days ago*. */
    facts?: string;
    edit?: PageAction;
    menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void };
  }

  const { kind, title, back, status, facts, edit, menu }: Props = $props();
</script>

<header class="record-head">
  {#if back}
    <a
      class="back"
      href={back.href}
      onclick={(event) => {
        if (back.onnavigate) {
          event.preventDefault();
          back.onnavigate();
        }
      }}
    >
      <Icon name="chevron-left" size="sm" /><span>{back.label}</span>
    </a>
  {/if}
  <div class="row">
    <div class="titles">
      <p class="kind">{kind}</p>
      <div class="title-line">
        <h1 class="title">{title}</h1>
        {#if status}<Badge tone={status.tone ?? 'neutral'}>{status.label}</Badge>{/if}
      </div>
      {#if facts}<p class="facts">{facts}</p>{/if}
    </div>
    <div class="acts">
      {#if edit}
        <Button tone="secondary" icon={edit.icon} onclick={edit.onclick} disabledReason={edit.disabledReason} data-opener={edit.opener}>{edit.label}</Button>
      {/if}
      {#if menu && menu.items.length > 0}
        <Menu label={menu.label} items={menu.items} placement={{ side: 'block-end', align: 'end' }} onselect={menu.onselect}>
          {#snippet trigger(props)}
            <IconButton icon="ellipsis" label={menu.label} size="sm" {...props} />
          {/snippet}
        </Menu>
      {/if}
    </div>
  </div>
</header>

<style>
  .record-head { display: flex; flex-direction: column; gap: var(--sp-150); min-width: 0; }

  .back {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    align-self: flex-start;
    color: var(--text-secondary);
    font-size: var(--fs-100);
    text-decoration: none;
  }

  .back:hover { color: var(--text-brand); }

  .back:focus-visible {
    outline: var(--bw-ring) solid var(--focus-ring);
    outline-offset: var(--sp-025);
    border-radius: var(--r-xs);
  }

  .row { display: flex; flex-wrap: wrap; align-items: flex-start; gap: var(--sp-200); }
  .titles { flex: 1 1 var(--layout-barsearch-side); min-width: 0; }

  .kind {
    margin: 0 0 var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-transform: uppercase;
  }

  .title-line { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); }

  .title {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-600);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }

  .facts { margin: var(--sp-050) 0 0; color: var(--text-secondary); font-size: var(--fs-100); }
  .acts { display: flex; align-items: center; gap: var(--sp-100); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .title { font-size: var(--fs-500); }
  }
</style>
