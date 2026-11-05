<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's inventory by class (UC-AST-01 check 8): one heading per class with its CIs, narrowed to
  // a class and its descendants, a page at a time; a member who may write makes a CI by hand,
  // through the same reconciliation every source writes through (as `manual`). The head's create
  // opens the dialog here (`isAsked`); an empty inventory offers it with the import beside it.

  import { untrack } from 'svelte';
  import { Badge, Button, Dialog, Input, ListRow, LoadMore, Select, Skeleton, Stack, Toolbar } from '@hubtask/design-system/components';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiClass, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { cisPath, classWord, cmdb, instantiable, inventoryByClass, missingRequired, parseAttribute, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ProductEmpty from '../product/ProductEmpty.svelte';
  import PlaceChip from '../product/PlaceChip.svelte';
  import { createPlaces } from '../product/places.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import AttributeField from './AttributeField.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
    /** The head asked for a new CI; the dialog opens once and `onasked` gives the ask back. */
    isAsked?: boolean;
    onasked?: () => void;
    /** Where an empty inventory's *Import a file* goes. */
    onimport?: () => void;
  }

  const { hubId, classes, words, mayWrite, onnavigate, isAsked = false, onasked, onimport }: Props = $props();

  let shown = $state('');
  let first = $state<ConfigurationItemList | undefined>(undefined);
  let more = $state<readonly ConfigurationItem[]>([]);
  let next = $state<string | null | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);

  $effect(() => {
    const at = hubId;
    const narrowed = shown;
    return untrack(() =>
      engine.subscribe<ConfigurationItemList>({ path: cisPath(at, narrowed ? { class: narrowed } : {}) }, (state) => {
        if (state.status === 'ready') {
          first = state.data;
          more = [];
          next = state.data.next;
        }
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      }),
    );
  });

  const all = $derived([...(first?.data ?? []), ...more]);
  const groups = $derived(inventoryByClass(all, classes));

  async function loadMore() {
    if (!next) return;
    isLoadingMore = true;
    moreFailed = undefined;
    try {
      const page = await cmdb.page(hubId, next, shown || undefined);
      more = [...more, ...page.data];
      next = page.next;
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  const offered = $derived(instantiable(classes));
  const classOptions = $derived(offered.map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));

  let isCreating = $state(false);
  let createFailure = $state<string | undefined>(undefined);
  let draftClass = $state('');
  let draftName = $state('');
  let drafts = $state<Record<string, string>>({});
  /** Where the CI is made (§5.2): this hub, or another where the assets are on; the classes are the workspace's. */
  let draftHub = $state('');
  const placesToMake = createPlaces('ASSETS', 'HUB', () => isCreating);
  const chosen = $derived(classes.find((each) => each.key === draftClass));
  const missing = $derived(chosen ? missingRequired(chosen.all_attributes, drafts) : []);

  function startCreating() {
    draftClass = shown && offered.some((each) => each.key === shown) ? shown : '';
    draftName = '';
    drafts = {};
    draftHub = hubId;
    createFailure = undefined;
    isCreating = true;
  }

  $effect(() => {
    if (!isAsked || !mayWrite) return;
    untrack(() => {
      startCreating();
      onasked?.();
    });
  });

  async function create() {
    if (!chosen) return;
    const attributes: Record<string, unknown> = {};
    const invalid: string[] = [];
    for (const attribute of chosen.all_attributes) {
      const parsed = parseAttribute(attribute, drafts[attribute.key] ?? '');
      if (!parsed.ok) invalid.push(attribute.key);
      else if (parsed.value !== null) attributes[attribute.key] = parsed.value;
    }
    if (invalid.length > 0) {
      createFailure = t('app.assets.attribute_invalid', { keys: invalid.join(', ') });
      return;
    }
    try {
      const made = await cmdb.create(draftHub || hubId, { class: chosen.key, name: draftName.trim(), attributes });
      isCreating = false;
      onnavigate(`/cis/${made.id}`);
    } catch (error) {
      // Said inside the dialog, which keeps what was typed (user-experience.md §5.10).
      createFailure = renderProblem(error as never, messages).message;
    }
  }

  const createReason = $derived(
    !draftClass ? t('app.assets.needs_class')
    : !draftName.trim() ? t('app.assets.needs_name')
    : missing.length > 0 ? t('app.assets.required_missing', { keys: missing.join(', ') })
    : undefined,
  );
</script>

<Stack gap="150">
  {#if refusal}
    <FailureNote problem={refusal} />
  {:else if !first}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if all.length === 0 && !shown}
    <ProductEmpty product="ASSETS" place={{ type: 'HUB', id: hubId }}
      kind="unused"
      icon="shield"
      title={t('app.assets.empty')}
      description={mayWrite ? t('app.assets.empty_hint') : t('app.assets.empty_reader')}
      create={mayWrite ? { label: t('app.assets.new'), onclick: startCreating } : undefined}
      templates={mayWrite && onimport ? [{ label: t('app.assets.create_import'), onclick: onimport }] : []}
    />
  {:else}
    <Toolbar label={t('app.assets.inventory_tools')}>
      <Select
        label={t('app.assets.filter_class')}
        size="sm"
        options={[{ value: '', label: t('app.assets.all_classes') }, ...classes.filter((each) => each.key !== 'ci').map((each) => ({ value: each.key, label: classWord(each.key, classes, words) }))]}
        bind:value={shown}
      />
    </Toolbar>
    {#if all.length === 0}
      <ProductEmpty kind="filtered" icon="funnel" title={t('app.assets.empty_class')} clear={{ label: t('app.assets.all_classes'), onclick: () => (shown = '') }} />
    {/if}
    {#each groups as group (group.class)}
      <section class="group" aria-labelledby={`class-${group.class}`}>
        <h2 class="heading" id={`class-${group.class}`}>{classWord(group.class, classes, words)} <span class="meta">{t('app.assets.count', { count: group.items.length })}</span></h2>
        <ul class="rows">
          {#each group.items as ci (ci.id)}
            <li>
              <ListRow href={`/cis/${ci.id}`} onactivate={() => onnavigate(`/cis/${ci.id}`)} label={ci.name}>
                <span class="name" title={ci.name}>{ci.name}</span>
                {#snippet trailing()}
                  <span class="facts">
                    <Badge size="sm">{t(`app.assets.stage.${ci.stage}`)}</Badge>
                    {#if ci.status !== 'OK'}<Badge size="sm" tone="warning">{t(`app.assets.status.${ci.status}`)}</Badge>{/if}
                  </span>
                {/snippet}
              </ListRow>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
    {#if all.length > 0}
      <LoadMore
        label={t('app.assets.load_more')}
        busyLabel={t('app.assets.loading_more')}
        hasMore={Boolean(next)}
        isBusy={isLoadingMore}
        endLabel={t('app.assets.end')}
        {...(moreFailed ? { failedLabel: moreFailed } : {})}
        onLoadMore={() => void loadMore()}
      />
    {/if}
  {/if}
</Stack>

{#if isCreating}
  <Dialog bind:isOpen={isCreating} title={t('app.assets.new')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      <Select label={t('app.assets.class_label')} options={classOptions} placeholder={t('app.assets.needs_class')} bind:value={draftClass} />
      <Input label={t('app.assets.name_label')} bind:value={draftName} maxlength={200} />
      {#if chosen}
        {#each chosen.all_attributes as attribute (attribute.key)}
          <AttributeField {attribute} bind:value={() => drafts[attribute.key] ?? '', (typed: string) => (drafts[attribute.key] = typed)} />
        {/each}
      {/if}
      <PlaceChip current={draftHub} fallback={containers.find(hubId)?.name} places={placesToMake.list} onchoose={(id) => (draftHub = id)} />
      {#if createFailure}<FailureNote problem={createFailure} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCreating = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={createReason} onclick={() => void create()}>{t('app.assets.create')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .group { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-050); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-025); }
  .name { display: block; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: var(--fw-medium); }
  .facts { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); font-weight: var(--fw-regular); }
</style>
