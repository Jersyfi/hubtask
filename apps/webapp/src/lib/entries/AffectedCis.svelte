<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The configuration items an entry is about (UC-AST-09 check 6): each affected CI with its class
  // and stage, the active services and offerings their failure reaches, the other open entries on
  // the same CIs, and the entries planned on them in an overlapping window. All four lists are what
  // the server let the reader see; the screen adds nothing.
  //
  // **Naming a CI is one dialog**: a hub, then a CI of its inventory by name (`CiPicker`). The limit
  // of fifty and a CI out of reach are the server's to refuse, and the refusal is said in the
  // dialog. Without the right to change the entry neither naming nor removing is drawn (P-05).

  import { untrack } from 'svelte';
  import { Badge, Button, DataTable, Dialog, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { DataColumn } from '@hubtask/design-system/components';
  import type { ConfigurationItem, EntryCis, EntryOnCi } from '@hubtask/sync-engine';

  import CiPicker from '../assets/CiPicker.svelte';
  import { cmdb, collisionsInOrder, entryCisPath, entryKey, reachedServices } from '../data/cmdb.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    itemId: string;
    /** Whether the reader may change this entry, which naming and removing a CI need. */
    mayChange: boolean;
    /** How many items are named once read: the heading's count and the jump mark's (ET-2). */
    count?: number;
    /** The entry's hub: the picker opens on its inventory. */
    hubId?: string;
  }

  let { itemId, mayChange, count = $bindable(undefined), hubId }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntryCis>(untrack(() => entryCisPath(itemId)));
  // An answer without one of its lists is read as an empty one, so the panel says *none* rather than
  // taking the whole entry down with it.
  const answer = $derived(read.state.status === 'ready' ? withLists(read.state.data) : undefined);
  function withLists(data: EntryCis): EntryCis {
    return { ...data, cis: data.cis ?? [], services: data.services ?? [], related: data.related ?? [], collisions: data.collisions ?? [] };
  }
  const failure = $derived(read.state.status === 'failed' ? renderProblem(read.state.error, messages).message : undefined);
  $effect(() => {
    count = answer?.cis.length;
  });
  const services = $derived(answer ? reachedServices(answer) : []);
  const collisions = $derived(answer ? collisionsInOrder(answer) : []);
  const nameOf = (ciId: string) => answer?.cis.find((each) => each.ci_id === ciId)?.name ?? t('app.assets.unknown_ci');
  const when = (at: string | null) => (at ? formatDateTime(at, messages.locale) : '—');
  const classWord = (key: string) => (messages.has(`app.assets.class.${key}`) ? t(`app.assets.class.${key}`) : key);

  let refusal = $state<string | undefined>(undefined);

  async function remove(ciId: string) {
    refusal = undefined;
    try {
      await cmdb.unaffect(itemId, ciId);
      toast.show({ text: t('app.affected.removed'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  let isOpen = $state(false);
  let chosen = $state<ConfigurationItem | undefined>(undefined);
  let isNaming = $state(false);
  let nameRefusal = $state<string | undefined>(undefined);

  function open() {
    chosen = undefined;
    nameRefusal = undefined;
    isOpen = true;
  }

  async function name() {
    if (!chosen) return;
    isNaming = true;
    nameRefusal = undefined;
    try {
      await cmdb.affect(itemId, chosen.id);
      isOpen = false;
      toast.show({ text: t('app.affected.named'), tone: 'success' });
    } catch (error) {
      nameRefusal = renderProblem(error as never, messages).message;
    } finally {
      isNaming = false;
    }
  }

  const ciColumns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.affected.col_name'), priority: 1 },
    { id: 'class', label: t('app.affected.col_class'), priority: 2 },
    { id: 'stage', label: t('app.affected.col_stage'), priority: 2, isStatus: true },
  ]);
</script>

{#snippet entry(other: EntryOnCi)}
  <a href={`/items/${other.item_id}`} class="entry"><span class="key">{entryKey(other)}</span> {other.title}</a>
{/snippet}

<section class="affected" aria-labelledby="affected-heading-{itemId}" data-affected-cis>
  <div class="head">
    <!-- Named as its jump mark is: one name for one section. -->
    <h2 class="section" id="affected-heading-{itemId}">{t('app.entry.mark_affected')}{#if answer?.cis.length}{' '}<span class="count">{answer.cis.length}</span>{/if}</h2>
    <!-- Nothing named: the section is one line (ET-3). -->
    {#if answer && !failure && answer.cis.length === 0}<span class="quiet none">{t('app.affected.none')}</span>{/if}
    {#if mayChange}
      <Button size="sm" tone="subtle" icon="plus" data-name-ci onclick={open}>{t('app.affected.add')}</Button>
    {/if}
  </div>

  {#if failure}
    <p class="refusal" role="alert">{failure}</p>
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={2} /></div>
  {:else if answer.cis.length > 0}
    <!-- A table, as the ticket's scene draws it (§6.9): what it is, its class and its stage in
         columns; taking one off is the row's menu. -->
    <DataTable
      label={t('app.affected.title')}
      columns={ciColumns}
      rows={answer.cis}
      density="compact"
      key={(ci) => ci.ci_id}
      rowLabel={(ci) => ci.name}
      href={(ci) => `/cis/${ci.ci_id}`}
      menu={mayChange ? (ci) => ({ label: t('app.affected.col_acts'), items: [{ id: 'remove', label: t('app.affected.remove', { name: ci.name }) }] }) : undefined}
      onmenu={(ci) => void remove(ci.ci_id)}
    >
      {#snippet cell(ci, column)}
        {#if column.id === 'name'}<span data-affected-ci={ci.ci_id}>{ci.name}</span>
        {:else if column.id === 'class'}{classWord(ci.class)}
        {:else}<Badge size="sm">{t(`app.assets.stage.${ci.stage}`)}</Badge>{/if}
      {/snippet}
    </DataTable>


    <!-- What it reaches and the other open entries are one line each under the table (the scene's
         foot line); a collision keeps its row, it is a warning. -->
    <div class="group" data-inline>
      <h3 class="relation">{t('app.affected.services')}</h3>
      {#if services.length === 0}
        <p class="quiet">{t('app.affected.services_none')}</p>
      {:else}
        <ul class="rows">
          {#each services as service (service.ci_id)}
            <li class="row" data-reached-service={service.ci_id}>
              <span class="what"><a href={`/cis/${service.ci_id}`}>{service.name}</a><span class="quiet">{classWord(service.class)}</span></span>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    {#if collisions.length > 0}
      <div class="group">
        <h3 class="relation">{t('app.affected.collisions')}</h3>
        <ul class="rows">
          {#each collisions as other (`${other.item_id}:${other.ci_id}`)}
            <li class="row collision" data-collision={other.item_id}>
              <span class="what">
                {@render entry(other)}
                <span class="quiet">
                  {t('app.affected.window', { start: when(other.start_at), due: when(other.due_at) })} ·
                  {other.direct ? t('app.affected.on_ci', { name: nameOf(other.ci_id) }) : t('app.affected.on_reached')}
                </span>
              </span>
              <Badge size="sm" tone="warning">{t('app.affected.collision')}</Badge>
            </li>
          {/each}
        </ul>
      </div>
    {/if}

    {#if answer.related.length > 0}
      <div class="group" data-inline>
        <h3 class="relation">{t('app.affected.related')}</h3>
        <ul class="rows">
          {#each answer.related as other (`${other.item_id}:${other.ci_id}`)}
            <li class="row" data-related-entry={other.item_id}>
              <!-- The key alone on the line, the title its name and its tip. -->
              <a href={`/items/${other.item_id}`} class="entry key" title={other.title} aria-label={`${entryKey(other)} ${other.title}`}>{entryKey(other)}</a>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</section>

{#if isOpen}
  <Dialog size="l" primary={{ label: t('app.affected.confirm'), isBusy: isNaming, disabledReason: chosen ? undefined : t('app.affected.choose_first'), onclick: () => void name() }} bind:isOpen title={t('app.affected.picker_title')} dismissLabel={t('app.assets.cancel')} onClose={() => { chosen = undefined; }}>
    <Stack gap="200">
      <CiPicker {chosen} {hubId} onchoose={(found) => (chosen = found)} />
      {#if nameRefusal}<p class="refusal" role="alert">{nameRefusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .none { flex: 1; min-inline-size: 0; font-size: var(--fs-075); }

  .affected { display: flex; flex-direction: column; gap: var(--sp-100); }
  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .section { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .count { color: var(--text-subtle); font-weight: var(--fw-regular); }
  .group { display: flex; flex-direction: column; gap: var(--sp-050); }
  .relation { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .group[data-inline] { flex-direction: row; flex-wrap: wrap; align-items: baseline; column-gap: var(--sp-150); }
  .group[data-inline] .rows { display: flex; flex-wrap: wrap; column-gap: var(--sp-200); }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .what { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); min-inline-size: 0; }
  .what a, .entry { color: var(--text-primary); font-weight: var(--fw-medium); }
  .key { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); }

</style>
