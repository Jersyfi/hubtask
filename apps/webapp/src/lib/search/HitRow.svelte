<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One hit row for every kind (§6.11, scene search 5): the product tile, the key, the title with
  // the found words marked, the passage that matched, and under it the place, how long ago and
  // the kind where the title alone does not say it. A verified page carries its lozenge; a state
  // is a word beside its colour, never colour alone.
  import { ProductTile, StateLozenge, VisuallyHidden } from '@hubtask/design-system/components';
  import type { IconName } from '@hubtask/design-system/icons';

  import { accounts } from '../data/accounts.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { formatRelative } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { marked, markWords, pathOf, saysItsKind, tileOf, type Hit } from './everything.ts';

  interface Props {
    hit: Hit;
    /** The words typed, which the title marks. */
    words: string;
    /** A person's hit has no page: it searches their entries. */
    onperson?: (hit: Hit) => void;
  }

  const { hit, words, onperson }: Props = $props();

  const tile = $derived(tileOf(hit));
  const href = $derived(pathOf(hit));
  const collection = $derived(hit.collection_id ? containers.find(hit.collection_id) : undefined);
  const hub = $derived(hit.hub_id ? containers.find(hit.hub_id) : collection?.parent_id ? containers.find(collection.parent_id) : undefined);
  const lozenge = $derived.by(() => {
    if (hit.kind === 'PAGE' && (hit.state === 'VERIFIED' || hit.state === 'REVIEW_DUE')) {
      return { tone: hit.state === 'VERIFIED' ? ('success' as const) : ('warning' as const), icon: hit.state === 'VERIFIED' ? ('circle-check' as const) : undefined };
    }
    return undefined;
  });
  const meta = $derived(
    [
      hub?.name && collection?.name ? `${hub.name} › ${collection.name}` : (hub?.name ?? collection?.name),
      hit.updated_at ? formatRelative(hit.updated_at, messages.locale) : undefined,
      hit.author_id && accounts.nameOf(hit.author_id) ? t('app.search.by_name', { name: accounts.nameOf(hit.author_id) ?? '' }) : undefined,
      saysItsKind(hit.kind) || hit.kind === 'PERSON' ? t('app.search.kind_one', { kind: hit.kind }) : undefined,
    ].filter(Boolean),
  );
  const initials = (name: string) => name.split(/\s+/u).filter(Boolean).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? '').join('');
</script>

{#snippet body()}
  {#if hit.kind === 'PERSON'}
    <span class="initials" aria-hidden="true">{initials(hit.title)}</span>
  {:else}
    <ProductTile icon={tile.icon as IconName} product={tile.product} color={tile.color} size="sm" />
  {/if}
  <span class="text">
    <span class="line">
      {#if hit.key}<span class="key">{hit.key}</span>{/if}
      <span class="title">{#each markWords(hit.title, words) as piece, index (index)}{#if piece.isFound}<mark>{piece.text}</mark>{:else}{piece.text}{/if}{/each}</span>
      <VisuallyHidden>{t('app.search.kind_one', { kind: hit.kind })}</VisuallyHidden>
    </span>
    {#if hit.snippet}
      <span class="snippet">{#each marked(hit.snippet) as piece, index (index)}{#if piece.isFound}<mark>{piece.text}</mark>{:else}{piece.text}{/if}{/each}</span>
    {/if}
    {#if meta.length > 0 || lozenge}
      <span class="meta">
        {meta.join(' · ')}
        {#if lozenge}<StateLozenge label={t('app.search.state', { state: hit.state ?? '' })} tone={lozenge.tone} icon={lozenge.icon} />{/if}
      </span>
    {/if}
  </span>
  {#if hit.state && !lozenge && (hit.kind === 'ENTRY' || hit.kind === 'TICKET' || hit.kind === 'GOAL' || hit.kind === 'RELEASE' || hit.kind === 'SPRINT')}
    <span class="state"><StateLozenge label={t('app.search.state', { state: hit.state })} tone={hit.state === 'DONE' || hit.state === 'RELEASED' ? 'success' : 'info'} /></span>
  {/if}
{/snippet}

{#if href}
  <a class="hit" {href} data-kind={hit.kind} data-hit-row={hit.id}>{@render body()}</a>
{:else}
  <button type="button" class="hit" data-kind={hit.kind} data-hit-row={hit.id} onclick={() => onperson?.(hit)}>{@render body()}</button>
{/if}

<style>
  .hit {
    display: flex;
    align-items: flex-start;
    gap: var(--sp-150);
    box-sizing: border-box;
    inline-size: 100%;
    min-inline-size: 0;
    padding: var(--sp-150) var(--sp-200);
    border: 0;
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
    background: none;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    text-decoration: none;
    cursor: pointer;
  }

  .hit:hover { background: var(--bg-surface-hover); }

  .hit:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }

  .text { display: flex; flex: 1; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }

  .line { display: flex; align-items: baseline; gap: var(--sp-100); min-inline-size: 0; }

  .key { flex: none; color: var(--text-secondary); font-family: var(--font-mono); font-size: var(--fs-075); }

  .title { overflow: hidden; font-weight: var(--fw-semibold); text-overflow: ellipsis; white-space: nowrap; }

  .snippet { overflow: hidden; color: var(--text-secondary); font-size: var(--fs-075); text-overflow: ellipsis; white-space: nowrap; }

  .meta { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); color: var(--text-subtle); font-family: var(--font-mono); font-size: var(--fs-075); }

  .state { flex: none; }

  mark { border-radius: var(--r-sm); background: var(--status-warning-surface); color: inherit; }

  .initials {
    display: inline-flex;
    flex: none;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-300);
    block-size: var(--sp-300);
    border-radius: var(--r-full);
    background: var(--bg-recess);
    color: var(--text-secondary);
    font-size: var(--fs-050);
    font-weight: var(--fw-semibold);
  }
</style>
