<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Delete the group?* (page-system §5.2): a Dialog S naming the group and what deleting it costs -
  // whatever it granted, nobody holds afterwards. Said before the button, not after it.
  import { Button, Dialog } from '@hubtask/design-system/components';
  import type { Group } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    group: Group | undefined;
    memberCount: number;
    onClose: () => void;
    /** After the group is gone: a record page leaves for the list. */
    onDeleted?: () => void;
  }

  const { group, memberCount, onClose, onDeleted }: Props = $props();

  let isWriting = $state(false);
  let failure = $state<string | undefined>(undefined);

  async function remove(): Promise<void> {
    if (!group) return;
    const name = group.name;
    failure = undefined;
    isWriting = true;
    try {
      await groups.remove(group.id);
      announcer.say(t('app.groups.removed_announced'));
      toast.show({ text: t('app.groups.removed_toast', { name }), tone: 'success' });
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
  title={t('app.groups.delete_title', { name: group?.name ?? '' })}
  isOpen={group !== undefined}
  dismissLabel={t('app.groups.keep')}
  {onClose}
  primary={{ label: t('app.groups.delete_now'), tone: 'danger', isBusy: isWriting, busyLabel: t('app.groups.working'), onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body" data-delete-cost>{t('app.groups.delete_cost_count', { count: String(memberCount) })} {t('app.groups.delete_cost')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.groups.keep')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
