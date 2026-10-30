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
  import { Badge, Button, Dialog, IconButton, Skeleton, Stack } from '@hubtask/design-system/components';
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
  }

  const { itemId, mayChange }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<EntryCis>(untrack(() => entryCisPath(itemId)));
  const answer = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const failure = $derived(read.state.status === 'failed' ? renderProblem(read.state.error, messages).message : undefined);
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
</script>

{#snippet entry(other: EntryOnCi)}
  <a href={`/items/${other.item_id}`} class="entry"><span class="key">{entryKey(other)}</span> {other.title}</a>
{/snippet}

<section class="affected" aria-labelledby="affected-heading-{itemId}" data-affected-cis>
  <div class="head">
    <h2 class="section" id="affected-heading-{itemId}">{t('app.affected.title')}</h2>
    {#if mayChange}
      <Button size="sm" tone="subtle" icon="plus" data-name-ci onclick={open}>{t('app.affected.add')}</Button>
    {/if}
  </div>

  {#if failure}
    <p class="refusal" role="alert">{failure}</p>
  {:else if !answer}
    <div aria-busy="true"><Skeleton lines={2} /></div>
  {:else if answer.cis.length === 0}
    <p class="quiet">{t('app.affected.none')}</p>
  {:else}
    <ul class="rows">
      {#each answer.cis as ci (ci.ci_id)}
        <li class="row" data-affected-ci={ci.ci_id}>
          <span class="what">
            <a href={`/cis/${ci.ci_id}`}>{ci.name}</a>
            <span class="quiet">{classWord(ci.class)}</span>
            <Badge size="sm">{t(`app.assets.stage.${ci.stage}`)}</Badge>
          </span>
          {#if mayChange}
            <IconButton
              icon="x" size="sm" tone="subtle"
              label={t('app.affected.remove', { name: ci.name })}
              data-unaffect-ci={ci.ci_id}
              onclick={() => void remove(ci.ci_id)}
            />
          {/if}
        </li>
      {/each}
    </ul>

    <div class="group">
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
      <div class="group">
        <h3 class="relation">{t('app.affected.related')}</h3>
        <ul class="rows">
          {#each answer.related as other (`${other.item_id}:${other.ci_id}`)}
            <li class="row" data-related-entry={other.item_id}>
              <span class="what">{@render entry(other)}</span>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
  {/if}
  {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
</section>

{#if isOpen}
  <Dialog bind:isOpen title={t('app.affected.picker_title')} dismissLabel={t('app.assets.cancel')} onClose={() => { chosen = undefined; }}>
    <Stack gap="200">
      <CiPicker {chosen} onchoose={(found) => (chosen = found)} />
      {#if nameRefusal}<p class="refusal" role="alert">{nameRefusal}</p>{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button
        tone="primary"
        data-confirm-ci
        isBusy={isNaming}
        disabledReason={chosen ? undefined : t('app.affected.choose_first')}
        onclick={name}
      >{t('app.affected.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .affected { display: flex; flex-direction: column; gap: var(--sp-100); }
  .head { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .section { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .group { display: flex; flex-direction: column; gap: var(--sp-050); }
  .relation { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-medium); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .row { display: flex; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .what { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); min-inline-size: 0; }
  .what a, .entry { color: var(--text-primary); font-weight: var(--fw-medium); }
  .key { color: var(--text-secondary); font-variant-numeric: tabular-nums; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
