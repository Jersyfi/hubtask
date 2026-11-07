<!--
  SPDX-License-Identifier: Apache-2.0
  Copyright (c) 2026 Jérôme Bastian Winkel
-->
<script lang="ts">
  // The mail server refused the person's address for good, so no message is sent to it: said on
  // the profile with a way to have it tried again once the mailbox works. Absent where mail arrives.
  import { Button, Callout, Stack } from '@hubtask/design-system/components';

  import { actor } from '../data/account.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { renderProblem } from '../problem.ts';

  let isClearing = $state(false);

  async function retry(): Promise<void> {
    isClearing = true;
    try {
      await actor.clearUndeliverable();
      toast.show({ text: t('app.profile.undeliverable_cleared'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isClearing = false;
    }
  }
</script>

{#if actor.mailUndeliverableSince && actor.account?.email}
  <Callout tone="warning" title={t('app.profile.undeliverable_title')}>
    <Stack gap="100">
      <span data-undeliverable>{t('app.profile.undeliverable_text', {
        address: actor.account.email,
        since: formatDateTime(actor.mailUndeliverableSince, messages.locale),
      })}</span>
      <span>
        <Button size="sm" isBusy={isClearing} busyLabel={t('app.profile.undeliverable_retrying')} onclick={() => void retry()}>
          {t('app.profile.undeliverable_retry')}
        </Button>
      </span>
    </Stack>
  </Callout>
{/if}
