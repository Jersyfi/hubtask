<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A group's own fields - its name and description - as a Dialog M (page-system §5.1), opened by
  // *Edit* on its record page and by *Edit…* in its row's menu. Who is in it is the record page's
  // list, never this dialog: renaming places nothing, so it needs only MANAGE_MEMBERS.
  import { Button, Dialog, Input, Stack, Textarea } from '@hubtask/design-system/components';
  import type { Group } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { groups } from '../data/groups.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { isComplete, type Missing } from '../forms/missing.ts';
  import { marked } from '../forms/press.ts';

  const { group, onClose }: { group: Group | undefined; onClose: () => void } = $props();

  let isOpen = $state(false);
  let name = $state('');
  let description = $state('');
  let missing = $state<Missing>({});
  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  $effect(() => {
    if (!group) return;
    name = group.name;
    description = group.description ?? '';
    missing = {};
    failure = undefined;
    isOpen = true;
  });

  const guard = new DiscardGuard(
    () => group !== undefined && (name !== group.name || description !== (group.description ?? '')),
    () => (isOpen = false),
  );

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!group) return;
    const next = name.trim();
    missing = marked(event, [{ kind: 'field', key: 'rename', code: 'app.forms.required', isFilled: next !== '' }]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWriting = true;
    try {
      await groups.update(group.id, { name: next, description: description.trim() === '' ? null : description.trim() });
      announcer.say(t('app.groups.renamed_announced'));
      toast.show({ text: t('app.groups.saved_toast', { name: next }), tone: 'success' });
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.groups.edit_title', { name: group?.name ?? '' })}
  bind:isOpen
  dismissLabel={t('app.groups.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={{ label: t('app.groups.save'), isBusy: isWriting, busyLabel: t('app.groups.working'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={save}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Input label={t('app.groups.name')} hint={t('app.groups.name_hint')} bind:value={name} isRequired error={missing.rename && !name.trim() ? t(missing.rename) : undefined} />
      <Textarea label={t('app.groups.description')} hint={t('app.groups.description_hint')} bind:value={description} />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) isOpen = false; }}>{t('app.groups.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
</style>
