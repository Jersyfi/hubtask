<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's links from both sides, its backlinks, and what its notes mention (UC-PLN-01 checks
  // 1, 4 and 7). Each link is named from this entry's side ("blocks" / "is blocked by"); the far
  // end is what the server let the reader see of it, so one out of sight is "not available to
  // you" and nothing more.
  //
  // **Drawing a link is one dialog**: a type and an entry found across every collection the reader
  // can see. Loops, the limit of one and a link already drawn are the server's to refuse
  // (`links.cycle`, `links.cardinality`, `links.exists`), and the refusal is said in the dialog.
  // Without the right to change the entry neither drawing nor removing is drawn (P-05).

  import { untrack } from 'svelte';
  import { Button, Dialog, IconButton, Select, Skeleton, Stack, StateLozenge } from '@hubtask/design-system/components';
  import type { EntryLinks, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from './EntryPicker.svelte';
  import ReferenceChip from './ReferenceChip.svelte';
  import {
    drawableTypes, entryLinksPath, groupLinks, links, linkTypes, referenceState, referencesIn, resolved, sideName, targetLabel,
  } from '../data/links.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Whether the reader may change this entry, which is what drawing and removing a link need. */
    mayChange: boolean;
    /** How many rows the section holds once read: its heading's count and the jump mark's (ET-2). */
    count?: number;
  }

  let { item, mayChange, count = $bindable(undefined) }: Props = $props();

  // The section is keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntryLinks>(untrack(() => entryLinksPath(item.id)));
  $effect(() => untrack(() => linkTypes.open()));

  /** A group folds from five rows (ET-3); the groups opened here, by name. */
  const LINK_FOLD = 5;
  let openGroups = $state<string[]>([]);
  const cut = <T,>(name: string, rows: readonly T[]): readonly T[] => (openGroups.includes(name) || rows.length <= LINK_FOLD ? rows : rows.slice(0, LINK_FOLD));

  const answer = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const groups = $derived(answer ? groupLinks(answer.links, linkTypes.all, t) : []);
  const backlinks = $derived(answer?.backlinks ?? []);
  // The pages that point at the entry, from hubs whose pages the reader may read (M3).
  const pageBacklinks = $derived(answer?.page_backlinks ?? []);
  const failure = $derived(read.state.status === 'failed' ? renderProblem(read.state.error, messages) : undefined);

  // What the notes mention, resolved for the reader, in the order the text holds them.
  const mentioned = $derived(referencesIn(item.notes ?? ''));
  const mentions = resolved(() => mentioned.map((found) => found.reference));
  /** Every row the section lists, once read: its heading's count and the jump mark's (ET-2). */
  const total = $derived(
    answer ? groups.reduce((sum, group) => sum + group.rows.length, 0) + backlinks.length + pageBacklinks.length + mentioned.length : undefined,
  );
  $effect(() => {
    count = total;
  });

  let refusal = $state<string | undefined>(undefined);

  async function remove(linkId: string) {
    const link = answer?.links.find((each) => each.id === linkId);
    if (!link) return;
    refusal = undefined;
    try {
      await links.remove(link);
      toast.show({ text: t('app.links.removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  // --- Drawing a link ------------------------------------------------------------------------

  let isDrawOpen = $state(false);
  let type = $state('');
  let chosen = $state<WorkItem | undefined>(undefined);
  let isDrawing = $state(false);
  let drawRefusal = $state<string | undefined>(undefined);

  const typeOptions = $derived(
    drawableTypes(linkTypes.all).map((each) => ({ value: each.key, label: sideName(each, each.key, true, t) })),
  );
  const excluded = $derived(new Set([item.id]));

  function startDraw() {
    type = typeOptions[0]?.value ?? '';
    chosen = undefined;
    drawRefusal = undefined;
    isDrawOpen = true;
  }

  async function draw() {
    if (!chosen || type === '') return;
    isDrawing = true;
    drawRefusal = undefined;
    try {
      await links.draw(item.id, chosen.id, type);
      isDrawOpen = false;
      toast.show({ text: t('app.links.drawn'), tone: 'success' });
    } catch (error) {
      drawRefusal = renderProblem(error as never, messages).message;
    } finally {
      isDrawing = false;
    }
  }
</script>

<section class="links" aria-labelledby="links-heading-{item.id}" data-links>
  <div class="head">
    <h2 class="section" id="links-heading-{item.id}">{t('app.links.title')}{#if total}{' '}<span class="count">{total}</span>{/if}</h2>
    <!-- Nothing linked: the section is one line (ET-3). -->
    {#if answer && !failure && groups.length === 0 && backlinks.length === 0 && pageBacklinks.length === 0 && mentioned.length === 0}<span class="quiet none">{t('app.links.none')}</span>{/if}
    {#if mayChange && typeOptions.length > 0}
      <Button size="sm" tone="subtle" icon="link" data-draw-link onclick={startDraw}>{t('app.links.add')}</Button>
    {/if}
  </div>

  {#if failure}
    <p class="refusal" role="alert">{failure.message}</p>
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={2} /></div>
  {:else}
    {#each groups as group (group.name)}
      <div class="group">
        <h3 class="relation">{group.name} <span class="count">{group.rows.length}</span></h3>
        <ul class="rows">
          {#each cut(group.name, group.rows) as row (row.id)}
            <li class="row" data-link={row.id}>
              <ReferenceChip target={row.other} row />
              <!-- An entry's state on its line (§6.9 links), as its workflow names it. -->
              {#if row.other?.available && row.other.kind === 'ENTRY'}
                {@const state = referenceState(row.other, t)}
                <span class="state" data-link-state={state.state}><StateLozenge label={state.label} vocabulary="category" state={state.state} size="sm" /></span>
              {/if}
              {#if mayChange}
                <IconButton
                  icon="x" size="sm" tone="subtle"
                  label={t('app.links.remove', { name: targetLabel(row.other) ?? t('app.links.unavailable') })}
                  data-remove-link={row.id}
                  onclick={() => void remove(row.id)}
                />
              {/if}
            </li>
          {/each}
        </ul>
        {@render more(group.name, group.rows.length)}
      </div>
    {/each}
    {#if backlinks.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.links.backlinks')} <span class="count">{backlinks.length}</span></h3>
        <ul class="rows">
          {#each cut('backlinks', backlinks) as backlink (`${backlink.source_kind}:${backlink.source_id}:${backlink.anchor ?? ''}`)}
            <li class="row" data-backlink={backlink.entry.id}>
              <span class="backlink">
                <ReferenceChip target={backlink.entry} strength={backlink.strength} row />
                {#if backlink.source_kind === 'COMMENT'}<span class="quiet">{t('app.links.backlink_comment')}</span>{/if}
              </span>
            </li>
          {/each}
        </ul>
        {@render more('backlinks', backlinks.length)}
      </div>
    {/if}
    {#if pageBacklinks.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.links.page_backlinks')} <span class="count">{pageBacklinks.length}</span></h3>
        <ul class="rows">
          {#each cut('pages', pageBacklinks) as backlink (backlink.page.id)}
            <li class="row" data-page-backlink={backlink.page.id}>
              <ReferenceChip target={backlink.page} strength={backlink.strength} row />
            </li>
          {/each}
        </ul>
        {@render more('pages', pageBacklinks.length)}
      </div>
    {/if}
    {#if mentioned.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.mentions.title')} <span class="count">{mentioned.length}</span></h3>
        <ul class="rows mentions">
          {#each cut('mentions', mentioned) as found, index (`${found.reference.kind}/${found.reference.id}#${found.reference.anchor ?? ''}`)}
            <li class="row" data-mention={found.reference.id}>
              <ReferenceChip target={mentions.targets[index]} strength={found.strength} row />
            </li>
          {/each}
        </ul>
        {@render more('mentions', mentioned.length)}
      </div>
    {/if}
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</section>

{#snippet more(name: string, total: number)}
  {#if total > LINK_FOLD}
    {@const isOpen = openGroups.includes(name)}
    <button type="button" class="more" aria-expanded={isOpen} onclick={() => (openGroups = isOpen ? openGroups.filter((each) => each !== name) : [...openGroups, name])}>
      {isOpen ? t('app.entry.show_less') : t('app.entry.show_all', { count: String(total) })}
    </button>
  {/if}
{/snippet}

{#if isDrawOpen}
  <Dialog size="l" primary={{ label: t('app.links.confirm'), isBusy: isDrawing, disabledReason: chosen ? undefined : t('app.links.choose_first'), onclick: () => void draw() }}
    bind:isOpen={isDrawOpen}
    title={t('app.links.picker_title')}
    dismissLabel={t('app.links.cancel')}
    onClose={() => { chosen = undefined; }}
  >
    <Stack gap="200">
      <Select label={t('app.links.type')} options={typeOptions} bind:value={type} />
      <EntryPicker {excluded} {chosen} onchoose={(found) => (chosen = found)} />
      {#if drawRefusal}<p class="refusal" role="alert">{drawRefusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isDrawOpen = false)}>{t('app.links.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .none { flex: 1; min-inline-size: 0; font-size: var(--fs-075); }

  .links { display: flex; flex-direction: column; gap: var(--sp-100); }

  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .section { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }

  .group { display: flex; flex-direction: column; gap: var(--sp-050); }

  .relation { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  /* One column no wider than the section: an auto track would grow to a title that does not wrap. */
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: 0; }

  /* One tight line per link: the reference, its state at the end, the remove. */
  .row { display: flex; align-items: center; gap: var(--sp-100); min-block-size: var(--sp-400); }
  .row > :global(:first-child) { flex: 1; min-inline-size: 0; }

  .backlink { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .count { color: var(--text-subtle); font-weight: var(--fw-regular); }

  .more { align-self: flex-start; padding: 0; border: none; background: none; color: var(--text-brand); font: inherit; font-size: var(--fs-075); cursor: pointer; }
  .more:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .refusal { margin: 0; color: var(--text-danger); }
</style>
