<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's inventory by class (UC-AST-01 check 8): one heading per class with its CIs, narrowed to
  // a class and its descendants, a page at a time; a member who may write makes a CI by hand,
  // through the same reconciliation every source writes through (as `manual`).

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, Dialog, EmptyState, Input, LoadMore, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiClass, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { cisPath, classWord, cmdb, instantiable, inventoryByClass, missingRequired, parseAttribute, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import AttributeField from './AttributeField.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
  }

  const { hubId, classes, words, mayWrite, onnavigate }: Props = $props();

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
  let draftClass = $state('');
  let draftName = $state('');
  let drafts = $state<Record<string, string>>({});
  const chosen = $derived(classes.find((each) => each.key === draftClass));
  const missing = $derived(chosen ? missingRequired(chosen.all_attributes, drafts) : []);

  function startCreating() {
    draftClass = shown && offered.some((each) => each.key === shown) ? shown : '';
    draftName = '';
    drafts = {};
    isCreating = true;
  }

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
      toast.show({ text: t('app.assets.attribute_invalid', { keys: invalid.join(', ') }), tone: 'danger' });
      return;
    }
    try {
      const made = await cmdb.create(hubId, { class: chosen.key, name: draftName.trim(), attributes });
      isCreating = false;
      onnavigate(`/cis/${made.id}`);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
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
  <div class="tools">
    <Select
      label={t('app.assets.filter_class')}
      size="sm"
      options={[{ value: '', label: t('app.assets.all_classes') }, ...classes.filter((each) => each.key !== 'ci').map((each) => ({ value: each.key, label: classWord(each.key, classes, words) }))]}
      bind:value={shown}
    />
    {#if mayWrite}<Button icon="plus" onclick={startCreating}>{t('app.assets.new')}</Button>{/if}
  </div>

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !first}
    <Spinner label={t('app.assets.loading')} />
  {:else if all.length === 0}
    <EmptyState kind="unused" title={t('app.assets.empty')} description={t('app.assets.empty_hint')} icon="shield">
      {#snippet action()}
        {#if mayWrite}<Button icon="plus" onclick={startCreating}>{t('app.assets.new')}</Button>{/if}
      {/snippet}
    </EmptyState>
  {:else}
    {#each groups as group (group.class)}
      <section class="group">
        <h2 class="heading">{classWord(group.class, classes, words)} <span class="meta">{t('app.assets.count', { count: group.items.length })}</span></h2>
        <ul class="rows">
          {#each group.items as ci (ci.id)}
            <li class="row">
              <a href={`/cis/${ci.id}`}>{ci.name}</a>
              <span class="facts">
                <Badge size="sm">{t(`app.assets.stage.${ci.stage}`)}</Badge>
                {#if ci.status !== 'OK'}<Badge size="sm" tone="warning">{t(`app.assets.status.${ci.status}`)}</Badge>{/if}
              </span>
            </li>
          {/each}
        </ul>
      </section>
    {/each}
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
</Stack>

{#if isCreating}
  <Dialog bind:isOpen={isCreating} title={t('app.assets.new')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      <Select label={t('app.assets.class_label')} options={classOptions} placeholder={t('app.assets.needs_class')} bind:value={draftClass} />
      <Input label={t('app.assets.name_label')} bind:value={draftName} maxlength={200} />
      {#if chosen}
        {#each chosen.all_attributes as attribute (attribute.key)}
          <AttributeField {attribute} bind:value={drafts[attribute.key]} />
        {/each}
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isCreating = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={createReason} onclick={() => void create()}>{t('app.assets.create')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .tools { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .group { display: grid; gap: var(--sp-050); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .row a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  .row a:hover { text-decoration: underline; }
  .facts { display: inline-flex; gap: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); font-weight: var(--fw-regular); }
</style>
