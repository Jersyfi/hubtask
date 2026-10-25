<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A place's mail address (ADR-0092, UC-JUM-09 check 1): where the jumble, a collection or an
  // entry takes mail, with *Copy* beside it.
  //
  // Two shapes. `line` is the address and a copy button on one row - the jumble's head, *Today*,
  // an entry's details. `panel` adds what may be shaped there - *New address* and who may send -
  // for the collection's settings and the jumble's own section.
  //
  // Without a receiving domain there is no field: the people who can change the place read one line
  // that says what is missing (`adminHint`), and everybody else reads nothing.
  import { untrack } from 'svelte';
  import { Banner, Button, IconButton, Radio, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { mailAddresses, copyAddress, type MailTarget, type SenderPolicy } from '../data/mailaddress.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    target: MailTarget;
    shape?: 'line' | 'panel';
    /** Whether the reader may shape the place: *New address*, who may send, and the admin hint. */
    mayShape?: boolean;
    /** The sentence above the address in `line`; none draws the address alone. */
    lead?: string;
  }

  const { target, shape = 'line', mayShape = false, lead }: Props = $props();

  const key = $derived(`${target.kind}:${target.id ?? ''}`);
  $effect(() => {
    void key;
    untrack(() => void mailAddresses.load(target));
  });

  const read = $derived(mailAddresses.of(target));
  const address = $derived(read && read !== 'failed' && read.available ? read.address : undefined);
  const unavailable = $derived(read !== undefined && read !== 'failed' && !read.available);

  let copied = $state(false);
  let rotating = $state(false);
  let isWorking = $state(false);
  let failure = $state<string | undefined>(undefined);
  let policy = $state<SenderPolicy>('MEMBERS');
  $effect(() => {
    if (read && read !== 'failed' && read.sender_policy) policy = read.sender_policy;
  });

  async function copy(): Promise<void> {
    if (!address) return;
    copied = await copyAddress(address);
    if (copied) announcer.say(t('app.mail.copied_announced'));
  }

  async function attempt(work: () => Promise<unknown>, said: string): Promise<void> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      announcer.say(said);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }

  function choosePolicy(next: string): void {
    if (read && read !== 'failed' && read.sender_policy === next) return;
    void attempt(() => mailAddresses.setPolicy(target, next as SenderPolicy), t('app.mail.policy_announced'));
  }
</script>

{#if address}
  {#if shape === 'line'}
    <div class="line" data-mail-address>
      {#if lead}<span class="lead">{lead}</span>{/if}
      <code class="address">{address}</code>
      <Button tone="subtle" size="sm" icon={copied ? 'check' : 'copy'} onclick={() => void copy()}>
        {copied ? t('app.mail.copied') : t('app.mail.copy')}
      </Button>
    </div>
  {:else}
    <Stack gap="150">
      <div class="line" data-mail-address>
        <code class="address">{address}</code>
        <IconButton icon={copied ? 'check' : 'copy'} label={copied ? t('app.mail.copied') : t('app.mail.copy')} onclick={() => void copy()} />
      </div>
      {#if failure}<Banner tone="danger">{failure}</Banner>{/if}
      {#if mayShape && target.kind !== 'JUMBLE'}
        <!-- Who may send means nothing for the jumble: everything that reaches it waits there. -->
        <Radio
          label={t('app.mail.policy')}
          bind:value={() => policy, (next) => { policy = next as SenderPolicy; choosePolicy(next); }}
          options={[
            { value: 'MEMBERS', label: t('app.mail.policy_members'), hint: t('app.mail.policy_members_hint') },
            { value: 'ANYONE', label: t('app.mail.policy_anyone'), hint: t('app.mail.policy_anyone_hint') },
          ]}
        />
      {/if}
      {#if mayShape}
        {#if rotating}
          <Banner tone="warning">{t('app.mail.rotate_cost')}</Banner>
          <div class="row">
            <Button
              tone="danger"
              isBusy={isWorking}
              busyLabel={t('app.mail.rotating')}
              onclick={() =>
                void attempt(async () => {
                  await mailAddresses.rotate(target);
                  rotating = false;
                  copied = false;
                }, t('app.mail.rotated_announced'))}
            >
              {t('app.mail.rotate_now')}
            </Button>
            <Button tone="subtle" onclick={() => (rotating = false)}>{t('app.mail.cancel')}</Button>
          </div>
        {:else}
          <div><Button tone="secondary" size="sm" onclick={() => (rotating = true)}>{t('app.mail.rotate')}</Button></div>
        {/if}
      {/if}
    </Stack>
  {/if}
{:else if unavailable && mayShape}
  <p class="hint" data-mail-unavailable>{t('app.mail.unavailable_admin')}</p>
{/if}

<style>
  .line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--sp-100);
    min-inline-size: 0;
  }

  .lead { color: var(--text-secondary); }

  .address {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    padding-block: var(--sp-025);
    padding-inline: var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--bg-recess);
    overflow-wrap: anywhere;
    min-inline-size: 0;
  }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .hint { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
