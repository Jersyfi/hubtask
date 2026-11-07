<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Remove the app?* (page-system §5.2): a Dialog S naming what it costs - removing an app
  // withdraws every grant that pointed at it, and whatever it runs stops on its next request.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import { apps, type App } from '../data/apps.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { app, onClose, onRemoved }: { app: App | undefined; onClose: () => void; onRemoved?: () => void } = $props();

  let isWorking = $state(false);
  let failure = $state<string | undefined>(undefined);

  async function remove(): Promise<void> {
    if (!app) return;
    const name = app.name;
    failure = undefined;
    isWorking = true;
    try {
      await apps.remove(app.id);
      announcer.say(t('app.apps.removed_announced'));
      toast.show({ text: t('app.apps.removed', { name }), tone: 'success' });
      onClose();
      onRemoved?.();
    } catch (cause) {
      failure = renderProblem(cause as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  title={t('app.apps.remove_title', { name: app?.name ?? '' })}
  isOpen={app !== undefined}
  dismissLabel={t('app.apps.keep')}
  onClose={() => {
    failure = undefined;
    onClose();
  }}
  primary={{ label: t('app.apps.remove_now'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.apps.working'), onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.apps.remove_cost')}</p>
  {#snippet actions()}
    <Button tone="secondary" onclick={onClose}>{t('app.apps.keep')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
