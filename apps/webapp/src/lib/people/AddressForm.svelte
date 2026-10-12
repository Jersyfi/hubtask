<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Add an address* for a managed account (UC-ID-20 check 7), on the profile for one's own and on
  // *People* for somebody else's. A link goes to the address; nothing changes until somebody holding
  // that mailbox follows it. A refusal of the address stands under its field, anything else above.

  import { Banner, Button, Input, Stack } from '@hubtask/design-system/components';

  import { people } from '../data/people.svelte.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    accountId: string;
    /** What the form says the address does, in the reader's words: for oneself or for somebody. */
    hint: string;
    /** Called once the link is on its way, with the address it went to. */
    onSent?: (email: string) => void;
  }

  const { accountId, hint, onSent }: Props = $props();

  let email = $state('');
  let isWriting = $state(false);
  let failure = $state<string | undefined>(undefined);
  let fieldError = $state<string | undefined>(undefined);
  /** The address the newest link went to; the banner is mounted empty and filled after. */
  let sentTo = $state<string | undefined>(undefined);

  async function send(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const address = email.trim();
    if (!address) return;
    failure = undefined;
    fieldError = undefined;
    isWriting = true;
    try {
      await people.addAddress(accountId, address);
      sentTo = address;
      email = '';
      announcer.say(t('app.people.address_sent', { email: address }));
      onSent?.(address);
    } catch (error) {
      const problem = renderProblem(error as never, messages);
      const own = problem.fields.get('/email');
      if (own) fieldError = own;
      else failure = problem.message;
    } finally {
      isWriting = false;
    }
  }
</script>

<Stack gap="150">
  {#if failure}
    <Banner tone="danger">{failure}</Banner>
  {/if}
  {#if sentTo}
    <Banner tone="success">{t('app.people.address_sent', { email: sentTo })}</Banner>
  {/if}
  <form onsubmit={send}>
    <Stack gap="150">
      <Input
        label={t('app.people.address_email')}
        {hint}
        bind:value={email}
        type="email"
        autocomplete="email"
        spellcheck={false}
        error={fieldError}
        isRequired
      />
      <div>
        <Button type="submit" tone="primary" isBusy={isWriting} busyLabel={t('app.people.working')}>
          {t('app.people.address_submit')}
        </Button>
      </div>
    </Stack>
  </form>
</Stack>

<style>
  form { margin: 0; max-inline-size: 52ch; }
</style>
