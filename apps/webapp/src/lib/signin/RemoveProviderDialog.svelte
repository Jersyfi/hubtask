<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Removing a provider costs something, and the Dialog S says what before its button: the people
  // it signed in keep their accounts and lose the way back in through it (page-system §5.2).
  import { Button, Dialog } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { identityProvider } from '../data/identityprovider.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The provider to remove; the dialog is open while there is one. */
    target?: { id: string; name: string };
    onClose: () => void;
    onremoved?: () => void;
  }

  const { target, onClose, onremoved }: Props = $props();

  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  async function remove(): Promise<void> {
    if (!target) return;
    isWorking = true;
    failure = undefined;
    try {
      await identityProvider.remove(target.id);
      onClose();
      onremoved?.();
    } catch (cause) {
      failure = cause instanceof TransportError
        ? renderProblem(cause, messages)
        : { message: messages.t('app.something_went_wrong'), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={target !== undefined}
  title={t('app.identity_provider.remove_title', { name: target?.name ?? '' })}
  dismissLabel={t('app.identity_provider.keep')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.identity_provider.remove_now'), tone: 'danger', onclick: () => void remove(), isBusy: isWorking, busyLabel: t('app.identity_provider.removing') }}
>
  <p class="body">{t('app.identity_provider.remove_cost')}</p>
  <p class="body quiet">{t('app.identity_provider.remove_confirm')}</p>
  {#if failure}<FailureNote problem={failure} />{/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={onClose}>{t('app.identity_provider.keep')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
  .quiet { color: var(--text-secondary); }
</style>
