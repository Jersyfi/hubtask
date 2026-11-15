<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An object's relations (UC-AST-01 check 5, work-surface §6.10, scene `ci`) as a picture or as a
  // list - one at a time, the reader's choice kept on this device. The picture has the object in
  // the middle, what rests on it on one side and what it depends on or runs on on the other, each
  // edge named by its relation type; a neighbour with an open entry on it shows that entry's key,
  // one in a stage other than *in use* says the stage. The list names direction, type and object
  // with the source that wrote it; removing is in a row's ⋯. *+ Relation* opens a dialog.
  //
  // What the reader may never do is not drawn (P-05): without a write, no ⋯ and no *+ Relation*.
  import { untrack } from 'svelte';
  import { Button, Dialog, FoldingSection, Menu, IconButton, Select, Stack, StateLozenge, ViewSwitcher, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, CiRelationType, ConfigurationItem } from '@hubtask/sync-engine';

  import { classWord, cmdb, relationRows, relationSides, relationWord, typeAllows, type RelationRow, type Words } from '../data/cmdb.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { isQuietStage, keepRelationsView, readRelationsView, type RelationsView } from './objectpage.ts';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    types: readonly CiRelationType[];
    words: Words;
    /** The objects the hub's copy holds, and those read alone: the other ends' names and stages. */
    known: ReadonlyMap<string, ConfigurationItem>;
    /** The hub's objects a relation may be made to. */
    hubCis: readonly ConfigurationItem[];
    /** The key of an open entry on a neighbour, by the neighbour's id. */
    openKeys: ReadonlyMap<string, string>;
    mayWrite: boolean;
    /** The reader, whose choice of picture or list this device keeps. */
    reader?: string;
    /** Asked from the head's ⋯: the relate dialog opens once. */
    isRelateAsked?: boolean;
    onrelateasked?: () => void;
  }

  const { ci, classes, types, words, known, hubCis, openKeys, mayWrite, reader, isRelateAsked = false, onrelateasked }: Props = $props();

  const rows = $derived(relationRows(ci.id, ci.relations ?? []));
  const sides = $derived(relationSides(rows));
  const nameOf = (id: string) => known.get(id)?.name ?? t('app.assets.unknown_ci');
  const typeOf = (key: string) => types.find((type) => type.key === key);
  const wordOf = (row: RelationRow) => relationWord(typeOf(row.type), row.type, row.outgoing, words);
  const sourceWord = (source: string) => (source === 'manual' ? t('app.object.origin_manual') : t('app.object.origin_source', { source }));

  const store = () => (typeof localStorage === 'undefined' ? undefined : localStorage);
  // A phone opens on the list (scene `ci`, phone), a desk on the picture, until the reader chooses.
  let view = $state<RelationsView>(untrack(() => readRelationsView(store(), reader)) ?? (viewport.isCompact ? 'list' : 'picture'));
  function choose(id: string) {
    view = id === 'list' ? 'list' : 'picture';
    keepRelationsView(store(), view, reader);
  }

  async function act<T>(run: () => Promise<T>, done?: string): Promise<T | undefined> {
    try {
      const out = await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  // A relation made from this object's end, offered only where the type's object-type pairs join
  // the two (`cmdb.relation_not_allowed` stays the server's answer).
  const klass = $derived(classes.find((each) => each.key === ci.class));
  let isRelateOpen = $state(false);
  let relateTarget = $state('');
  let relateChoice = $state('');
  let isRelating = $state(false);
  const target = $derived(known.get(relateTarget));
  const targetClass = $derived(target ? classes.find((each) => each.key === target.class) : undefined);
  const relateOptions = $derived<SelectOption[]>(
    !target
      ? []
      : types.flatMap((type) => [
          ...(typeAllows(type, klass, targetClass) ? [{ value: `${type.key}:out`, label: relationWord(type, type.key, true, words) }] : []),
          ...(typeAllows(type, targetClass, klass) ? [{ value: `${type.key}:in`, label: relationWord(type, type.key, false, words) }] : []),
        ]),
  );
  const targetOptions = $derived<SelectOption[]>(
    hubCis.filter((each) => each.id !== ci.id && each.status !== 'MERGED').map((each) => ({ value: each.id, label: `${each.name} · ${classWord(each.class, classes, words)}` })),
  );

  function openRelate() {
    relateTarget = '';
    relateChoice = '';
    isRelateOpen = true;
  }
  $effect(() => {
    if (!isRelateAsked) return;
    openRelate();
    onrelateasked?.();
  });

  async function relate() {
    const [type, way] = relateChoice.split(':');
    if (!type || !relateTarget) return;
    isRelating = true;
    const done = await act(
      () => (way === 'out' ? cmdb.relate(ci.hub_id, ci.id, relateTarget, type) : cmdb.relate(ci.hub_id, relateTarget, ci.id, type)),
      t('app.assets.related'),
    );
    isRelating = false;
    if (done) isRelateOpen = false;
  }

  function onrow(row: RelationRow, id: string) {
    if (id !== 'remove') return;
    const relation = ci.relations?.find((each) => each.id === row.id);
    if (relation) void act(() => cmdb.unrelate(ci.hub_id, relation), t('app.assets.unrelated'));
  }

  const canWrite = $derived(mayWrite && !ci.merged_into);
</script>

<FoldingSection id="ci-relations" title={t('app.object.mark_relations')} count={rows.length > 0 ? String(rows.length) : undefined} {actions}>
  {#snippet children()}
    {#if rows.length === 0}
      <p class="none">{t('app.assets.no_relations')}</p>
    {:else if view === 'picture'}
      <div class="picture-frame"><div class="picture" data-ci-graph role="group" aria-label={t('app.object.picture', { name: ci.name })}>
        {#snippet side(each: readonly RelationRow[], more: number, kind: 'in' | 'out')}
          <ul class="side" data-side={kind}>
            {#each each as row (row.id)}
              {@const other = known.get(row.other)}
              {@const key = openKeys.get(row.other)}
              <li class="node" data-has-entry={key ? '' : undefined}>
                <span class="edge">{wordOf(row)}</span>
                <a class="node-name" href={`/cis/${row.other}`}>{nameOf(row.other)}</a>
                {#if key || (other && isQuietStage(other.stage))}
                  <span class="marks">
                    {#if key}<StateLozenge tone="danger" label={key} />{/if}
                    {#if other && isQuietStage(other.stage)}<StateLozenge tone="warning" label={t(`app.assets.stage.${other.stage}`)} />{/if}
                  </span>
                {/if}
              </li>
            {/each}
            {#if more > 0}<li class="more">{t('app.assets.graph_more', { count: more })}</li>{/if}
          </ul>
        {/snippet}
        {@render side(sides.incoming, sides.moreIncoming, 'in')}
        <div class="centre"><span class="node-name">{ci.name}</span></div>
        {@render side(sides.outgoing, sides.moreOutgoing, 'out')}
      </div></div>
    {:else}
      <ul class="rows" data-ci-relation-list>
        {#each rows as row (row.id)}
          {@const other = known.get(row.other)}
          <li class="row" data-relation-row>
            <span class="row-what">
              <span class="row-word">{wordOf(row)}</span>
              <a href={`/cis/${row.other}`}>{nameOf(row.other)}</a>
              {#if other}<span class="meta">{classWord(other.class, classes, words)}</span>{/if}
            </span>
            <span class="row-end">
              {#if openKeys.get(row.other)}<StateLozenge tone="danger" label={openKeys.get(row.other) ?? ''} />{/if}
              <span class="meta">{sourceWord(row.source)}</span>
              {#if canWrite}
                <Menu label={t('app.object.relation_menu', { name: nameOf(row.other) })} placement={{ side: 'block-end', align: 'end' }}
                  items={[{ id: 'remove', label: t('app.assets.unrelate'), icon: 'x', isDestructive: true }]} onselect={(id) => onrow(row, id)}>
                  {#snippet trigger(props)}
                    <IconButton icon="ellipsis" label={t('app.object.relation_menu', { name: nameOf(row.other) })} tone="subtle" size="sm" {...props} />
                  {/snippet}
                </Menu>
              {/if}
            </span>
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</FoldingSection>

{#snippet actions()}
  {#if rows.length > 0}
    <ViewSwitcher label={t('app.object.view')} views={[{ id: 'picture', label: t('app.object.view_picture') }, { id: 'list', label: t('app.object.view_list') }]} selected={view} onselect={choose} />
  {/if}
  {#if canWrite}
    <Button size="sm" tone="subtle" icon="plus" data-relate onclick={openRelate}>{t('app.object.add_relation')}</Button>
  {/if}
{/snippet}

{#if isRelateOpen}
  <Dialog size="m" bind:isOpen={isRelateOpen} title={t('app.object.relate_title', { name: ci.name })} dismissLabel={t('app.assets.cancel')}
    primary={{ label: t('app.assets.relate'), isBusy: isRelating, disabledReason: relateChoice ? undefined : t('app.assets.relation_type'), onclick: () => void relate() }}>
    <Stack gap="150">
      <Select label={t('app.assets.relation_target')} options={targetOptions} placeholder={t('app.assets.relation_pick')} bind:value={relateTarget} />
      {#if target}
        {#if relateOptions.length === 0}
          <p class="none">{t('app.assets.relation_none_fits')}</p>
        {:else}
          <Select label={t('app.assets.relation_type')} options={relateOptions} placeholder={t('app.assets.relation_type')} bind:value={relateChoice} />
        {/if}
      {/if}
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isRelateOpen = false)}>{t('app.assets.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .none { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); }

  /* The picture: the object in the middle, each neighbour's edge its own line towards it, so
     nothing is measured; on a narrow column the sides stand above and below. */
  .picture-frame { container-type: inline-size; }
  .picture {
    display: grid; grid-template-columns: minmax(0, 1fr); align-items: center; gap: var(--sp-200);
    padding: var(--sp-300) var(--sp-200); border-radius: var(--r-md); background: var(--bg-surface-sunken);
  }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a container query cannot read a custom property. */
  @container (inline-size >= 600px) {
    .picture { grid-template-columns: minmax(0, 1fr) minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-500); }
    .node::after { content: ''; position: absolute; inset-block-start: 50%; inline-size: var(--sp-500); border-block-start: var(--bw-hairline) solid var(--border-strong); }
    .node[data-has-entry]::after { border-block-start-style: dashed; }
    .side[data-side='in'] .node::after { inset-inline-start: 100%; }
    .side[data-side='out'] .node::after { inset-inline-end: 100%; }
  }
  .side { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-150); }
  .node, .centre {
    position: relative; display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-050) var(--sp-100);
    padding: var(--sp-050) var(--sp-150); border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md); background: var(--bg-surface); min-inline-size: 0;
  }
  .node[data-has-entry] { border-color: var(--status-danger-border); background: var(--status-danger-surface); }
  .centre { justify-self: center; border-color: var(--accent-primary); border-width: var(--bw-thick); font-weight: var(--fw-semibold); }
  .edge { flex-basis: 100%; color: var(--text-subtle); font-size: var(--fs-065); }
  .node-name { min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  a.node-name:hover, .row-what a:hover { text-decoration: underline; }
  .marks { display: inline-flex; gap: var(--sp-050); }
  .more { color: var(--text-subtle); font-size: var(--fs-075); padding-inline: var(--sp-150); }

  .rows { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-050) var(--sp-150);
    min-block-size: var(--sp-500); padding-block: var(--sp-050); border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }
  .row:last-child { border-block-end: 0; }
  .row-what { display: inline-flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-050) var(--sp-100); min-inline-size: 0; }
  .row-word { color: var(--text-secondary); font-size: var(--fs-075); }
  .row-what a { color: var(--text-primary); font-weight: var(--fw-medium); text-decoration: none; }
  /* A thumb's target (§7): the link's press reaches layout.target beyond its line of text. */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .row-what a { position: relative; }
    .row-what a::after { content: ''; position: absolute; inset-block: min(0%, calc((100% - var(--layout-target)) / 2)); inset-inline: 0; }
  }
  .row-end { display: inline-flex; align-items: center; gap: var(--sp-100); margin-inline-start: auto; }
</style>
