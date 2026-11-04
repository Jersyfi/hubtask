<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's key prefix (user-experience.md §5.6): the word its entries' keys start with,
  // "INC" in "INC-42". Derived from the name when the collection was made; whoever shapes the
  // collection changes or clears it, and a cleared prefix reads the keys with the handle again.

  import { Banner, Button, Input } from '@hubtask/design-system/components';
  import type { Container } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { ticketKeyOf } from '../data/ticket.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    container: Container;
    mayShape: boolean;
  }

  let { container, mayShape }: Props = $props();

  let draft = $state('');
  // The draft follows the stored prefix whenever that changes - a save, another device's change.
  $effect.pre(() => {
    draft = container.key_prefix ?? '';
  });
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);

  const wanted = $derived(draft.trim().toUpperCase());
  const isChanged = $derived(wanted !== (container.key_prefix ?? ''));
  // What the keys will read once saved, beside the field, so the effect is seen before it is made.
  const example = $derived(ticketKeyOf({ handle: container.handle, key_prefix: wanted || null }, 42));

  async function save() {
    failure = undefined;
    isSaving = true;
    try {
      const saved = await containers.update(container.id, { key_prefix: wanted || null }, container.version);
      announcer.say(t('app.keys.saved', { key: ticketKeyOf(saved, 42) ?? '' }));
    } catch (cause) {
      failure = renderProblem(cause as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<div class="key-prefix" data-key-prefix>
  {#if mayShape}
    <div class="edit">
      <Input label={t('app.keys.prefix')} hint={t('app.keys.prefix_hint')} bind:value={draft} maxlength={5} />
      <Button tone="secondary" isBusy={isSaving} busyLabel={t('app.mail.saving')} disabledReason={isChanged ? undefined : t('app.mail.unchanged')} onclick={() => void save()}>{t('app.keys.save')}</Button>
    </div>
  {/if}
  {#if example}<p class="hint">{t('app.keys.example', { key: example })}</p>{/if}
  {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
</div>

<style>
  .key-prefix { display: grid; gap: var(--sp-100); }
  .edit { display: flex; align-items: flex-end; gap: var(--sp-150); flex-wrap: wrap; }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
