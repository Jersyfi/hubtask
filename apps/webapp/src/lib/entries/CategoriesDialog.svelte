<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Which of a collection's labels its list groups by, and in which order (UC-WRK-25 check 5,
  // record D10): the aisles of a shop. Opened where the list is used, by somebody who may change
  // the collection's shape; the *Structure* panel opens the same dialog.
  //
  // The write is the whole `fields` setting of the collection's own layer, the unit a structure
  // stores, with LABELS switched on for the type - categories are labels, and a *List* has none
  // until somebody groups by them. The collection keeps its own value of that setting from then
  // on rather than following its hub, which the hint says. Online only.

  import { labelTokens } from '@hubtask/design-system';
  import { Button, Checkbox, Dialog, IconButton, Inline, Input, Stack } from '@hubtask/design-system/components';
  import type { EffectiveStructure } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { labels } from '../data/labels.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    collectionId: string;
    /** The collection's version, which the write is made against (ADR-0025). */
    version: number;
    structure: EffectiveStructure;
    /** The level the list draws: its type is the one the categories group. */
    itemType: string;
    onclose: () => void;
  }

  const { collectionId, version, structure, itemType, onclose }: Props = $props();

  const available = $derived(labels.of(collectionId));
  /** The categories, in order: what the collection names now, the labels that still exist. */
  let chosen = $state<string[]>([]);
  /**
   * Whether the person has changed the choice. Until then it follows what arrives - the labels
   * loading, the answer to a write just made, another device's change; from then on it is theirs,
   * and an arriving structure does not take it back.
   */
  let touched = false;
  $effect.pre(() => {
    const named = structure.settings?.fields?.[itemType]?.categories ?? [];
    const existing = named.filter((id) => labels.of(collectionId).some((label) => label.id === id));
    if (!touched) chosen = existing;
  });

  let newName = $state('');
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);

  function toggle(id: string) {
    touched = true;
    chosen = chosen.includes(id) ? chosen.filter((each) => each !== id) : [...chosen, id];
  }

  function shift(id: string, by: -1 | 1) {
    const at = chosen.indexOf(id);
    const to = at + by;
    if (at < 0 || to < 0 || to >= chosen.length) return;
    const next = [...chosen];
    [next[at], next[to]] = [next[to] as string, next[at] as string];
    touched = true;
    chosen = next;
  }

  async function addLabel() {
    const name = newName.trim();
    if (name === '') return;
    failure = undefined;
    try {
      const made = await labels.create(
        collectionId,
        { name, color_token: labelTokens[available.length % labelTokens.length] ?? labelTokens[0] },
        crypto.randomUUID(),
      );
      touched = true;
      chosen = [...chosen, made.id];
      newName = '';
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }

  async function save() {
    isSaving = true;
    failure = undefined;
    try {
      const fields = { ...(structure.settings?.fields ?? {}) };
      const own = fields[itemType]?.capabilities ?? [];
      fields[itemType] = {
        capabilities: own.includes('LABELS') ? own : [...own, 'LABELS'],
        categories: chosen,
      };
      await containers.setStructure(collectionId, { settings: { fields } }, version);
      announcer.say(t('app.entries.categories_saved_announced'));
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<Dialog isOpen={true} title={t('app.entries.categories')} dismissLabel={t('app.entries.cancel')} onClose={onclose}>
  <Stack gap="150">
    <p class="hint">{t('app.entries.categories_hint')}</p>
    {#each available as label (label.id)}
      {@const at = chosen.indexOf(label.id)}
      <Inline gap="100">
        <Checkbox label={label.name} checked={at >= 0} onchange={() => toggle(label.id)} />
        {#if at >= 0}
          <IconButton
            icon="chevron-up"
            size="sm"
            label={t('app.entries.category_up', { name: label.name })}
            disabledReason={at === 0 ? t('app.rank.already_first') : undefined}
            onclick={() => shift(label.id, -1)}
          />
          <IconButton
            icon="chevron-down"
            size="sm"
            label={t('app.entries.category_down', { name: label.name })}
            disabledReason={at === chosen.length - 1 ? t('app.rank.already_last') : undefined}
            onclick={() => shift(label.id, 1)}
          />
        {/if}
      </Inline>
    {/each}
    <Inline gap="100">
      <Input label={t('app.entries.category_new')} bind:value={newName} />
      <Button size="sm" tone="secondary" onclick={addLabel}>{t('app.entries.category_add')}</Button>
    </Inline>
    {#if failure}
      <p class="failure" role="alert">{failure}</p>
    {/if}
    <Inline gap="100">
      <Button size="sm" isBusy={isSaving} busyLabel={t('app.workspace.saving')} onclick={save}>{t('app.entries.save')}</Button>
      <Button size="sm" tone="secondary" onclick={onclose}>{t('app.entries.cancel')}</Button>
    </Inline>
  </Stack>
</Dialog>

<style>
  .hint { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); max-width: 64ch; }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
</style>
