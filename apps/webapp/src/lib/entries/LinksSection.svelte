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
  import { Button, Dialog, IconButton, Select, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { EntryLinks, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from './EntryPicker.svelte';
  import ReferenceChip from './ReferenceChip.svelte';
  import {
    drawableTypes, entryLinksPath, groupLinks, links, linkTypes, referencesIn, resolved, sideName, targetLabel,
  } from '../data/links.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Whether the reader may change this entry, which is what drawing and removing a link need. */
    mayChange: boolean;
  }

  const { item, mayChange }: Props = $props();

  // The section is keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntryLinks>(untrack(() => entryLinksPath(item.id)));
  $effect(() => untrack(() => linkTypes.open()));

  const answer = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const groups = $derived(answer ? groupLinks(answer.links, linkTypes.all, t) : []);
  const backlinks = $derived(answer?.backlinks ?? []);
  // The pages that point at the entry, from hubs whose pages the reader may read (M3).
  const pageBacklinks = $derived(answer?.page_backlinks ?? []);
  const failure = $derived(read.state.status === 'failed' ? renderProblem(read.state.error, messages) : undefined);

  // What the notes mention, resolved for the reader, in the order the text holds them.
  const mentioned = $derived(referencesIn(item.notes ?? ''));
  const mentions = resolved(() => mentioned.map((found) => found.reference));

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
    <h2 class="section" id="links-heading-{item.id}">{t('app.links.title')}</h2>
    {#if mayChange && typeOptions.length > 0}
      <Button size="sm" tone="subtle" icon="link" data-draw-link onclick={startDraw}>{t('app.links.add')}</Button>
    {/if}
  </div>

  {#if failure}
    <p class="refusal" role="alert">{failure.message}</p>
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={2} /></div>
  {:else}
    {#if groups.length === 0 && backlinks.length === 0 && pageBacklinks.length === 0 && mentioned.length === 0}
      <p class="quiet">{t('app.links.none')}</p>
    {/if}
    {#each groups as group (group.name)}
      <div class="group">
        <h3 class="relation">{group.name}</h3>
        <ul class="rows">
          {#each group.rows as row (row.id)}
            <li class="row" data-link={row.id}>
              <ReferenceChip target={row.other} />
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
      </div>
    {/each}
    {#if backlinks.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.links.backlinks')}</h3>
        <ul class="rows">
          {#each backlinks as backlink (`${backlink.source_kind}:${backlink.source_id}:${backlink.anchor ?? ''}`)}
            <li class="row" data-backlink={backlink.entry.id}>
              <span class="backlink">
                <ReferenceChip target={backlink.entry} strength={backlink.strength} />
                {#if backlink.source_kind === 'COMMENT'}<span class="quiet">{t('app.links.backlink_comment')}</span>{/if}
              </span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    {#if pageBacklinks.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.links.page_backlinks')}</h3>
        <ul class="rows">
          {#each pageBacklinks as backlink (backlink.page.id)}
            <li class="row" data-page-backlink={backlink.page.id}>
              <ReferenceChip target={backlink.page} strength={backlink.strength} />
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    {#if mentioned.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.mentions.title')}</h3>
        <ul class="rows mentions">
          {#each mentioned as found, index (`${found.reference.kind}/${found.reference.id}#${found.reference.anchor ?? ''}`)}
            <li class="row" data-mention={found.reference.id}>
              <ReferenceChip target={mentions.targets[index]} strength={found.strength} />
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</section>

{#if isDrawOpen}
  <Dialog
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
      <Button
        tone="primary"
        data-confirm-link
        isBusy={isDrawing}
        disabledReason={chosen ? undefined : t('app.links.choose_first')}
        onclick={draw}
      >{t('app.links.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .links { display: flex; flex-direction: column; gap: var(--sp-100); }

  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .section { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }

  .group { display: flex; flex-direction: column; gap: var(--sp-050); }

  .relation { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }

  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .backlink { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .refusal { margin: 0; color: var(--text-danger); }
</style>
