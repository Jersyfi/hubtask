<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's forge hooks (UC-INT-13 check 1): GitHub, GitLab, Forgejo or Gitea post the hub's
  // pushes and pull requests to the address shown, proving the secret drawn here. Whoever may
  // shape the hub adds one, renames it, switches it off or draws its secret again; every member
  // reads the list. A hook is switched off, never removed: removing it would erase its links.
  //
  // **A secret is never read back**: it is shown once when drawn and lives in this component until
  // the reader confirms they kept it.

  import { untrack } from 'svelte';
  import {
    Banner, Button, Dialog, EmptyState, Input, OneTimeSecret, Select, Spinner, Stack, Switch, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CodeHook, CodeHookList } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { FORGES, codeHooksPath, codeIntakeAddress } from '../data/code.ts';
  import { code } from '../data/code.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import IntakeAddress from '../mail/IntakeAddress.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
  }

  const { hubId }: Props = $props();

  const mayShape = $derived(holdsHere('STRUCTURE') === true);

  let hooks = $state<readonly CodeHook[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CodeHookList>({ path: codeHooksPath(at) }, (state) => {
          if (state.status === 'ready') hooks = state.data.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        workspace.open(),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const fail = (error: unknown) => toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
  const forgeWord = (kind: CodeHook['kind']) => t(`app.code.forge.${kind}`);
  const forgeOptions = $derived<SelectOption[]>(FORGES.map((kind) => ({ value: kind, label: forgeWord(kind) })));
  const address = (hook: CodeHook) =>
    workspace.workspace?.id ? codeIntakeAddress(window.location.origin, workspace.workspace.id, hook.id) : undefined;

  let drawn = $state<{ readonly hook: CodeHook; readonly secret: string } | undefined>(undefined);

  // Adding.
  let isAddOpen = $state(false);
  let kind = $state<string>('GITHUB');
  let name = $state('');
  async function add() {
    try {
      const answered = await code.create(hubId, kind as CodeHook['kind'], name.trim());
      drawn = { hook: answered, secret: answered.secret };
      isAddOpen = false;
      name = '';
    } catch (error) {
      fail(error);
    }
  }

  // Renaming.
  let renaming = $state<CodeHook | undefined>(undefined);
  let newName = $state('');
  async function rename() {
    if (!renaming) return;
    try {
      await code.update(renaming, { name: newName.trim() });
      renaming = undefined;
    } catch (error) {
      fail(error);
    }
  }

  async function toggle(hook: CodeHook, enabled: boolean) {
    try {
      await code.update(hook, { enabled });
    } catch (error) {
      fail(error);
    }
  }

  async function rotate(hook: CodeHook) {
    try {
      const answered = await code.rotate(hook);
      drawn = { hook: answered, secret: answered.secret };
    } catch (error) {
      fail(error);
    }
  }
</script>

<Stack gap="200">
  {#if drawn}
    <OneTimeSecret
      value={drawn.secret}
      label={t('app.code.secret', { name: drawn.hook.name })}
      hint={t(`app.code.secret_hint.${drawn.hook.kind}`)}
      revealLabel={t('app.apps.reveal')}
      hideLabel={t('app.apps.hide')}
      copyLabel={t('app.apps.copy')}
      copiedLabel={t('app.apps.copied')}
      acknowledgementLabel={t('app.apps.kept')}
      notAcknowledgedReason={t('app.apps.keep_first')}
      dismissLabel={t('app.apps.done')}
      onDismiss={() => (drawn = undefined)}
    />
  {/if}

  <div class="head">
    <p class="meta">{t('app.code.hooks_intro')}</p>
    {#if mayShape}<Button size="sm" tone="primary" icon="plus" onclick={() => (isAddOpen = true)}>{t('app.code.hook_new')}</Button>{/if}
  </div>

  {#if refusal}
    <Banner tone="danger">{refusal}</Banner>
  {:else if !hooks}
    <Spinner label={t('app.code.loading')} />
  {:else if hooks.length === 0}
    <EmptyState kind="unused" title={t('app.code.hooks_empty')} icon="git-branch" />
  {:else}
    <ul class="rows">
      {#each hooks as hook (hook.id)}
        {@const at = address(hook)}
        <li class="hook" data-code-hook={hook.kind}>
          <div class="row-line">
            <span class="what">
              <strong>{hook.name}</strong>
              <span class="meta">
                {forgeWord(hook.kind)}
                · {hook.last_delivery_at ? t('app.code.last_delivery', { when: formatDateTime(hook.last_delivery_at, messages.locale) }) : t('app.code.never_delivered')}
              </span>
            </span>
            <Switch
              label={t('app.code.hook_enabled')}
              checked={hook.enabled}
              disabledReason={mayShape ? undefined : t('app.code.shape_first')}
              onchange={(event: Event) => void toggle(hook, (event.currentTarget as HTMLInputElement).checked)}
            />
          </div>
          {#if at}<IntakeAddress address={at} lead={t('app.code.address')} />{/if}
          {#if mayShape}
            <div class="actions">
              <Button size="sm" tone="subtle" icon="pencil" onclick={() => { renaming = hook; newName = hook.name; }}>{t('app.code.rename')}</Button>
              <Button size="sm" tone="subtle" icon="key" onclick={() => void rotate(hook)}>{t('app.code.redraw')}</Button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</Stack>

{#if isAddOpen}
  <Dialog bind:isOpen={isAddOpen} title={t('app.code.hook_new')} dismissLabel={t('app.code.cancel')}>
    <Stack gap="150">
      <Select label={t('app.code.forge_label')} options={forgeOptions} bind:value={kind} />
      <Input label={t('app.code.name_label')} hint={t('app.code.name_hint')} bind:value={name} maxlength={80} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isAddOpen = false)}>{t('app.code.cancel')}</Button>
      <Button tone="primary" disabledReason={name.trim() ? undefined : t('forge.hook_name_required')} onclick={() => void add()}>{t('app.code.add')}</Button>
    {/snippet}
  </Dialog>
{/if}

{#if renaming}
  <Dialog isOpen={true} onClose={() => (renaming = undefined)} title={t('app.code.rename')} dismissLabel={t('app.code.cancel')}>
    <Input label={t('app.code.name_label')} bind:value={newName} maxlength={80} />
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (renaming = undefined)}>{t('app.code.cancel')}</Button>
      <Button tone="primary" disabledReason={newName.trim() ? undefined : t('forge.hook_name_required')} onclick={() => void rename()}>{t('app.code.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .head { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row-line { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }
  .hook {
    display: grid; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .what { display: grid; gap: var(--sp-025); }
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-050); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
