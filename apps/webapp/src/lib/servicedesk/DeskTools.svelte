<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The team's tools beside a comment (UC-SVC-06): a canned reply filled for the entry and put into
  // the draft - it sends nothing -, and a macro, a manual rule pressed on the entry (check 4). Only the rules
  // the reader may see and that are switched on and pressed by hand are offered; whether one may
  // run on this entry is the server's to say.

  import { untrack } from 'svelte';
  import { Button, Select } from '@hubtask/design-system/components';
  import type { ReplyTemplateList } from '@hubtask/sync-engine';

  import { desk, repliesPath } from '../data/desk.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { rules } from '../data/rules.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { itemId, oninsert }: { itemId: string; oninsert: (text: string) => void } = $props();

  const replies = untrack(() => resource<ReplyTemplateList>(repliesPath));
  const replyOptions = $derived(
    replies.state.status === 'ready' ? replies.state.data.data.map((reply) => ({ value: reply.key, label: reply.name })) : [],
  );
  $effect(() => untrack(() => rules.open()));
  const macros = $derived(rules.all.filter((rule) => rule.enabled && rule.trigger.kind === 'MANUAL'));
  const macroOptions = $derived(macros.map((rule) => ({ value: rule.id, label: rule.name })));

  let reply = $state('');
  let macro = $state('');
  let isBusy = $state(false);
  let refusal = $state<string | undefined>(undefined);

  async function fill() {
    if (!reply) return;
    refusal = undefined;
    isBusy = true;
    try {
      oninsert((await desk.render(itemId, reply)).body);
      reply = '';
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }

  async function press() {
    if (!macro) return;
    refusal = undefined;
    isBusy = true;
    try {
      await desk.press(macro, itemId);
      toast.show({ text: t('app.desk.macro_pressed'), tone: 'success' });
      macro = '';
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isBusy = false;
    }
  }
</script>

{#if replyOptions.length > 0 || macroOptions.length > 0}
  <div class="tools" data-desk-tools>
    {#if replyOptions.length > 0}
      <div class="pair">
        <Select label={t('app.desk.reply_pick')} options={[{ value: '', label: t('app.desk.reply_none') }, ...replyOptions]} bind:value={reply} />
        <Button tone="subtle" size="sm" icon="message-square" isBusy={isBusy && reply !== ''} busyLabel={t('app.desk.filling')}
          disabledReason={reply === '' ? t('app.desk.reply_first') : undefined} onclick={() => void fill()}>{t('app.desk.reply_insert')}</Button>
      </div>
    {/if}
    {#if macroOptions.length > 0}
      <div class="pair">
        <Select label={t('app.desk.macro_pick')} options={[{ value: '', label: t('app.desk.macro_none') }, ...macroOptions]} bind:value={macro} />
        <Button tone="subtle" size="sm" icon="zap" isBusy={isBusy && macro !== ''} busyLabel={t('app.desk.pressing')}
          disabledReason={macro === '' ? t('app.desk.macro_first') : undefined} onclick={() => void press()}>{t('app.desk.macro_press')}</Button>
      </div>
    {/if}
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </div>
{/if}

<style>
  .tools { display: grid; gap: var(--sp-100); grid-template-columns: repeat(auto-fit, minmax(min(100%, 28ch), 1fr)); }
  .pair { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .refusal { margin: 0; color: var(--text-danger); grid-column: 1 / -1; }
</style>
