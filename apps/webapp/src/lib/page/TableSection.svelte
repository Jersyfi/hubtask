<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A settings section that is a table (page-system §4.2 rule 2, §9.3: *Targets (table)*): its
  // heading, count and *Add* on the canvas, the `DataTable` the one region under it. Not a
  // `SettingsSection`, whose region would put the table's region inside a second one (PS-16).
  import type { Snippet } from 'svelte';

  interface Props {
    title: string;
    /** A count or a state after the name. */
    note?: string;
    description?: string;
    /** The section's own acts at the heading's end: its *Add*. */
    acts?: Snippet;
    children: Snippet;
  }

  const { title, note, description, acts, children }: Props = $props();
  const id = $props.id();
</script>

<section class="table-section" aria-labelledby={`table-section-${id}`}>
  <div class="head">
    <div class="line">
      <h2 class="name" id={`table-section-${id}`}>{title}</h2>
      {#if note}<span class="note">{note}</span>{/if}
      {#if acts}<span class="acts">{@render acts()}</span>{/if}
    </div>
    {#if description}<p class="description">{description}</p>{/if}
  </div>
  {@render children()}
</section>

<style>
  .table-section { display: flex; flex-direction: column; gap: var(--sp-250); min-width: 0; }
  .head { display: flex; flex-direction: column; gap: var(--sp-050); min-width: 0; }
  .line { display: flex; align-items: baseline; flex-wrap: wrap; gap: var(--sp-100); }
  .name {
    margin: 0;
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
    overflow-wrap: anywhere;
  }
  .note { color: var(--text-subtle); font-size: var(--fs-100); }
  .acts { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }
  .description { margin: 0; color: var(--text-secondary); font-size: var(--fs-100); max-inline-size: 72ch; }
</style>
