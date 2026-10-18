<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // "Your provider's account page" (UC-INS-13 check 11): where the workspace's owners and
  // administrators manage what they are on. Drawn only where the server answered an address to this
  // reader - nothing for anybody else and nothing where the operator set none (P-05). The label is
  // ours and says no price, no plan and no offer; the address is the operator's.

  import { accountLinkHere } from '../data/capability.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** An address answered elsewhere - the paused refusal's - in place of the manifest's. */
    href?: string;
  }

  const { href }: Props = $props();

  const address = $derived(href ?? accountLinkHere());
</script>

{#if address}
  <a class="account-link" href={address} target="_blank" rel="noopener noreferrer" data-testid="account-link">{t('app.plan.account_link')}</a>
{/if}

<style>
  .account-link { font-weight: var(--fw-medium); }
</style>
