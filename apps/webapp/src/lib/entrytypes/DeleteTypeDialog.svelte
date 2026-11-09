<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Delete the type?* as a Dialog S (page-system §5.2). A type in use is refused by the server and
  // can only be archived (UC-FLW-01 check 5), which the sentence says before the button and the
  // refusal repeats inside the dialog.
  import { Button, Dialog } from '@hubtask/design-system/components';
  import type { EntryType } from '@hubtask/sync-engine';

  import { entryTypes } from '../data/entrytypes.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    type: EntryType | undefined;
    onClose: () => void;
    /** After the type is gone: its own page leaves for the list. */
    onDeleted?: () => void;
  }

  const { type, onClose, onDeleted }: Props = $props();

  let isWriting = $state(false);
  let failure = $state<string | undefined>(undefined);
  const nameOf = (each: EntryType) => each.name ?? t('app.entry_types.level', { level: each.level });

  async function remove() {
    if (!type) return;
    const name = nameOf(type);
    failure = undefined;
    isWriting = true;
    try {
      await entryTypes.remove(type);
      toast.show({ text: t('app.entry_types.deleted', { name }), tone: 'success' });
      onClose();
      onDeleted?.();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={type !== undefined}
  title={t('app.entry_types.delete_title', { name: type ? nameOf(type) : '' })}
  dismissLabel={t('app.entry_types.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.entry_types.delete_confirm'), tone: 'danger', isBusy: isWriting, onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.entry_types.delete_explain')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.entry_types.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; color: var(--text-secondary); }
</style>
