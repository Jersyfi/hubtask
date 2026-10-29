<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The open duplicates side by side (UC-AST-03 check 5): every attribute any of the CIs holds, what
  // differs marked in words, and a member who may write keeps one - the others merged into it and
  // retired, never deleted - or records them as distinct. A person decides every merge.

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, EmptyState, Spinner, Stack } from '@hubtask/design-system/components';
  import type { CiClass, CiDuplicate, CiDuplicateList, ConfigurationItem } from '@hubtask/sync-engine';

  import { classWord, cmdb, duplicatesPath, sideBySide, type Words } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
  }

  const { hubId, classes, words, mayWrite }: Props = $props();

  let list = $state<readonly CiDuplicate[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let read = $state<Record<string, ConfigurationItem>>({});
  $effect(() => {
    const at = hubId;
    return untrack(() =>
      engine.subscribe<CiDuplicateList>({ path: duplicatesPath(at) }, (state) => {
        if (state.status === 'ready') list = state.data.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      }),
    );
  });
  $effect(() => {
    const wanted = (list ?? []).flatMap((each) => each.ci_ids);
    untrack(() => {
      for (const id of new Set(wanted)) {
        if (read[id]) continue;
        void cmdb.read(id).then((ci) => {
          if (ci) read = { ...read, [id]: ci };
        });
      }
    });
  });

  const word = (key: string) =>
    key === 'name' ? t('app.assets.name_label') : key === 'class' ? t('app.assets.class_label')
    : messages.has(`app.assets.attribute.${key}`) ? t(`app.assets.attribute.${key}`) : key;

  async function resolve(duplicate: CiDuplicate, survivor?: ConfigurationItem) {
    try {
      await cmdb.resolveDuplicate(hubId, duplicate.id, survivor?.id);
      toast.show({ text: survivor ? t('app.assets.merged', { name: survivor.name }) : t('app.assets.separated'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
{:else if !list}
  <Spinner label={t('app.assets.loading')} />
{:else if list.length === 0}
  <EmptyState kind="unused" title={t('app.assets.duplicates_empty')} icon="copy" />
{:else}
  <Stack gap="200">
    {#each list as duplicate (duplicate.id)}
      {@const cis = duplicate.ci_ids.map((id) => read[id]).filter((each): each is ConfigurationItem => Boolean(each))}
      <section class="duplicate">
        <div class="head">
          <span class="meta">{t('app.assets.duplicate_rule', { rule: duplicate.rule.split('+').map(word).join(' + ') })}</span>
          {#if mayWrite}<Button size="sm" tone="secondary" onclick={() => void resolve(duplicate)}>{t('app.assets.distinct')}</Button>{/if}
        </div>
        {#if cis.length < duplicate.ci_ids.length}
          <Spinner label={t('app.assets.loading')} />
        {:else}
          <div class="scroll">
            <table class="side">
              <thead>
                <tr>
                  <th scope="col">{t('app.assets.attribute')}</th>
                  {#each cis as ci (ci.id)}
                    <th scope="col">
                      <a href={`/cis/${ci.id}`}>{ci.name}</a>
                      {#if mayWrite}<Button size="sm" tone="subtle" icon="check" onclick={() => void resolve(duplicate, ci)}>{t('app.assets.merge_into')}</Button>{/if}
                    </th>
                  {/each}
                </tr>
              </thead>
              <tbody>
                {#each sideBySide(cis) as row (row.key)}
                  <tr>
                    <th scope="row">
                      {word(row.key)}
                      {#if row.differ}<Badge size="sm" tone="warning">{t('app.assets.differs')}</Badge>{/if}
                    </th>
                    {#each row.values as value, index (index)}
                      <td>{row.key === 'class' ? classWord(value, classes, words) : value || '—'}</td>
                    {/each}
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </section>
    {/each}
  </Stack>
{/if}

<style>
  .duplicate {
    display: grid; gap: var(--sp-100); padding: var(--sp-100) var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); background: var(--bg-surface);
  }
  .head { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .scroll { overflow-x: auto; }
  .side { border-collapse: collapse; inline-size: 100%; font-size: var(--fs-075); }
  .side th, .side td { text-align: start; vertical-align: baseline; padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .side thead th { display: table-cell; }
  .side a { color: var(--text-primary); font-weight: var(--fw-medium); margin-inline-end: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); }
</style>
