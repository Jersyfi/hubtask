<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Erstellen* (work-surface concept §6.12, C-1…C-6): the capture line on top - the one way to
  // make an entry from a sentence (UC-WRK-26/7) -, below it the kinds one may create where one
  // stands, the departments one may ask, and at the foot template and import.
  //
  // **It decides nothing of its own.** Which kinds follow from the role at the hub, the modules on
  // there and whether one may capture is `kinds.ts`'s; each kind then opens the door its product
  // already has - capture for an entry, the goal's and the time's dialog on their screens, the
  // inventory's on its page - or, for the large ones, makes the object at once and opens it (C-3).
  //
  // On a phone the same content is a sheet over the bottom bar (§7); the ＋ that opens it stays
  // in its place, and a long press on it makes the last kind (`AppFrame`).
  import { untrack } from 'svelte';
  import { Dialog, Icon, Kbd, ProductTile, Sheet, type IconName } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { TransportError } from '@hubtask/sync-engine';

  import { create } from './create.svelte.ts';
  import { AT_ONCE, LETTERS, TILES, kindOfLetter, kindsHere, lastHere, type CreateKind } from './kinds.ts';
  import { capture } from '../capture/capture.svelte.ts';
  import { mayCaptureHere } from '../capture/places.svelte.ts';
  import CreateContainerDialog from '../workspace/CreateContainerDialog.svelte';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { holds, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { goals } from '../data/goals.svelte.ts';
  import { modules } from '../data/modules.svelte.ts';
  import { modulesHere } from '../data/modules.ts';
  import { pages } from '../data/pages.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { whiteboards } from '../data/whiteboards.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Where the reader stands, as the route names it: the place every kind is made in (C-2). */
    here?: { readonly hub?: string; readonly collection?: string; readonly item?: string };
    onnavigate: (path: string) => void;
  }

  const { here = {}, onnavigate }: Props = $props();

  let line = $state('');
  let failure = $state<string | undefined>(undefined);
  let isBusy = $state(false);
  let isCreatingCollection = $state(false);
  let collectionHub = $state<string | undefined>(undefined);

  const isOpen = $derived(create.isOpen);
  const collection = $derived(here.collection ? containers.find(here.collection) : undefined);
  const hub = $derived.by(() => {
    const id = here.hub ?? collection?.parent_id ?? undefined;
    return id ? containers.find(id) : undefined;
  });

  // The role at the hub and the modules on there, read while it is open - the questions the hub's
  // own *New* asks.
  let hubModules = $state<readonly string[] | undefined>(undefined);
  $effect(() => {
    const hubId = isOpen ? hub?.id : undefined;
    if (!hubId) return;
    return untrack(() => {
      const closePeople = people.open({ hubId });
      const stop = modules.here(hubId, (next) => {
        if (next.status === 'ready') hubModules = next.data.modules_on;
      });
      return () => {
        closePeople?.();
        stop();
      };
    });
  });

  const role = $derived(hub ? (people.roleOf({ hubId: hub.id }, actor.account?.id, roleOrder()) ?? undefined) : undefined);
  const kinds = $derived(
    kindsHere({
      mayCapture: mayCaptureHere(),
      hubId: hub?.id,
      modulesOn: modulesHere(manifest.value?.modules_on, hubModules) as readonly string[],
      mayWrite: holds(role, 'WRITE_ITEMS').status === 'permitted',
      mayShape: holds(role, 'STRUCTURE').status === 'permitted',
    }),
  );
  const last = $derived(lastHere(create.last, kinds));
  const placeName = $derived(collection?.name ?? hub?.name);
  const portals = $derived(actor.portals);

  // The ＋ held: the kind made last, made at once once the place allows it (C-5).
  $effect(() => {
    const kind = isOpen && create.isDirect ? last[0] : undefined;
    if (!kind) return;
    untrack(() => {
      if (create.takeDirect()) void choose(kind);
    });
  });

  // A new opening starts with an empty line and no failure.
  $effect(() => {
    if (!isOpen) return;
    untrack(() => {
      line = '';
      failure = undefined;
    });
  });

  const door = $derived({ ...(collection ? { collectionId: collection.id } : {}), ...(hub ? { hubId: hub.id } : {}) });

  function takeLine(event: SubmitEvent) {
    event.preventDefault();
    const typed = line.trim();
    create.close();
    create.note('entry');
    capture.open({ ...door, ...(typed === '' ? {} : { text: typed }) });
  }

  async function atOnce(kind: CreateKind, hubId: string): Promise<string> {
    const title = t('app.create.untitled', { kind });
    if (kind === 'page') return `/pages/${(await pages.create(hubId, { title })).id}`;
    if (kind === 'whiteboard') return `/whiteboards/${(await whiteboards.create(hubId, { title })).id}`;
    return `/dashboards/${(await goals.createDashboard({ hub_id: hubId, name: title })).id}`;
  }

  async function choose(kind: CreateKind) {
    if (isBusy) return;
    create.note(kind);
    if (AT_ONCE.has(kind) && hub) {
      isBusy = true;
      failure = undefined;
      try {
        const path = await atOnce(kind, hub.id);
        create.close();
        onnavigate(path);
      } catch (error) {
        failure = renderProblem(error as TransportError, messages).message;
      } finally {
        isBusy = false;
      }
      return;
    }
    create.close();
    if (kind === 'entry') return capture.open(door);
    if (kind === 'collection') {
      collectionHub = hub?.id;
      isCreatingCollection = true;
      return;
    }
    if (kind === 'asset' && hub) return onnavigate(`/hubs/${hub.id}/inventory?new=1`);
    create.wish(kind);
    if (kind === 'goal') return onnavigate(hub ? `/hubs/${hub.id}/goals` : '/goals');
    if (kind === 'time') return onnavigate('/time');
  }

  /** The foot's two ways into capture: from a template, or a file to import (C-4). */
  function captureAs(mode: 'template' | 'import') {
    create.close();
    capture.open({ ...door, mode });
  }

  /** A kind's letter jumps to it while the line is not being typed in (C-5). */
  function keydown(event: KeyboardEvent) {
    if (event.metaKey || event.ctrlKey || event.altKey) return;
    if (event.target instanceof HTMLInputElement && event.target.value !== '') return;
    const kind = event.key.length === 1 ? kindOfLetter(event.key, kinds) : undefined;
    if (!kind) return;
    event.preventDefault();
    void choose(kind);
  }
</script>

{#snippet kindButton(kind: CreateKind, isLast: boolean)}
  {@const tile = TILES[kind]}
  {@const letter = LETTERS[kind]}
  <button type="button" class="kind" data-kind={kind} disabled={isBusy} aria-keyshortcuts={letter} onclick={() => void choose(kind)}>
    <ProductTile icon={tile.icon as IconName} product={tile.product} color={tile.product ? undefined : 'brand'} size="sm" />
    <span class="words">
      <span class="name">{t('app.create.kind', { kind })}</span>
      <span class="sub">{isLast ? (placeName ?? '') : t('app.create.kind_sub', { kind })}</span>
    </span>
    {#if letter}<span class="letter" aria-hidden="true"><Kbd size="sm">{letter}</Kbd></span>{/if}
  </button>
{/snippet}

{#snippet body()}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div class="create" role="group" aria-label={t('app.create.title')} onkeydown={keydown}>
    {#if mayCaptureHere()}
      <form class="line" onsubmit={takeLine}>
        <Icon name="plus" />
        <!-- svelte-ignore a11y_autofocus -->
        <input class="field" type="text" bind:value={line} placeholder={t('app.create.line')} aria-label={t('app.create.line_label')} autocomplete="off" autofocus />
        {#if placeName}<span class="place"><Icon name="hub" size="sm" />{t('app.create.in', { name: placeName })}</span>{/if}
      </form>
    {/if}
    {#if failure}<FailureNote problem={failure} />{/if}

    {#if viewport.isCompact}
      <div class="grid phone" role="group" aria-label={t('app.create.kinds')}>
        {#each kinds as kind (kind)}
          <button type="button" class="tilebutton" data-kind={kind} disabled={isBusy} onclick={() => void choose(kind)}>
            <ProductTile icon={TILES[kind].icon as IconName} product={TILES[kind].product} color={TILES[kind].product ? undefined : 'brand'} size="lg" />
            <span>{t('app.create.kind', { kind })}</span>
          </button>
        {/each}
        {#each portals.slice(0, 1) as portal (portal.collection_id)}
          <button type="button" class="tilebutton" onclick={() => { create.close(); onnavigate(`/portal/${portal.collection_id}`); }}>
            <ProductTile icon="inbox" product="desk" size="lg" />
            <span>{t('app.create.request')}</span>
          </button>
        {/each}
      </div>
      {#if create.last[0]}<p class="hint">{t('app.create.long_press')}</p>{/if}
    {:else}
      {#if last.length > 0}
        <section aria-labelledby="create-last">
          <h3 class="head" id="create-last">{t('app.create.last')}</h3>
          <div class="grid">{#each last as kind (kind)}{@render kindButton(kind, true)}{/each}</div>
        </section>
      {/if}
      {#if kinds.length > 0}
        <section aria-labelledby="create-here">
          <h3 class="head" id="create-here">{hub ? t('app.create.here', { name: hub.name }) : t('app.create.anywhere')}</h3>
          <div class="grid">{#each kinds as kind (kind)}{@render kindButton(kind, false)}{/each}</div>
        </section>
      {/if}
      {#if portals.length > 0}
        <!-- Every department one may ask (PO-1); its own directory page is the portal family's. -->
        <section aria-labelledby="create-ask">
          <h3 class="head" id="create-ask">{t('app.create.ask')}</h3>
          <div class="grid">
            {#each portals as portal (portal.collection_id)}
              <button type="button" class="kind" onclick={() => { create.close(); onnavigate(`/portal/${portal.collection_id}`); }}>
                <ProductTile icon="inbox" product="desk" size="sm" />
                <span class="words"><span class="name">{portal.name}</span><span class="sub">{t('app.create.ask_sub')}</span></span>
              </button>
            {/each}
          </div>
        </section>
      {/if}
      <footer class="foot">
        {#if mayCaptureHere()}
          <button type="button" class="footbutton" onclick={() => captureAs('template')}><Icon name="copy" size="sm" />{t('app.create.from_template')}</button>
          <button type="button" class="footbutton" onclick={() => captureAs('import')}><Icon name="download" size="sm" />{t('app.create.import')}</button>
        {/if}
        <span class="keys" aria-hidden="true">
          <Kbd size="sm">C</Kbd> {t('app.create.keys_open')}
          {#each kinds.filter((kind) => LETTERS[kind]).slice(0, 3) as kind (kind)}<Kbd size="sm">{LETTERS[kind]}</Kbd>{/each}
          {t('app.create.keys_jump')}
        </span>
      </footer>
    {/if}
  </div>
{/snippet}

{#if viewport.isCompact}
  <Sheet title={t('app.create.title')} closeLabel={t('app.create.close')} isOpen={isOpen} onClose={() => create.close()}>
    {@render body()}
  </Sheet>
{:else}
  <Dialog size="l" title={t('app.create.title')} dismissLabel={t('app.create.close')} isOpen={isOpen} onClose={() => create.close()}>
    {@render body()}
  </Dialog>
{/if}

{#if isCreatingCollection && collectionHub}
  <CreateContainerDialog bind:isOpen={isCreatingCollection} type="COLLECTION" parentId={collectionHub} oncreated={(id, landing) => onnavigate(landing ?? `/collections/${id}`)} />
{/if}

<style>
  .create { display: flex; flex-direction: column; gap: var(--sp-150); }

  .line {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-raised);
    color: var(--text-secondary);
  }

  .line:focus-within { border-color: var(--accent-primary); }
  .field { flex: 1 1 auto; min-inline-size: 0; border: 0; background: transparent; color: var(--text-primary); font: inherit; outline: none; }

  .place {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    flex: none;
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-md);
    background: var(--accent-primary-subtle);
    color: var(--text-primary);
    font-size: var(--fs-075);
  }

  .head { margin: 0 0 var(--sp-050); color: var(--text-secondary); font-size: var(--fs-065); font-weight: var(--fw-medium); text-transform: uppercase; }

  .grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--sp-050); }
  .grid.phone { grid-template-columns: repeat(4, minmax(0, 1fr)); gap: var(--sp-150); }

  .kind {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--density-control-md-min);
    padding: var(--sp-100);
    border: 0;
    border-radius: var(--r-md);
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .kind:hover, .kind:focus-visible { background: var(--accent-primary-subtle); }
  .words { display: flex; flex: 1 1 auto; flex-direction: column; min-inline-size: 0; }
  .name { font-size: var(--fs-090); }
  .sub { color: var(--text-secondary); font-size: var(--fs-065); }
  .letter { flex: none; }

  .tilebutton {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--sp-050);
    padding: var(--sp-050);
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    font-size: var(--fs-075);
    cursor: pointer;
  }

  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-065); text-align: center; }

  .foot {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-150);
    padding-block-start: var(--sp-100);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .footbutton {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    border: 0;
    background: transparent;
    color: var(--text-primary);
    font: inherit;
    cursor: pointer;
  }

  .keys { display: inline-flex; align-items: center; gap: var(--sp-050); margin-inline-start: auto; }
</style>
