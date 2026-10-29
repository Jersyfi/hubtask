<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Configuration items* under *How we work* (M6, UC-AST-02 check 6, UC-AST-06 check 5): the class
  // hierarchy Hubtask ships with the workspace's own classes beneath it, the relation catalogue with
  // which end suffers when the other fails, and the sources by precedence. An administrator adds
  // and changes the workspace's own; a shipped class or type is read, never changed.
  //
  // **What the reader may never do is not drawn** (P-05): without STRUCTURE at the workspace the
  // page lists the definitions and offers no control.

  import {
    Banner, Button, Checkbox, CheckboxGroup, Dialog, IconButton, Input, PageHeader, Select, Spinner, Stack, Switch, Tabs, Textarea,
    type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiAttribute, CiClass, CiClassList, CiRelationType, CiRelationTypeList, CiSource, CiSourceDefine, CiSourceList } from '@hubtask/sync-engine';

  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import {
    classDefine, classDraft, classesPath, classWord, cmdb, keyIsWellFormed, relationTypesPath, relationWord, sourcesPath, type ClassDraft, type Words,
  } from '../lib/data/cmdb.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => page.entitle(t('app.cmdb_admin.title')));

  const mayShape = $derived(holdsHere('STRUCTURE') === true);
  const words: Words = (code) => (messages.has(code) ? t(code) : undefined);

  const classesRead = resource<CiClassList>(classesPath);
  const typesRead = resource<CiRelationTypeList>(relationTypesPath);
  const sourcesRead = resource<CiSourceList>(sourcesPath);
  const classes = $derived(classesRead.state.status === 'ready' ? classesRead.state.data.data : []);
  const types = $derived(typesRead.state.status === 'ready' ? typesRead.state.data.data : []);
  const sources = $derived(sourcesRead.state.status === 'ready' ? [...sourcesRead.state.data.data].sort((a, b) => a.rank - b.rank) : []);
  const refusal = $derived(classesRead.state.status === 'failed' ? renderProblem(classesRead.state.error, messages).message : undefined);

  let shown = $state('classes');
  const tabs = $derived([
    { id: 'classes', label: t('app.cmdb_admin.classes') },
    { id: 'types', label: t('app.cmdb_admin.types') },
    { id: 'sources', label: t('app.cmdb_admin.sources') },
  ]);

  const nameOf = (key: string) => classWord(key, classes, words);
  const depthOf = (each: CiClass) => Math.max(0, each.lineage.length - 1);
  /** The hierarchy in reading order: each class after its parent, siblings by their order in the list. */
  const tree = $derived.by(() => {
    const out: CiClass[] = [];
    const visit = (parent: string) => {
      for (const each of classes.filter((one) => one.parent === parent)) {
        out.push(each);
        visit(each.key);
      }
    };
    const roots = classes.filter((each) => !each.parent);
    for (const root of roots) {
      out.push(root);
      visit(root.key);
    }
    return out;
  });

  async function act<T>(run: () => Promise<T>, done: string): Promise<T | undefined> {
    try {
      const out = await run();
      toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  const kinds: readonly CiAttribute['kind'][] = ['TEXT', 'NUMBER', 'BOOLEAN', 'DATE', 'CHOICE', 'URL', 'GROUP', 'ACCOUNT'];
  const kindOptions = $derived<SelectOption[]>(kinds.map((each) => ({ value: each, label: t(`app.cmdb_admin.kind.${each}`) })));
  const needOptions = $derived<SelectOption[]>((['OPTIONAL', 'RECOMMENDED', 'REQUIRED'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.need.${each}`) })));
  const classOptions = $derived<SelectOption[]>(tree.map((each) => ({ value: each.key, label: nameOf(each.key) })));

  // --- A class ---------------------------------------------------------------------------------

  let classEditing = $state<CiClass | undefined>(undefined);
  let classDraftState = $state<ClassDraft | undefined>(undefined);
  let isClassOpen = $state(false);

  function openClass(of?: CiClass) {
    classEditing = of;
    classDraftState = classDraft(of);
    isClassOpen = true;
  }

  const classReason = $derived(
    !classDraftState ? undefined
    : !classEditing && !keyIsWellFormed(classDraftState.key) ? t('app.cmdb_admin.key_hint')
    : !classDraftState.name.trim() ? t('app.cmdb_admin.needs_name')
    : undefined,
  );

  async function saveClass() {
    const draft = classDraftState;
    if (!draft) return;
    const done = await act(() => cmdb.defineClass(classEditing?.key ?? draft.key, classDefine(draft), classEditing?.version), t('app.cmdb_admin.saved'));
    if (done) isClassOpen = false;
  }

  const typeKeys = $derived<SelectOption[]>(types.map((each) => ({ value: each.key, label: relationWord(each, each.key, true, words) })));

  // --- A relation type -------------------------------------------------------------------------

  interface TypeDraft {
    key: string;
    name: string;
    reverse: string;
    impact: CiRelationType['impact'];
    pairs: { from: string; to: string }[];
    archived: boolean;
  }
  let typeEditing = $state<CiRelationType | undefined>(undefined);
  let typeDraft = $state<TypeDraft | undefined>(undefined);
  let isTypeOpen = $state(false);

  function openType(of?: CiRelationType) {
    typeEditing = of;
    typeDraft = {
      key: of?.key ?? '', name: of?.name ?? '', reverse: of?.reverse_name ?? '', impact: of?.impact ?? 'SOURCE',
      pairs: (of?.pairs ?? []).map((pair) => ({ ...pair })), archived: of?.archived ?? false,
    };
    isTypeOpen = true;
  }

  const typeReason = $derived(
    !typeDraft ? undefined
    : !typeEditing && !keyIsWellFormed(typeDraft.key) ? t('app.cmdb_admin.key_hint')
    : !typeDraft.name.trim() || !typeDraft.reverse.trim() ? t('app.cmdb_admin.needs_both_names')
    : undefined,
  );

  async function saveType() {
    const draft = typeDraft;
    if (!draft) return;
    const body = {
      name: draft.name.trim(), reverse_name: draft.reverse.trim(), impact: draft.impact,
      pairs: draft.pairs.filter((pair) => pair.from && pair.to), archived: draft.archived,
    };
    const done = await act(() => cmdb.defineRelationType(typeEditing?.key ?? draft.key, body, typeEditing?.version), t('app.cmdb_admin.saved'));
    if (done) isTypeOpen = false;
  }

  const impactOptions = $derived<SelectOption[]>((['NONE', 'SOURCE', 'TARGET', 'BOTH'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.impact.${each}`) })));

  // --- A source --------------------------------------------------------------------------------

  interface SourceDraft {
    key: string;
    kind: CiSourceDefine['kind'];
    rank: string;
    classes: string[];
    mayCreate: boolean;
  }
  let sourceEditing = $state<CiSource | undefined>(undefined);
  let sourceDraft = $state<SourceDraft | undefined>(undefined);
  let isSourceOpen = $state(false);

  function openSource(of?: CiSource) {
    sourceEditing = of;
    sourceDraft = {
      key: of?.key ?? '', kind: of && of.kind !== 'MANUAL' ? of.kind : 'FILE', rank: String(of?.rank ?? 50),
      classes: [...(of?.classes ?? [])], mayCreate: of?.may_create ?? true,
    };
    isSourceOpen = true;
  }

  const rankOf = (text: string) => Number.parseInt(text, 10);
  const sourceReason = $derived(
    !sourceDraft ? undefined
    : !sourceEditing && (!keyIsWellFormed(sourceDraft.key) || sourceDraft.key === 'manual') ? t('app.cmdb_admin.key_hint')
    : !(rankOf(sourceDraft.rank) >= 1 && rankOf(sourceDraft.rank) <= 1000) ? t('app.cmdb_admin.rank_hint')
    : undefined,
  );

  async function saveSource() {
    const draft = sourceDraft;
    if (!draft) return;
    const body: CiSourceDefine = {
      kind: draft.kind, rank: rankOf(draft.rank), classes: draft.classes, may_create: draft.mayCreate,
      ...(sourceEditing?.mapping ? { mapping: sourceEditing.mapping } : {}),
    };
    const done = await act(() => cmdb.defineSource(sourceEditing?.key ?? draft.key, body, sourceEditing?.version), t('app.cmdb_admin.saved'));
    if (done) isSourceOpen = false;
  }

  const sourceKinds = $derived<SelectOption[]>((['FILE', 'API', 'CONNECTOR'] as const).map((each) => ({ value: each, label: t(`app.cmdb_admin.source_kind.${each}`) })));
  const classChecks = $derived(tree.filter((each) => each.key !== 'ci').map((each) => ({ value: each.key, label: nameOf(each.key) })));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.cmdb_admin.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'cmdb', label: t('app.cmdb_admin.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if classesRead.state.status !== 'ready'}
    <Spinner label={t('app.assets.loading')} />
  {:else}
    <Tabs label={t('app.cmdb_admin.title')} {tabs} bind:selected={shown}>
      {#if shown === 'classes'}
        <Stack gap="100">
          <p class="meta">{t('app.cmdb_admin.classes_hint')}</p>
          {#if mayShape}<div><Button icon="plus" onclick={() => openClass()}>{t('app.cmdb_admin.new_class')}</Button></div>{/if}
          <ul class="rows">
            {#each tree as each (each.key)}
              <li class="row" style:--depth={depthOf(each)}>
                <span class="what">
                  <strong class="indent">{nameOf(each.key)}</strong>
                  <span class="meta">
                    {each.key}{#if each.abstract} · {t('app.cmdb_admin.abstract')}{/if}{#if each.system} · {t('app.cmdb_admin.shipped')}{/if}{#if each.archived} · {t('app.cmdb_admin.archived')}{/if}
                    · {t('app.cmdb_admin.attribute_count', { count: each.all_attributes.length })} · {t('app.cmdb_admin.staleness', { days: each.staleness_days })}
                  </span>
                </span>
                {#if mayShape && !each.system}
                  <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.cmdb_admin.edit_named', { name: nameOf(each.key) })} onclick={() => openClass(each)} />
                {/if}
              </li>
            {/each}
          </ul>
        </Stack>
      {:else if shown === 'types'}
        <Stack gap="100">
          <p class="meta">{t('app.cmdb_admin.types_hint')}</p>
          {#if mayShape}<div><Button icon="plus" onclick={() => openType()}>{t('app.cmdb_admin.new_type')}</Button></div>{/if}
          <ul class="rows">
            {#each types as each (each.key)}
              <li class="row">
                <span class="what">
                  <strong>{relationWord(each, each.key, true, words)} / {relationWord(each, each.key, false, words)}</strong>
                  <span class="meta">
                    {each.key} · {t(`app.cmdb_admin.impact.${each.impact}`)}{#if each.system} · {t('app.cmdb_admin.shipped')}{/if}{#if each.archived} · {t('app.cmdb_admin.archived')}{/if}
                    {#if each.pairs.length > 0} · {each.pairs.map((pair) => t('app.cmdb_admin.pair', { from: nameOf(pair.from), to: nameOf(pair.to) })).join(', ')}{/if}
                  </span>
                </span>
                {#if mayShape && !each.system}
                  <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.cmdb_admin.edit_named', { name: each.key })} onclick={() => openType(each)} />
                {/if}
              </li>
            {/each}
          </ul>
        </Stack>
      {:else}
        <Stack gap="100">
          <p class="meta">{t('app.cmdb_admin.sources_hint')}</p>
          {#if mayShape}<div><Button icon="plus" onclick={() => openSource()}>{t('app.cmdb_admin.new_source')}</Button></div>{/if}
          <ul class="rows">
            {#each sources as each (each.key)}
              <li class="row">
                <span class="what">
                  <strong>{each.key}</strong>
                  <span class="meta">
                    {t(`app.cmdb_admin.source_kind.${each.kind}`)} · {t('app.cmdb_admin.rank', { rank: each.rank })}
                    · {each.classes.length > 0 ? each.classes.map(nameOf).join(', ') : t('app.cmdb_admin.every_class')}
                    · {each.may_create ? t('app.cmdb_admin.may_create') : t('app.cmdb_admin.updates_only')}
                  </span>
                </span>
                {#if mayShape && !each.system}
                  <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.cmdb_admin.edit_named', { name: each.key })} onclick={() => openSource(each)} />
                {/if}
              </li>
            {/each}
          </ul>
        </Stack>
      {/if}
    </Tabs>
  {/if}
</Stack>

{#if isClassOpen && classDraftState}
  <Dialog bind:isOpen={isClassOpen} title={classEditing ? t('app.cmdb_admin.edit_named', { name: nameOf(classEditing.key) }) : t('app.cmdb_admin.new_class')} dismissLabel={t('app.assets.cancel')} isWide>
    <Stack gap="150">
      {#if !classEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={classDraftState.key} maxlength={63} />{/if}
      <Input label={t('app.assets.name_label')} bind:value={classDraftState.name} maxlength={120} />
      <Select label={t('app.cmdb_admin.parent')} options={classOptions.filter((each) => each.value !== classEditing?.key)} bind:value={classDraftState.parent} />
      <Switch label={t('app.cmdb_admin.abstract')} bind:checked={classDraftState.abstract} />
      <fieldset class="group">
        <legend>{t('app.assets.attributes')}</legend>
        {#each classDraftState.attributes as attribute, index (index)}
          <div class="line">
            <Input label={t('app.cmdb_admin.key')} size="sm" bind:value={attribute.key} maxlength={63} />
            <Select label={t('app.cmdb_admin.kind_label')} size="sm" options={kindOptions} bind:value={attribute.kind} />
            <Select label={t('app.cmdb_admin.need_label')} size="sm" options={needOptions} bind:value={attribute.need} />
            <Checkbox label={t('app.cmdb_admin.identifying')} bind:checked={attribute.identifier} />
            {#if attribute.kind === 'CHOICE'}<Textarea label={t('app.cmdb_admin.options')} rows={2} bind:value={attribute.options} />{/if}
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.attributes.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.attributes.push({ key: '', kind: 'TEXT', need: 'OPTIONAL', identifier: false, options: '' })}>{t('app.cmdb_admin.add_attribute')}</Button></div>
      </fieldset>
      <Textarea label={t('app.cmdb_admin.rules')} hint={t('app.cmdb_admin.rules_hint')} rows={3} bind:value={classDraftState.rules} />
      <fieldset class="group">
        <legend>{t('app.cmdb_admin.required')}</legend>
        {#each classDraftState.required as needed, index (index)}
          <div class="line">
            <CheckboxGroup label={t('app.cmdb_admin.required_types')} options={typeKeys.map((each) => ({ value: each.value, label: each.label }))} selected={needed.types} onChange={(next) => (needed.types = [...next])} />
            <Checkbox label={t('app.cmdb_admin.outgoing')} bind:checked={needed.outgoing} />
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => classDraftState?.required.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => classDraftState?.required.push({ types: [], outgoing: true })}>{t('app.cmdb_admin.add_required')}</Button></div>
      </fieldset>
      <Input label={t('app.cmdb_admin.staleness_label')} hint={t('app.cmdb_admin.staleness_hint')} inputmode="numeric" bind:value={classDraftState.staleness} />
      {#if classEditing}<Switch label={t('app.cmdb_admin.archived')} bind:checked={classDraftState.archived} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isClassOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={classReason} onclick={() => void saveClass()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isTypeOpen && typeDraft}
  <Dialog bind:isOpen={isTypeOpen} title={typeEditing ? t('app.cmdb_admin.edit_named', { name: typeEditing.key }) : t('app.cmdb_admin.new_type')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      {#if !typeEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={typeDraft.key} maxlength={63} />{/if}
      <Input label={t('app.cmdb_admin.type_name')} bind:value={typeDraft.name} maxlength={120} />
      <Input label={t('app.cmdb_admin.type_reverse')} bind:value={typeDraft.reverse} maxlength={120} />
      <Select label={t('app.cmdb_admin.impact_label')} options={impactOptions} bind:value={typeDraft.impact} />
      <fieldset class="group">
        <legend>{t('app.cmdb_admin.pairs')}</legend>
        {#each typeDraft.pairs as pair, index (index)}
          <div class="line">
            <Select label={t('app.cmdb_admin.pair_from')} size="sm" options={classOptions} bind:value={pair.from} />
            <Select label={t('app.cmdb_admin.pair_to')} size="sm" options={classOptions} bind:value={pair.to} />
            <IconButton icon="x" size="sm" tone="subtle" label={t('app.cmdb_admin.remove')} onclick={() => typeDraft?.pairs.splice(index, 1)} />
          </div>
        {/each}
        <div><Button size="sm" tone="subtle" icon="plus" onclick={() => typeDraft?.pairs.push({ from: 'ci', to: 'ci' })}>{t('app.cmdb_admin.add_pair')}</Button></div>
      </fieldset>
      {#if typeEditing}<Switch label={t('app.cmdb_admin.archived')} bind:checked={typeDraft.archived} />{/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isTypeOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={typeReason} onclick={() => void saveType()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if isSourceOpen && sourceDraft}
  <Dialog bind:isOpen={isSourceOpen} title={sourceEditing ? t('app.cmdb_admin.edit_named', { name: sourceEditing.key }) : t('app.cmdb_admin.new_source')} dismissLabel={t('app.assets.cancel')}>
    <Stack gap="150">
      {#if !sourceEditing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={sourceDraft.key} maxlength={63} />{/if}
      <Select label={t('app.cmdb_admin.source_kind_label')} options={sourceKinds} bind:value={sourceDraft.kind} />
      <Input label={t('app.cmdb_admin.rank_label')} hint={t('app.cmdb_admin.rank_hint')} inputmode="numeric" bind:value={sourceDraft.rank} />
      <Switch label={t('app.cmdb_admin.may_create')} bind:checked={sourceDraft.mayCreate} />
      <CheckboxGroup label={t('app.cmdb_admin.source_classes')} hint={t('app.cmdb_admin.source_classes_hint')} options={classChecks} selected={sourceDraft.classes} onChange={(next) => { if (sourceDraft) sourceDraft.classes = [...next]; }} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isSourceOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" disabledReason={sourceReason} onclick={() => void saveSource()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what { display: grid; gap: var(--sp-025); }
  .indent { padding-inline-start: calc(var(--depth, 0) * var(--sp-150)); }
  .group { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .line { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
