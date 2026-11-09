<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One administered object (page-system §7.2): its `RecordHead`, its own lists as regions - each
  // with its heading, a count and its *Add* on the canvas - its read-only facts as a region of
  // label and value rows, and the `DangerZone` last. At most one strip, where the object has more
  // than three lists.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import DangerZone, { type DangerAct } from './DangerZone.svelte';
  import RecordHead from './RecordHead.svelte';
  import PageFrame from './_PageFrame.svelte';
  import { pageBar } from './page.ts';
  import RegionHead from './_RegionHead.svelte';
  import type { MenuItem } from './overlay.ts';
  import type { PageAction } from './PageHeader.svelte';

  export interface RecordList {
    readonly id: string;
    readonly title: string;
    /** *2*. */
    readonly count?: string;
    readonly add?: PageAction;
  }

  interface Props {
    /** `RecordHead`'s props. */
    record: {
      kind: string;
      title: string;
      back?: { label: string; href: string; onnavigate?: () => void };
      status?: { label: string; tone?: 'success' | 'warning' | 'danger' | 'info' | 'neutral' };
      facts?: string;
      edit?: PageAction;
      menu?: { label: string; items: readonly MenuItem[]; onselect: (id: string) => void };
    };
    strip?: Snippet;
    /** The object's own lists, in order. */
    lists?: readonly RecordList[];
    /** One list's region: a `DataTable`. */
    list?: Snippet<[RecordList]>;
    /** Read-only facts. */
    facts?: { title: string; rows: readonly { label: string; value: string }[] };
    danger?: { title: string; acts: readonly DangerAct[]; onact: (id: string) => void };
    refused?: Snippet;
  }

  const { record, strip, lists = [], list, facts, danger, refused }: Props = $props();

  // On a phone the way back to the list is the bar's (the navigation concept §9): handed over,
  // and not drawn a second time in the head.
  const bar = pageBar();
  $effect(() => {
    const back = record.back;
    if (bar && back?.onnavigate) return bar.carry({ label: back.label, onback: back.onnavigate });
  });
  const head = $derived(bar?.hasBack && record.back ? { ...record, back: undefined } : record);
</script>

<PageFrame kind="record" {refused}>
  {#snippet top()}
    <RecordHead {...head} />
  {/snippet}
  {#snippet children()}
    {#if strip}{@render strip()}{/if}
    {#each lists as item (item.id)}
      <section class="part">
        <RegionHead title={item.title} note={item.count}>
          {#snippet acts()}
            {#if item.add}
              <Button icon={item.add.icon ?? 'plus'} onclick={item.add.onclick} disabledReason={item.add.disabledReason} data-opener={item.add.opener}>{item.add.label}</Button>
            {/if}
          {/snippet}
        </RegionHead>
        {@render list?.(item)}
      </section>
    {/each}
    {#if facts}
      <section class="part">
        <RegionHead title={facts.title} />
        <dl class="facts">
          {#each facts.rows as row (row.label)}
            <div class="fact"><dt>{row.label}</dt><dd>{row.value}</dd></div>
          {/each}
        </dl>
      </section>
    {/if}
    {#if danger}<DangerZone title={danger.title} acts={danger.acts} onact={danger.onact} />{/if}
  {/snippet}
</PageFrame>

<style>
  .part { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }

  .facts {
    margin: 0;
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    font-size: var(--fs-100);
  }

  .fact { display: flex; gap: var(--sp-200); padding: var(--sp-150) var(--sp-250); }
  .fact + .fact { border-block-start: var(--bw-hairline) solid var(--border-subtle); }
  .fact dt { flex: none; inline-size: var(--layout-barsearch-side); color: var(--text-subtle); }
  .fact dd { margin: 0; min-width: 0; overflow-wrap: anywhere; }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .fact { flex-direction: column; gap: var(--sp-025); padding: var(--sp-150) var(--sp-200); }
    .fact dt { inline-size: auto; }
  }
</style>
