<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's Matrix bots (UC-INT-18 check 1): an account seated in one room that answers links to
  // the hub's entries and makes `!task` messages entries. Whoever may shape the hub seats one,
  // switches it or removes it; every member reads the list. The token is typed once and never read
  // back.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Checkbox, Dialog, EmptyState, Input, Select, Spinner, Stack, Switch, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { ChatBot, ChatBotList, Container } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { chatBotMissing, chatBotsPath, type ChatBotDraft } from '../data/chatbots.ts';
  import { chatBots } from '../data/chatbots.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
    collections: readonly Container[];
  }

  const { hubId, collections }: Props = $props();

  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  let bots = $state<readonly ChatBot[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() =>
      engine.subscribe<ChatBotList>({ path: chatBotsPath(at) }, (state) => {
        if (state.status === 'ready') bots = state.data.data;
        if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
      }),
    );
  });

  const fail = (error: unknown) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
  const collectionOptions = $derived<SelectOption[]>(collections.map((c) => ({ value: c.id, label: c.name })));
  const collectionName = (id: string) => collections.find((c) => c.id === id)?.name ?? '';

  let isAddOpen = $state(false);
  let isSeating = $state(false);
  let draft = $state<ChatBotDraft>({ collectionId: '', homeserver: '', room: '', token: '', unfurl: true, capture: true });
  const missing = $derived(chatBotMissing(draft));

  function open() {
    draft = { collectionId: collections[0]?.id ?? '', homeserver: '', room: '', token: '', unfurl: true, capture: true };
    isAddOpen = true;
  }

  async function seat() {
    isSeating = true;
    try {
      await chatBots.seat(hubId, draft);
      isAddOpen = false;
      toast.show({ text: t('app.chatbots.seated'), tone: 'success' });
    } catch (error) {
      fail(error);
    } finally {
      isSeating = false;
      draft.token = '';
    }
  }

  async function change(bot: ChatBot, what: Partial<Pick<ChatBot, 'enabled' | 'unfurl' | 'capture'>>) {
    try {
      await chatBots.change(bot, what);
    } catch (error) {
      fail(error);
    }
  }

  async function remove(bot: ChatBot) {
    try {
      await chatBots.remove(bot);
      toast.show({ text: t('app.chatbots.removed'), tone: 'success' });
    } catch (error) {
      fail(error);
    }
  }
</script>

<Stack gap="150">
  <div class="head">
    <p class="meta">{t('app.chatbots.intro')}</p>
    {#if mayShape}<Button size="sm" tone="secondary" icon="plus" data-seat-bot onclick={open}>{t('app.chatbots.add')}</Button>{/if}
  </div>
  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if bots === undefined}
    <Spinner label={t('app.chatbots.loading')} />
  {:else if bots.length === 0}
    <EmptyState kind="unused" title={t('app.chatbots.empty')} icon="message-square" />
  {:else}
    <ul class="rows">
      {#each bots as bot (bot.id)}
        <li class="bot" data-chat-bot={bot.id}>
          <div class="row-line">
            <span class="what">
              <span>{bot.room}</span>
              <span class="meta">{bot.bot_user} · {bot.homeserver}</span>
              {#if bot.capture}<span class="meta">{t('app.chatbots.captures_into', { collection: collectionName(bot.collection_id) })}</span>{/if}
              {#if bot.failure}<span class="meta failure">{t(bot.failure)}</span>{/if}
            </span>
            <Switch
              label={t('app.chatbots.enabled')}
              checked={bot.enabled}
              disabledReason={mayShape ? undefined : t('app.chatbots.shape_first')}
              onchange={(event: Event) => void change(bot, { enabled: (event.currentTarget as HTMLInputElement).checked })}
            />
          </div>
          {#if mayShape}
            <div class="actions">
              <Checkbox label={t('app.chatbots.unfurl')} checked={bot.unfurl}
                onchange={(event: Event) => void change(bot, { unfurl: (event.currentTarget as HTMLInputElement).checked })} />
              <Checkbox label={t('app.chatbots.capture')} checked={bot.capture}
                onchange={(event: Event) => void change(bot, { capture: (event.currentTarget as HTMLInputElement).checked })} />
              <Button size="sm" tone="danger" onclick={() => void remove(bot)}>{t('app.chatbots.remove')}</Button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</Stack>

{#if isAddOpen}
  <Dialog bind:isOpen={isAddOpen} title={t('app.chatbots.add')} dismissLabel={t('app.chatbots.cancel')}>
    <Stack gap="150">
      <Input label={t('app.chatbots.homeserver')} hint={t('app.chatbots.homeserver_hint')} bind:value={draft.homeserver} maxlength={512} />
      <Input label={t('app.chatbots.room')} hint={t('app.chatbots.room_hint')} bind:value={draft.room} maxlength={255} />
      <Input label={t('app.chatbots.token')} hint={t('app.chatbots.token_hint')} bind:value={draft.token} type="password" autocomplete="off" />
      <Select label={t('app.chatbots.collection')} options={collectionOptions} bind:value={draft.collectionId} />
      <Checkbox label={t('app.chatbots.unfurl')} hint={t('app.chatbots.unfurl_hint')} bind:checked={draft.unfurl} />
      <Checkbox label={t('app.chatbots.capture')} hint={t('app.chatbots.capture_hint')} bind:checked={draft.capture} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isAddOpen = false)}>{t('app.chatbots.cancel')}</Button>
      <Button tone="primary" isBusy={isSeating} data-confirm-bot disabledReason={missing ? t(missing) : undefined} onclick={() => void seat()}>
        {t('app.chatbots.seat')}
      </Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row-line { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .bot {
    display: grid; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what { display: grid; gap: var(--sp-025); overflow-wrap: anywhere; }
  .actions { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .failure { color: var(--text-danger); }
</style>
