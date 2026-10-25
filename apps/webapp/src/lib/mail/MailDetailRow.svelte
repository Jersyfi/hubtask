<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's *By mail* line in its details (ADR-0092 M1, UC-JUM-09 check 1): the address a mail
  // becomes a comment at, and *Copy*. Drawn as a detail row is - the key small and quiet, the value
  // after it - and not drawn at all until the address is read, or where there is none.
  import { untrack } from 'svelte';
  import { IconButton } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { copyAddress, mailAddresses } from '../data/mailaddress.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  $effect(() => {
    const id = itemId;
    untrack(() => void mailAddresses.load({ kind: 'ITEM', id }));
  });

  const read = $derived(mailAddresses.of({ kind: 'ITEM', id: itemId }));
  const address = $derived(read && read !== 'failed' && read.available ? read.address : undefined);
  let copied = $state(false);

  async function copy(): Promise<void> {
    if (!address) return;
    copied = await copyAddress(address);
    if (copied) announcer.say(t('app.mail.copied_announced'));
  }
</script>

{#if address}
  <div class="row" data-detail="mail">
    <span class="key">{t('app.mail.by_mail')}</span>
    <span class="value"><code title={address}>{address}</code></span>
    <IconButton icon={copied ? 'check' : 'copy'} size="sm" label={copied ? t('app.mail.copied') : t('app.mail.copy')} onclick={() => void copy()} />
  </div>
{/if}

<style>
  .row {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--density-control-md-min);
    padding: var(--density-row-block) var(--sp-100);
    border-block-end: var(--bw-hairline) solid var(--border-subtle);
  }

  .key {
    flex: none;
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
  }

  .value { flex: 1 1 auto; min-inline-size: 0; text-align: end; }

  code {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    color: var(--text-secondary);
  }
</style>
