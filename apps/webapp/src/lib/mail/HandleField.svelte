<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A collection's mail handle (ADR-0094): the word a subject (`#einkauf`), an address and the
  // ticket keys (`EINKAUF-42`) use. Copied with one click, changed by whoever shapes the
  // collection. The subject works through every door the workspace's intake has, so it is the one
  // thing always shown; the address only where the operator's receiving domain makes one.

  import { Banner, Button, IconButton, Input, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import type { Container } from '@hubtask/sync-engine';
  import { copyHandle, readMailSettings, routesTo, type MailSettings } from '../data/mailintake.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    container: Container;
    mayShape: boolean;
  }

  let { container, mayShape }: Props = $props();

  let draft = $state('');
  // The draft follows the stored handle whenever that changes - a save, another device's change.
  $effect.pre(() => {
    draft = container.handle ?? '';
  });
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);
  let settings = $state<MailSettings | undefined>(undefined);

  const handle = $derived(container.handle ?? '');
  const ways = $derived(handle ? routesTo(handle, settings) : undefined);
  const isChanged = $derived(draft.trim().toLowerCase() !== handle);

  // The address needs the workspace's name on the domain, which only its administrators read; for
  // everybody else the subject word is the way, and the read's refusal is not an error here.
  $effect(() => {
    void readMailSettings().then((read) => (settings = read), () => undefined);
  });

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      announcer.say(t('app.mail.copied_announced'));
    } catch {
      announcer.say(t('app.mail.copy_failed', { address: text }));
    }
  }

  async function save() {
    failure = undefined;
    isSaving = true;
    try {
      const saved = await containers.setHandle(container.id, container.parent_id, draft.trim().toLowerCase());
      draft = saved;
      announcer.say(t('app.mail.handle_saved', { handle: saved }));
    } catch (cause) {
      failure = renderProblem(cause as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<Stack gap="200">
  {#if ways}
    <div class="way" data-mail-subject>
      <span class="label">{t('app.mail.subject_way')}</span>
      <code>{ways.subject}</code>
      <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: ways.subject })} onclick={() => void copyHandle(handle, (code, params) => t(code, params)).then((said) => said && announcer.say(said))} />
    </div>
    {#if ways.address}
      <div class="way" data-mail-address>
        <span class="label">{t('app.mail.address_way')}</span>
        <code>{ways.address}</code>
        <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: ways.address })} onclick={() => void copy(ways.address ?? '')} />
      </div>
    {/if}
    <p class="hint">{t('app.mail.key_hint', { key: `${handle.toUpperCase()}-42` })}</p>
  {/if}
  {#if mayShape}
    <div class="edit">
      <Input label={t('app.mail.handle')} hint={t('app.mail.handle_hint')} bind:value={draft} />
      <Button tone="secondary" isBusy={isSaving} busyLabel={t('app.mail.saving')} disabledReason={isChanged ? undefined : t('app.mail.unchanged')} onclick={() => void save()}>{t('app.mail.save_handle')}</Button>
    </div>
  {/if}
  {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
</Stack>

<style>
  .way { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; }
  .label { color: var(--text-secondary); font-size: var(--fs-075); }
  code { font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .edit { display: flex; align-items: flex-end; gap: var(--sp-150); flex-wrap: wrap; }
</style>
