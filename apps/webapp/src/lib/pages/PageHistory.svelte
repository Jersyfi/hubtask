<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A page's history in place of its text (UC-DOC-06): the versions with person and time on the
  // left, the chosen one beside the page as it stands, side by side or its changes only; bringing
  // it back and naming it are buttons on top. The comparison is the client's (compare.ts).

  import { Button, Inline, StateLozenge, ViewSwitcher } from '@hubtask/design-system/components';
  import { compareBodies } from '@hubtask/editor';

  import { t } from '../i18n/i18n.svelte.ts';
  import { changeRuns, sideBySide } from './history.ts';

  interface Version {
    readonly seq: number;
    readonly author_id: string | null;
    readonly name?: string | null;
    readonly restored_from?: number | null;
    readonly created_at: string;
  }

  interface Props {
    title: string;
    /** The page as it stands: its body, who changed it last and when. */
    body: string;
    changedBy: string;
    changedAt: string;
    versions: readonly Version[];
    /** The version chosen to compare, its body read. */
    chosen: { seq: number; body: string; name?: string | null; created_at: string } | undefined;
    writable: boolean;
    /** From a laptop on: the list beside the comparison, the two versions beside each other. */
    wide: boolean;
    nameOf: (id: string | null) => string;
    when: (iso: string) => string;
    onchoose: (seq: number) => void;
    onrestore: (seq: number) => void;
    onname: (seq: number) => void;
    onclose: () => void;
  }

  const { title, body, changedBy, changedAt, versions, chosen, writable, wide, nameOf, when, onchoose, onrestore, onname, onclose }: Props = $props();

  let layout = $state<'side' | 'changes'>('side');
  const compared = $derived(chosen ? compareBodies(chosen.body, body) : []);
  const columns = $derived(sideBySide(compared));
  const runs = $derived(changeRuns(compared));
</script>

<section class="history" class:wide aria-label={t('app.pages.side_history')} data-page-history>
  <div class="top">
    <Button size="sm" tone="subtle" icon="chevron-left" onclick={onclose}>{t('app.pages.history_back')}</Button>
    <span class="trail"><span class="ell">{title}</span> › <span class="here">{t('app.pages.side_history')}</span></span>
    {#if writable && chosen}
      <Inline gap="100">
        <Button size="sm" tone="secondary" icon="tag" onclick={() => onname(chosen.seq)}>{t('app.pages.version_name')}</Button>
        <Button size="sm" tone="primary" icon="rotate-ccw" onclick={() => onrestore(chosen.seq)}>{t('app.pages.version_restore')}</Button>
      </Inline>
    {/if}
  </div>
  <div class="grid">
    <section class="versions" aria-labelledby="history-versions">
      <h2 id="history-versions" class="head">{t('app.pages.history_versions', { count: String(versions.length) })}</h2>
      <ol>
        <li class="version now" aria-current="true">
          <span class="mark on" aria-hidden="true"></span>
          <span class="what"><span class="when">{t('app.pages.history_now')}</span><span class="who">{changedBy} · {when(changedAt)}</span></span>
        </li>
        {#each versions as version (version.seq)}
          <li>
            <button type="button" class="version" aria-pressed={chosen?.seq === version.seq} onclick={() => onchoose(version.seq)}>
              <span class="mark" class:chosen={chosen?.seq === version.seq} aria-hidden="true"></span>
              <span class="what">
                <span class="when">{when(version.created_at)}</span>
                <span class="who">{nameOf(version.author_id)}{version.restored_from ? ` · ${t('app.pages.version_restored_from', { seq: String(version.restored_from) })}` : ''}</span>
                {#if version.name}<StateLozenge size="sm" tone="brand" icon="tag" label={version.name} />{/if}
              </span>
            </button>
          </li>
        {:else}
          <li class="quiet">{t('app.pages.no_versions')}</li>
        {/each}
      </ol>
      <p class="note">{t('app.pages.history_note')}</p>
    </section>
    <div class="compare">
      <div class="bar">
        <ViewSwitcher label={t('app.pages.history_layout')} selected={layout} onselect={(id: string) => (layout = id as 'side' | 'changes')}
          views={[{ id: 'side', label: t('app.pages.history_side_by_side') }, { id: 'changes', label: t('app.pages.history_changes_only') }]} />
        <span class="legend">
          <del>{t('app.pages.history_removed')}</del>
          <ins>{t('app.pages.history_added')}</ins>
          <b>{t('app.pages.history_places', { count: String(runs.length) })}</b>
        </span>
      </div>
      {#if chosen}
        {#if layout === 'side'}
          <div class="sheet sides" data-history-sides>
            <div class="side">
              <span class="label">{when(chosen.created_at)}{chosen.name ? ` · „${chosen.name}“` : ''}</span>
              {#each columns.before as line, at (at)}
                <p class="line" class:empty={line.text === ''}>{#if line.kind === 'removed'}<del>{line.text}</del>{:else}{line.text}{/if}</p>
              {/each}
            </div>
            <div class="side">
              <span class="label">{t('app.pages.history_now')}</span>
              {#each columns.after as line, at (at)}
                <p class="line" class:empty={line.text === ''}>{#if line.kind === 'added'}<ins>{line.text}</ins>{:else}{line.text}{/if}</p>
              {/each}
            </div>
          </div>
        {:else}
          <div class="sheet runs" data-history-changes>
            {#each runs as run, at (at)}
              <div class="run">
                {#each run as line, index (index)}
                  <p class="line">{#if line.kind === 'removed'}<del>{line.text}</del>{:else}<ins>{line.text}</ins>{/if}</p>
                {/each}
              </div>
            {:else}
              <p class="quiet">{t('app.pages.history_same')}</p>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </div>
</section>

<style>
  .history { display: grid; gap: var(--sp-300); padding-block-end: var(--sp-800); }
  .top { display: flex; align-items: center; gap: var(--sp-150); flex-wrap: wrap; }
  .trail { flex: 1; min-inline-size: 0; display: flex; gap: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-090); }
  .trail .here { color: var(--text-primary); }
  .ell { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .grid { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-400); align-items: start; }
  .wide .grid { grid-template-columns: var(--layout-compare-side) minmax(0, 1fr); }
  .versions { border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised); }
  .head { margin: 0; padding: var(--sp-150) var(--sp-200); font-size: var(--fs-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  ol { margin: 0; padding: var(--sp-100); list-style: none; display: grid; gap: var(--sp-025); }
  .version { display: flex; gap: var(--sp-100); align-items: flex-start; inline-size: 100%; padding: var(--sp-100); border: none; border-radius: var(--r-md); background: none; color: inherit; font: inherit; text-align: start; cursor: pointer; }
  button.version:hover { background: var(--bg-surface-hover); }
  button.version[aria-pressed='true'] { background: var(--accent-primary-subtle); }
  .mark { flex: none; inline-size: var(--sp-200); block-size: var(--sp-200); margin-block-start: var(--sp-025); border: var(--bw-ring) solid var(--border-strong); border-radius: var(--r-full); }
  .mark.on { background: var(--accent-primary); border-color: var(--accent-primary); }
  .mark.chosen { border-color: var(--accent-primary); }
  .what { display: grid; gap: var(--sp-025); justify-items: start; }
  .when { font-weight: var(--fw-semibold); }
  .who { color: var(--text-subtle); font-size: var(--fs-090); }
  .note { margin: 0; padding: var(--sp-150) var(--sp-200); border-block-start: var(--bw-hairline) solid var(--border-subtle); color: var(--text-subtle); font-size: var(--fs-090); }
  .quiet { color: var(--text-subtle); }
  .compare { display: grid; gap: var(--sp-200); min-inline-size: 0; }
  .bar { display: flex; align-items: center; gap: var(--sp-200); flex-wrap: wrap; justify-content: space-between; }
  .legend { display: flex; gap: var(--sp-150); align-items: center; font-size: var(--fs-090); color: var(--text-subtle); }
  del { background: var(--status-danger-surface); color: var(--status-danger-text); }
  ins { background: var(--status-success-surface); color: var(--status-success-text); text-decoration: none; }
  .sheet { padding: var(--sp-300); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-raised); }
  .sides { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-300); }
  .wide .sides { grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); }
  .wide .sides .side + .side { border-inline-start: var(--bw-hairline) solid var(--border-subtle); padding-inline-start: var(--sp-300); }
  .side { display: grid; align-content: start; gap: var(--sp-050); min-inline-size: 0; }
  .label { font-size: var(--fs-090); font-weight: var(--fw-semibold); color: var(--text-subtle); margin-block-end: var(--sp-100); }
  .line { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: var(--lh-normal); }
  .line.empty { min-block-size: var(--sp-100); }
  .runs { display: grid; gap: var(--sp-200); }
  .run + .run { border-block-start: var(--bw-hairline) solid var(--border-subtle); padding-block-start: var(--sp-200); }
</style>
